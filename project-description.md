# Readmely

> **Your code, documented. Your projects, turned into your career.**
> Connect a repository and get a README, architecture diagrams, and a compact AI-ready context pack, then turn those projects into a resume and portfolio. You never have to write prompts or paste code.

_Product description, draft v2 · November 2026_

---

## 1. What Readmely is

Readmely reads an entire codebase once, builds a structured understanding of it (files, symbols, dependencies, database schema, APIs), and keeps that understanding up to date. Everything the user creates comes from that single understanding:

| Output | What the user gets |
|---|---|
| **README** | A complete README for GitHub, GitLab, or Bitbucket, ready to commit |
| **Architecture diagrams** | High-level, C4, DFD (level 0/1/2), UML class, sequence, ER, dependency, and deployment diagrams as SVG/PNG, plus editable diagram code |
| **Context Pack** | A compact, token-budgeted description of the whole project that the user can give to any AI tool without sharing the source code |
| **Resume** | An AI-written, Overleaf-compatible LaTeX resume based on the user's real projects |
| **Portfolio** | A hosted developer portfolio page generated from the user's profile and top projects |

**The core idea:** Readmely analyses a codebase once and reuses that analysis for every output. Because of this, each generation costs a small fraction of the tokens it would take to send the raw code to an LLM. This is what keeps the product cheap enough to sell at an Indian price point.

### Who it's for

- **Students and job seekers** who have projects on GitHub but poor READMEs and no strong resume.
- **Indie developers and freelancers** who need professional docs and diagrams for client handovers.
- **Developers who use AI tools heavily** (Claude, ChatGPT, Cursor, Claude Code, Grok) and need a reusable, compact project context instead of pasting files again and again.
- **Small teams** that want onboarding docs and diagrams that stay in sync with the code (later, see §8).

---

## 2. Core capabilities

### 2.1 Project ingestion
- **Sources:** GitHub (public and private, through a GitHub App with per-repository permissions), GitLab, and ZIP upload. Bitbucket comes later.
- **Smart filtering:** Readmely respects `.gitignore` and a `.readmelyignore` file. It skips binaries, vendored code, lock files, build output, and generated files.
- **Secret scanning before analysis:** API keys, tokens, and `.env` values are detected and removed before any code reaches an LLM.
- **Incremental re-indexing:** On re-sync, only changed files are processed again. Unchanged files are never re-analysed.
- **Auto-sync (Champ):** Readmely re-indexes on every push to the default branch and flags docs that have drifted out of date.

### 2.2 The Readmely engine (context & memory management)
This is the core of the product and what sets it apart. The user never sees it, but it is why Readmely is fast and cheap.

1. **Code structure parsing (no AI cost):** Every supported language is parsed into a syntax tree (AST), which gives Readmely the files, classes, functions and their signatures, imports, call relationships, API routes, database models and migrations, environment variables, and dependencies.
2. **Layered summaries:** Short summaries are written function → file → module → whole project, using a low-cost model. Each summary is stored against a fingerprint of the file's content, so it is reused until that file actually changes.
3. **Multiple purpose-built stores:**
   - a relational database for users, projects, and metadata
   - a vector store for semantic search over code and summaries
   - a code graph / AST store for structural questions such as "what calls this?" or "which tables does this service touch?"
   - object storage for generated documents, diagrams, and context packs
   - a cache for repeated prompts and results
4. **Minimal context retrieval:** Each generation (a README, a diagram) receives only the summaries and code fragments it needs, found by combining keyword, semantic, and graph search. A typical README is written from about 30–50K tokens of curated context instead of 1M+ tokens of raw code, **roughly 95%+ fewer input tokens**.
5. **Model routing and provider caching:** Bulk summarisation runs on a fast, cheap model. Final writing runs on a stronger model. Repeated context is served from provider-side prompt caches, and non-urgent work uses discounted batch APIs.
6. **Deterministic where possible:** ER diagrams from schema files, dependency graphs, folder trees, and much of the context pack are built directly from the parsed structure, with no AI call at all.

### 2.3 README generator
- Sections are detected from the code: overview, features, tech stack, folder structure, prerequisites, installation, environment variables, usage, API reference, scripts, deployment, testing, contributing, and license.
- **Style presets** (minimal, standard, detailed, open-source showcase) and a choice of tone.
- Automatic badges, a table of contents, and embedded Mermaid diagrams (GitHub and GitLab render them natively).
- **Section-level regeneration:** the user can rewrite only "Installation" without regenerating the whole README.
- Built-in Markdown editor with live preview and version history with diffs.
- **One-click "Open PR"** that commits the README to the repository (Champ).
- **README health score** (free, no AI cost): finds missing sections, broken links, outdated setup steps, and missing licenses.
- **Multilingual READMEs** (Champ), for example English + Hindi, Spanish, or Japanese.

