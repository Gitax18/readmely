# Cloud application security: rate limits, IP blocking and WAF rules

Status: **Phase B built** (A, C, D to do) · Budget: at most **+$5/month** over today's bill · Planned cost: **$0/month**

The goal is to stop floods, brute force and scanners before they cost money or availability:
- rate limits on every request,
- automatic temporary blocks for addresses that keep misbehaving,
- WAF rules at the edge,

all without raising the monthly bill by more than $5.

## 1. How requests reach us today

```text
Browser ──► Vercel edge ──► Next.js on Vercel ──► /api/* bridge ──┐
                                                                  │  HTTPS + X-Readmely-Origin-Key
Scanners ──────────────────────────────────────────────────────►  ▼
                                    EC2 t3.small · security group: 80, 443 only (no SSH)
                                    └─ Caddy :443  TLS · origin-key gate (404 without it) · 256 KB bodies
                                       └─ Go API  per-feature Redis limits ─► Postgres, Redis
```

Two facts shape the plan:

1. **Browsers never talk to the API directly.** The API only accepts requests that carry the origin key, which only the Vercel server has. Everything else gets a 404 from Caddy. Vercel passes the visitor's address in `X-Forwarded-For`, Caddy keeps it, and the API trusts it only from known proxies (`internal/clientip`).
   - So the real public surface is **Vercel**. Per-visitor limits belong at the Vercel edge and in the API, which both see the visitor's address.
   - A WAF in front of the API would see only Vercel's addresses.
2. **Free protection is already on.** Vercel mitigates network-level DDoS on every plan, and AWS Shield Standard covers the Elastic IP at no cost.

### What already exists

| Layer | Protection | Where |
|---|---|---|
| Edge | Vercel's automatic DDoS mitigation | Vercel, every plan |
| Origin | The origin-key gate: no key means 404, so scanners learn nothing | `infra/prod/Caddyfile` |
| Origin | Request bodies capped at 256 KB; the origin key is never logged | `infra/prod/Caddyfile` |
| Network | Security group: 80 and 443 only, no port 22 (SSM instead of SSH) | `docs/deploy/aws.md` |
| App | Redis-backed limits per address or per user on the sensitive actions (table below) | `internal/ratelimit` |
| App | CSRF: Origin must match `WEB_ORIGIN`, JSON-only bodies, CSRF token | `internal/auth/http.go` |
| Money | The daily LLM spend cap, credit holds, budgets and billing alarms | `app/meter.py`, AWS Budgets |

Existing per-feature limits: sign-in code 1/min and 5/hour per address, 20/hour per IP · code checks 30 per 15 min per IP · OAuth 30 per 15 min per IP · projects 10 created and 20 analyses an hour per user · README 10 an hour, sections 30 an hour · diagrams 10 an hour.

### Gaps

- No **global** per-IP limit: an address can call cheap endpoints (project lists, polling, `/v1/me`) as fast as it likes.
- No **automatic block**: an address that trips limits again and again is refused each time, but never shut out.
- No **edge WAF rules**: scanner paths (`/wp-login.php`, `/.env`, ...) and floods reach Vercel functions, which count toward the plan's usage.
- No easy view of who is being refused.

## 2. Options and what they cost

| Option | Protects | Monthly cost | Verdict |
|---|---|---|---|
| **Vercel WAF on Hobby**: 3 custom rules, one of which may be a rate limit (fixed window); IP blocks; Attack Challenge Mode | Everything users reach | **$0**: Hobby includes 1M rate-limited "allowed" requests and has hard caps rather than overage billing | **Do it** (phase A) |
| **Global limiter and temporary block list in the API** (Redis) | Every API request, keyed on the real visitor | **$0** | **Do it** (phase B) |
| **CrowdSec** agent reading Caddy logs, plus a firewall bouncer on the EC2 host | Direct hits on the Elastic IP (scanners) | **$0** (open source; ~100 MB of the ~1.8 GB free RAM) | Optional (phase C) |
| **CloudFront flat-rate Free plan with AWS WAF** in front of the API: 1M requests and 100 GB a month, 5 WAF rules | The API origin | **$0** while under 1M requests a month | Not now: it would only see Vercel's addresses. Revisit if browsers or a mobile app ever call the API directly (section 5) |
| AWS WAF on its own | Needs CloudFront or an ALB | Web ACL $5 + $1 a rule + $0.60 per million requests, so **≥ $8** | Over budget |
| Application Load Balancer for WAF | — | **~$16+** | Over budget |
| Vercel Pro (40 rules, more rate-limit rules) | Edge | **$20** | Over budget. Note that Hobby is for non-commercial use: if Readmely starts charging, Pro becomes a requirement, not an option |
| Shield Advanced | DDoS with response team | $3,000 | No |

Everything planned below costs **$0**, which keeps well inside the $5 limit.

## 3. The plan

### Phase A: Vercel edge rules ($0, about 30 minutes in the dashboard)