### 2.4 Architecture diagrams
- **Diagram types:** high-level architecture, C4 (context/container/component), DFD level 0/1/2, UML class, sequence (for a chosen flow or endpoint), ER, module dependency graph, and deployment/infrastructure (from Dockerfile, docker-compose, Kubernetes, or Terraform).
- Diagrams are built from a structured graph of the system, placed by a layout engine, and drawn as clean **SVG** (with PNG export) in Readmely's own visual style. Every component shown is backed by evidence in the code. Users can also export editable **D2 or Mermaid** source, so diagrams are not locked into Readmely.
- An in-browser editor lets users adjust layout and labels and re-render.
- Diagrams can be embedded into the README or exported as a ZIP.

### 2.5 Context Pack (context generator)
Readmely produces a portable "project brain" that can be given to any AI assistant without sharing the full source code.

- **Contents:** project overview, architecture summary, file map, symbol index (classes and functions with signatures and one-line purposes), database schema, API routes, environment variables (names only, never values), conventions, key dependencies, and how to run and test the project.
- **Size presets:** Small (~8K tokens), Medium (~32K), Large (~128K), each with an exact token count shown.
- **Formats:** `.md` and `.txt` (as in the original idea), JSON, and ready-made agent files: `AGENTS.md`, `CLAUDE.md`, Cursor rules, and `llms.txt`.
- **Download as a single ZIP.**
- **Live context endpoint (Champ):** an MCP server URL, so tools such as Claude Code, Cursor, or ChatGPT can query the project's context directly and always get the latest version.

### 2.6 Ask your codebase (Champ)
A chat panel answers questions such as "how does auth work?" or "where is the payment webhook handled?" using the same index, with clickable references to files and line numbers. Each message uses credits.

### 2.7 Extra documentation (Champ)
`CONTRIBUTING.md`, onboarding guide, API reference (from routes and OpenAPI/Swagger), environment setup guide, a `CHANGELOG` drafted from commits and releases, and architecture decision summaries.

### 2.8 Resume builder
- **Eligibility:** at least **2 analysed projects** in the account. (The original draft said 2 in one place and 3 in another; 2 is recommended because it lowers the barrier for students.)
- The resume is generated from profile data (education, experience, skills, links) plus the user's selected projects. Project bullet points describe what was actually built, with measurable details taken from the code (stack, scale, features).
- **Built-in LaTeX editor** with live PDF preview. Output is **Overleaf-compatible**: the user can download the `.tex` file or a ZIP and open it in Overleaf without changes.
- **5 built-in templates** uploaded by the Readmely team, plus **upload your own `.tex` template**.
- **Job-description tailoring:** paste a JD and get a tailored version of the resume, a keyword-match / ATS score, and an optional cover letter.
- Multiple saved resume versions, for example "Backend roles" and "ML roles".

### 2.9 Portfolio
- A hosted portfolio at `readmely.app/u/username` (the domain is a placeholder) using one fixed, polished template with theme and colour options.
- **Sections:** an AI-written "About me" (based on top projects, work experience, GitHub bio, and Readmely bio), selected top projects with auto-generated case studies and diagrams, a career timeline, skills, and contact links.
- The user picks which projects to show and can edit any generated text.
- Basic visitor analytics, and an optional custom domain (later).

### 2.10 Account, providers & security
- Sign up with **Google** or **GitHub** (GitHub is recommended and needed for private repositories), with **two-factor authentication (2FA)** available on every account. Plain email sign-up is still open and will be decided in the Phase 2 auth discussion. An account can connect GitHub or GitLab later.
- **No API keys needed.** Readmely provides all AI generation. Users never supply their own provider keys (see §4.3).
- **Privacy:** customer code is never used to train models. Raw source is kept only as long as needed for analysis; the derived index is stored, and code is only stored if the user keeps a project synced. Users can delete a project and all derived data with one click.

---

## 3. Plans: what users get

AI usage is measured in **credits**, so cost follows actual AI spend rather than a count of documents. Lite users get small monthly credits, so they keep coming back. (The original draft used lifetime caps, which cost retention.) Champ users get a larger monthly allowance and can buy top-ups.

**Credit cost per action (proposed):**