Vercel → Project → Firewall. Hobby allows **3 custom rules, at most one of them a rate limit**, so they are spent on the three things the edge does best.

| # | Name | If | Then | Why |
|---|---|---|---|---|
| 1 | `rate-limit-api` | Request path starts with `/api/` | **Rate limit**: 120 requests per 60 s per IP (fixed window), then **deny** (429) for the rest of the window | Floods hit the bridge, which costs a function call and an API call each. Normal use is far below this: the busiest page polls about every 2 s, roughly 30/min per tab |
| 2 | `deny-scanners` | Path matches `^/(wp-admin\|wp-login\.php\|xmlrpc\.php\|\.env\|\.git\|phpmyadmin\|cgi-bin\|vendor/phpunit\|server-status)` or ends in `.php` | **Deny** | No such paths exist; denying at the edge spends no function time |
| 3 | `deny-odd-methods` | Method is not one of GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS | **Deny** | Blocks TRACE, CONNECT and made-up methods that probing tools send |

Also, at no cost:
- **IP blocking** (Firewall → IP Blocking) is the manual tool when one address is clearly abusive. Hobby allows only a few entries, so keep it for incidents.
- **Attack Challenge Mode** is the emergency switch during a flood. Every visitor gets a browser challenge first, so turn it off again afterwards: it also challenges `fetch` calls from the page.
- **Check after a week** in Firewall → Traffic: allowed, denied and rate-limited counts. Adjust rule 1 if real users show up in the denied numbers.

The limits are verified for Hobby: 3 custom rules, 1 rate-limit rule, a fixed window, 1M included allowed requests. Vercel's docs state these; one user reports a 402 error when trying to add a second rate-limit rule on Hobby.

### Phase B: A global limiter and temporary blocks in the API ($0, code) · built

This is the layer that sees every request, from any client, keyed on the real visitor, so it is done first. It lives in `services/core/internal/guard` and runs on the public router before any handler.

1. **Global per-visitor limit.**
   - **Limit:** **300 requests a minute** per client key (`GUARD_PER_MINUTE`, `0` turns it off). The key is `clientip.RateKey`, so IPv6 is grouped per /64.
   - **Mechanism:** one Redis script per request checks for a block and counts the request, so it is one round trip.
   - **Exempt:** `/healthz` and the internal listener.
   - **Response:** over the limit, **429 `rate_limited`** with `Retry-After`. The contract's description now says any endpoint may answer it.
2. **Strikes and temporary blocks.** These answers are strikes:
   - a 429 from any limit (so repeated wrong sign-in codes count, once the sign-in limits refuse them),
   - a 403 (a refused origin or CSRF token),
   - a 405,
   - and a 404 for a path no route serves. A route's own 404, such as polling a deleted project, is not a strike.

   The rules:
   - **20 strikes in 10 minutes** block the address. Every request from it is answered 429 at once, before any database work.
   - **Escalation:** the first block lasts 15 minutes, a second within 24 hours lasts 1 hour, a third lasts 24 hours.
   - **Keys:** `rm:guard:min:<hash>`, `rm:guard:strikes:<hash>`, `rm:guard:block:<hash>`, `rm:guard:blocks:<hash>`. The address is hashed, and every key expires within a day.
   - **Logging:** each block is logged once (`address blocked`, with the address, the duration and the strike count). That line is the only place the address itself is written.
3. **Our own web server is never limited.** Server-rendered pages call the API from Vercel. Those calls now pass the visitor's address (`apps/web/src/lib/visitor.ts`), so they count against that visitor. A request that still resolves to one of our own proxies carries no visitor address, so it is let through uncounted: counting it would put the whole site behind one limit, and blocking it would take the site down.
4. **Failure behaviour.** If Redis is unavailable, the guard lets requests through and logs a warning at most once a minute. The sign-in and feature limits behind it keep failing closed.
5. **Local and CI:** the dev stack sets `GUARD_PER_MINUTE=0`, because every request there comes from one address (the web container). Production leaves it unset, which means 300.
6. **Tests** (`internal/guard/guard_integration_test.go`, against Redis):
   - the limit, the 429 and `Retry-After`, and two visitors behind the web server counted apart;
   - probing leads to a block, the block expires, and the next block is longer;
   - a route's own 404 is not a strike; unknown paths, including under `/v1/`, are;
   - a forged `X-Forwarded-For` from an untrusted peer is ignored;
   - the web server's own calls leave no keys;
   - Redis down lets requests through, and `0` turns the guard off.
7. **Still to do (Phase D):** `blocked_now` and `blocks_24h` counts in the internal status detail, and a script to list current blocks over SSM.

Starting values (tune from logs after a week):

| Rule | Limit | Window | On breach |
|---|---|---|---|
| Global, per visitor | 300 requests | 1 min | 429 + a strike |
| Strikes, per visitor | 20 strikes | 10 min | block: 15 min, then 1 h, then 24 h |
| Existing per-feature limits | unchanged | — | 429 + a strike |

### Phase C (optional): CrowdSec on the EC2 host ($0)