| Action | Credits |
|---|---|
| Import & index a project (within plan size limit) | Free |
| Full README generation | 10 |
| Regenerate one README section | 2 |
| One diagram | 5 |
| Context Pack (built from the existing index) | Free; only the download limit applies |
| Ask-your-codebase message | 1 |
| Extra doc (CONTRIBUTING, onboarding, API reference…) | 10 |
| Resume generation / JD tailoring | 10 / 5 |
| Portfolio "About me" + case studies | 10 |

### Feature comparison

| | **Lite** (free) | **Champ** (paid) |
|---|---|---|
| **Price** | Free forever | **$12/mo or $99/yr** (global) · **₹599/mo or ₹4,999/yr** (India, incl. GST). Launch offer: founding members keep **$10 / ₹499** for 12 months |
| **Monthly AI credits** | 40 (≈ 2 READMEs + 4 diagrams) | 400 (≈ 20 READMEs + 30 diagrams + chat), top-ups available |
| **AI model tier** | Fast model | Higher-quality model |
| **Active projects** | 3 | 10 |
| **Repo size limit** | ~50K lines of code | ~250K lines of code |
| **Sources** | GitHub (public + private), ZIP | GitHub, GitLab, ZIP |
| **README** | ✅ generate, edit, copy/download | ✅ + one-click PR, multilingual, all style presets |
| **README health score** | ✅ | ✅ |
| **Diagrams** | ✅ high-level, ER, dependency | ✅ all types incl. C4, DFD L0–L2, sequence, deployment |
| **Diagram export** | SVG/PNG + source | SVG/PNG + source, embed in README |
| **Context Pack** | Medium preset, `.md` + `.txt` ZIP, **1 download/week** | All presets and formats (AGENTS.md, CLAUDE.md, llms.txt…), **unlimited downloads** |
| **Live MCP context endpoint** | — | ✅ |
| **Ask your codebase** | — | ✅ (credits) |
| **Extra docs** (CONTRIBUTING, onboarding, API ref, CHANGELOG) | — | ✅ |
| **Auto-sync on push + drift alerts** | Manual re-sync | ✅ |
| **Version history** | Last 3 versions | Unlimited |
| **Resume** | 1 free resume preview (default template, PDF only) | ✅ full builder, LaTeX editor, 5 templates + custom `.tex`, JD tailoring, cover letter, multiple versions |
| **Portfolio** | Basic public profile (project list) | ✅ full portfolio with AI About-me, case studies, career timeline, analytics |
| **Support** | Community | Email, priority generation queue |

**Top-up packs (Champ):** 250 credits for $4 / ₹249. Credits never expire while the subscription is active.
**Student discount:** 30% off Champ with a verified student email (`.edu`, `.ac.in`) or the GitHub Student Pack. This tier matters most in India.

---

## 4. Business insights

### 4.1 Unit economics (what a generation costs us)
List prices as of November 2026 (per 1M tokens, input / output): Claude Haiku 5.5 $0.10 / $0.50 · Claude Sonnet 5.5 $2 / $10 · Claude Opus 5.5 $4 / $20 · OpenAI GPT-5 mini ~$0.25 / $2. Batch APIs are about 50% cheaper, and cached input is up to ~95% cheaper.

| Operation | Estimated cost to Readmely |
|---|---|
| Index a medium repo (~500 files, ~1.5M tokens) with a fast model | ~$0.20–0.30 (one-time; re-syncs cost cents) |
| README, Champ quality model (~40K in / ~10K out incl. reasoning) | ~$0.15–0.20 |
| README, Lite fast model | ~$0.01–0.02 |
| One diagram, quality model | ~$0.08–0.12 |
| Context Pack | ~$0 (built from the stored index) |

- **Worst-case Champ user** who spends all 400 credits on quality-model output: about **$6/month** in AI cost. A **typical user** uses 25–35% of their allowance: about **$1.5–2.5/month**.
- **Lite user:** about **$0.05–0.30/month** on average, mostly first-time indexing.

### 4.2 Why the price changes from $10 / ₹499
- **₹499 is thin.** After 18% GST and gateway fees, ₹499 nets about ₹415 (~$4.7), while AI costs are paid in dollars. ₹599 nets about $5.6 and leaves room for a heavy user.
- **"3 per day" limits are risky.** 3 READMEs + 3 diagrams a day is up to about 180 generations a month, which can cost more than the subscription. Monthly credits cap the worst case.
- **$12 global is still cheap** next to documentation tools priced per seat (Mintlify, Swimm, and similar tools charge roughly $20+ per user per month on paid tiers). Annual plans (~30% off) improve cash flow and retention.
- **Keep $10 / ₹499 as a founding-member price.** This keeps the original idea's attractive headline price while the real costs are measured.

### 4.3 Should we ask users for API keys?
**Decision: no. Readmely provides all AI generation, and there is no option for users to bring their own key.**

Why asking for keys hurts:
- **Conversion drops.** Most students and early-career developers (the core audience, especially in India) don't have a paid Claude, OpenAI, or xAI account. A required key blocks them before they see any value.
- **The subscription becomes hard to justify.** If users pay for tokens themselves, they will ask what they are paying Readmely for, which was exactly the concern in the original idea.
- **Quality varies.** Different providers and models give different results, so the product experience can't be guaranteed.
- **Liability.** Storing thousands of third-party API keys makes Readmely a security target.

With managed AI, the subscription pays for **the AI and the platform**: indexing, storage, caching, sync, diagram rendering, LaTeX compilation, hosting, resume, and portfolio. Heavy users buy credit top-ups instead of supplying a key.

### 4.4 Competitive landscape (checked November 2026)

| Product | What it does | How Readmely differs |
|---|---|---|
| **DeepWiki** (Cognition / Devin) | AI wiki, diagrams, and chat for any repo. Free for public repos; private repos through a Devin account. Has an MCP server. | Wiki lives on their site. No committed README, no resume or portfolio, and private use is tied to Devin pricing. |
| **Google Code Wiki** | Gemini-powered living wiki with diagrams and chat, regenerated on commits. Public repos free; private repos through a Gemini CLI extension still in development. | Same gap: hosted wiki, not artifacts in your repo. No career layer. Google could expand it at any time. |
| **Mintlify, Swimm, Driver.ai, Penify** | Documentation platforms or code-coupled docs for teams | Team- and enterprise-priced, focused on doc sites and internal docs rather than individual developers |
| **GitDiagram, Swark, Eraser codebase diagrams** | Architecture diagrams from a repo | Diagrams only |
| **readme-ai** (open source) | README from repo with your own LLM key | README only, self-hosted, needs a key |
| **Repomix, Gitingest** (open source) | Pack a repo into one text file for LLMs | Packs raw code and doesn't summarise it, so packs are large. Readmely's Context Pack is summarised and token-budgeted. |
| **Resummit, GitHired, YouHired, Autopolio, Folio** | Resume or portfolio from GitHub repos | Resume or portfolio only. They read repo metadata; Readmely reads the actual code deeply and offers an Overleaf-grade LaTeX editor. |
| **DeepDocs, Pushpen, GitScribe, Mintlify agent** | GitHub apps that open a docs-update PR on every push or merge | Docs maintenance only. No diagrams, context packs, or career layer. |
| **ContextDocs, agentsmith, hcc, codebase-md** (mostly local CLIs/plugins) | Generate AGENTS.md / CLAUDE.md / Cursor rules / llms.txt from a codebase | Local developer tools rather than hosted, synced services. Readmely builds the same files from a persistent, always-updated index, plus a hosted MCP endpoint. |

**What this means:**
1. **Each feature on its own already exists.** AI repo wikis (free from Cognition and Google), README PR bots, diagram generators, AGENTS.md generators, and GitHub-to-resume tools are all on the market. Auto-sync and PRs are **table stakes**, not a differentiator. Readmely should not compete as "yet another wiki for public repos".
2. **No one combines the three layers** (docs that live in the repo, portable AI context, and career assets such as resume and portfolio) in one affordable product for individual developers, all built from one shared code index. That combination is the positioning: *"Your code becomes your docs, your AI context, and your resume."*
3. **India-first pricing, student focus, and the resume hook** are realistic growth channels that the large players are not targeting.

### 4.5 Risks
- **Name / trademark:** A web screen (November 2026) found no existing product, repository, or site using "Readmely". However, **ReadMe.io Co.** (San Francisco) holds a live US registration for **README** (Reg. 7455572, Class 42, software-as-a-service for developer documentation), which is the same category. "Readme" is close to a generic term, which weakens that mark, but a challenge for confusing similarity is still possible. Before investing in the brand, file a trademark search with IP India (Class 9/42) and in target markets, get an attorney's opinion, and keep a backup name ready.
- **Free competition** from DeepWiki and Code Wiki (see §4.4). This is why the career layer and in-repo artifacts matter.
- **Hallucinated docs:** outputs must link back to source files and should mark low-confidence sections.
- **Security and trust:** reading private code demands strong security practices from day one, including minimum GitHub permissions, encryption, deletion, and a clear privacy policy.
- **LaTeX compilation safety:** user-uploaded `.tex` files must be compiled in an isolated sandbox with shell-escape disabled.