Scanners that hit the Elastic IP directly already get a 404 from Caddy. CrowdSec turns repeat offenders into firewall drops, so they stop reaching Caddy at all.

- **Components:** `crowdsecurity/crowdsec` as one more container reading Caddy's JSON access log, plus the `caddy`, `http-cve` and `http-probing` collections. `crowdsec-firewall-bouncer` runs on the host (nftables) and drops banned addresses for 4 hours.
- **Fits the box:** about 100 MB of memory and a few % CPU at idle; no new open ports. Enrolling in CrowdSec's community blocklist is free and optional.
- **The risk is self-blocking.** Allowlist Vercel's egress by trusting only requests that carry the origin key: those never count as attacks. Also check that Let's Encrypt renewals (port 80, `/.well-known/acme-challenge/`) are never banned.
- **Order:** do this only after phases A and B, and only if the logs show heavy direct scanning.

### Phase D: Watching it ($0)

- **Vercel:** Firewall → Traffic, weekly at first.
- **API:** the `address blocked` log lines, plus the counters from B7.
- **Uptime:** the uptime workflow (`.github/workflows/uptime.yml`) already alerts when the site or the API stops answering.
- **Billing:** the existing budgets stay the backstop. Nothing in this plan adds a billed resource, so no new budget is needed.

## 4. Rollout order

| Step | What | Cost | Effort | Reversible by |
|---|---|---|---|---|
| 1 | Phase B: limiter, strikes, blocks, tests (built) | $0 | ~1 day | Setting `GUARD_PER_MINUTE=0` for the API, then redeploying |
| 2 | Phase A: the three Vercel rules | $0 | 30 min | Disabling the rule in the dashboard |
| 3 | Phase D: counters and the weekly look | $0 | 1 h | — |
| 4 | Phase C, if the logs justify it | $0 | 2 h | `docker compose stop crowdsec` and removing the bouncer |

## 5. When AWS WAF becomes worth it

If the API is ever called directly by browsers or a mobile app, or Vercel stops being the only client, put **CloudFront on the flat-rate Free plan** in front of the API:
- $0 for up to 1M requests and 100 GB a month, with AWS WAF included and 5 rules.
- Restrict port 443 in the security group to the CloudFront origin-facing prefix list (`com.amazonaws.global.cloudfront.origin-facing`).
- Keep port 80 open only for certificate renewals.

Proposed web ACL (5 rules):

| Priority | Rule | Type | Action |
|---|---|---|---|
| 0 | `AWSManagedRulesAmazonIpReputationList` | Managed | Block known bad addresses |
| 1 | `AWSManagedRulesKnownBadInputsRuleSet` | Managed | Block exploit patterns (Log4j and similar) |
| 2 | `AWSManagedRulesCommonRuleSet` | Managed | Block OWASP basics (body size rule set to Count: README saves are up to 128 KB) |
| 3 | `rate-per-ip` | Rate-based: 1,500 requests per 5 min per IP | Block |
| 4 | `require-origin-key` | Header `X-Readmely-Origin-Key` missing | Block (moves the Caddy gate to the edge) |

Above 1M requests a month the next plan is Pro at $15, which breaks the $5 limit. At that point, compare it against Vercel Pro and decide together.

## 6. Risks

- **Shared addresses:** offices, universities and mobile carriers put many people behind one IP. The limits above are generous on purpose, and IPv6 is keyed per /64, not per address.
- **Trusting `X-Forwarded-For`:** the API only believes it from trusted proxies (audit F13). If the hosting changes, the trusted-proxy list must change with it, or every visitor shares one limit.
- **Redis outage:** the global limit fails open, so an outage loses that protection instead of taking the site down.
- **Hobby's terms:** Hobby is for non-commercial use. Paid plans for Readmely mean Vercel Pro at $20/month, which is outside this budget.

## Sources

- [Vercel WAF](https://vercel.com/docs/vercel-firewall/vercel-waf) and [WAF rate limiting](https://vercel.com/docs/vercel-firewall/vercel-waf/rate-limiting): rule limits per plan, Hobby's one fixed-window rate-limit rule, and 1M included allowed requests.
- [Rule configuration reference](https://vercel.com/docs/vercel-firewall/vercel-waf/rule-configuration).
- [A Hobby user getting 402 on a second rate-limit rule](https://github.com/RummerLab/husbandry/issues/56).
- [AWS WAF simplified pricing](https://aws.amazon.com/blogs/security/introducing-aws-waf-simplified-pricing/): WAF is included in CloudFront's flat-rate Free plan.
- [CloudFront flat-rate pricing plans (AWS re:Post)](https://repost.aws/articles/ARW8IO6PlQQsW9F33olXhG0w/understanding-cloudfront-flat-rate-pricing-plans-predictable-cdn-and-data-transfer-costs) and [Duckbill's guide](https://www.duckbillhq.com/blog/the-complete-guide-to-cloudfronts-flat-rate-pricing/): Free plan allowances and the 5 WAF rules.