### 4.6 Growth levers
- A "Docs by Readmely" badge in generated READMEs (optional, on by default for Lite).
- Public portfolio pages and resumes drive organic sign-ups.
- Campus ambassador and college hackathon partnerships in India.
- Launches on Product Hunt, Hacker News, and Reddit (r/developersIndia, r/cscareerquestions).

---

## 5. Investment required

### 5.1 Running costs (first 12 months)
| Item | Estimate |
|---|---|
| App hosting + background workers (indexing, rendering, LaTeX) | $40–100 / mo |
| Managed Postgres (with vector extension) + Redis + object storage | $25–70 / mo |
| AI for development and testing | $30–80 / mo |
| AI for free-tier users (~1,000 active) | $50–150 / mo |
| Email, monitoring, domain, misc. SaaS | $10–30 / mo |
| **Total monthly run-rate** | **~$150–400 / mo** |

### 5.2 One-time and setup costs
| Item | Estimate |
|---|---|
| Company registration (India, Pvt Ltd) + CA setup | ₹10,000–20,000 |
| Trademark filing (India, per class) | ₹4,500–9,000 + agent fees |
| Privacy policy / Terms of Service | ₹0–25,000 |
| Launch marketing | $0–500 |

**Payments:** Razorpay for India (UPI and cards), plus a merchant-of-record service (for example Paddle or Dodo Payments) for international sales, which handles foreign VAT and sales tax. A merchant of record charges about 5% + $0.50 per transaction. Factor that into the global price.

### 5.3 Budget scenarios (12 months)
| Scenario | Budget |
|---|---|
| **Bootstrapped:** founder(s) build it, no salaries | **₹2.5–4.5 lakh (~$3K–5K)** |
| **Lean team:** 1 hired developer in India + modest marketing | **₹10–15 lakh (~$12K–18K)** |

### 5.4 Break-even
- Net contribution per paying user, after taxes, fees, and average AI cost: about **$3 (India)** and **$8 (global)**. With a 60/40 India/global mix, that blends to about **$5 per subscriber per month**.
- With ~$400–500/month in fixed costs, break-even is about **80–100 paying users**.
- At a typical 2–5% free-to-paid conversion, that means **~2,000–5,000 sign-ups**.

---

## 6. Summary of changes from the original idea

| Original | Revised | Reason |
|---|---|---|
| API key mandatory before creating projects | Managed AI only; users never supply a key | Removes sign-up friction and gives the subscription a clear purpose |
| Lifetime caps on Lite (3 diagrams ever, 3 READMEs per project) | Small monthly credits | Users come back every month, and costs stay capped |
| Champ: 3 READMEs + 3 diagrams **per day** | 400 monthly credits + top-ups | A daily limit can exceed the subscription price |
| $10 / ₹499 | $12 / ₹599 (founding price $10 / ₹499), annual plans, student discount | ₹499 margin is too thin after GST |
| Resume eligibility "2 projects" vs "3 projects" | 2 projects | Removes the contradiction and lowers the barrier |
| Context file `.txt` + `.md` | Plus token-size presets, AGENTS.md / CLAUDE.md / llms.txt, live MCP endpoint | Differentiates from Repomix, Gitingest, and DeepWiki |
| — | Added: auto-sync, PR to repo, README health score, ask-your-codebase, extra docs, JD tailoring, cover letter, portfolio analytics, GitLab | More value in each plan |

---

## 7. Open questions
1. Trademark clearance for "Readmely" (given ReadMe.io's registered "README" mark, see §4.5) and securing the domains (.com / .app / .in).
2. Which languages are supported at launch? Suggested: JS/TS, Python, Java, Go; then C#, PHP, Ruby, and Rust.
3. Should the free tier offer instant docs for any public repository? It is not recommended, because that competes head-on with the free DeepWiki and Code Wiki.

## 8. Later roadmap
- **Teams plan:** shared workspaces, organisation repos, roles, and SSO, priced per seat.
- Hosted docs site per project (custom subdomain).
- Bitbucket and self-hosted GitLab.
- VS Code extension and a CLI (`readmely sync`), plus a GitHub Action to refresh docs in CI.
- LinkedIn profile and GitHub profile README generation.
