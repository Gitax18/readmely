// Package guard limits every request per client address and blocks addresses that keep misbehaving.
//
// It runs before any handler, on every public request but /healthz:
//
//   - An address may make PerMinute requests a minute across the whole API. Past that it gets 429.
//   - Some answers are strikes: a 429 from any limit, a 403 (a refused origin or CSRF token), a 405,
//     and a 404 for a path the API does not serve at all (probing). A 404 from a real route, such as
//     a deleted project still being polled, is not a strike.
//   - Enough strikes within StrikeWindow block the address: every request gets 429 at once, before
//     any database work. Blocks grow when an address comes back: 15 minutes, then an hour, then a day.
//
// Addresses come from clientip, so visitors behind the web server are told apart, and IPv6 is
// grouped per /64. A request that resolves to one of our own proxies carries no visitor address
// (the web server calling for itself): it is let through uncounted, because counting it would
// put the whole site behind one limit, and a block there would take the whole site down. They are hashed before they become Redis keys. When Redis is unavailable the
// guard lets requests through and says so in the log: losing it must not take the site down,
// while the per-feature limits behind it keep failing closed.
package guard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/Gitax18/readmely/services/core/internal/clientip"
	"github.com/Gitax18/readmely/services/core/internal/httpx"
)

// Config sets the limits. The zero value of PerMinute turns the guard off.
type Config struct {
	PerMinute    int64           // requests per address per minute
	Strikes      int64           // strikes within StrikeWindow that block an address
	StrikeWindow time.Duration   // how long a strike counts
	Blocks       []time.Duration // the first block, the second within a day, the third and later
}

// Defaults are generous on purpose: a busy page polls about every 2 seconds, and offices, campuses
// and mobile carriers put many people behind one address.
func Defaults(perMinute int64) Config {
	return Config{
		PerMinute:    perMinute,
		Strikes:      20,
		StrikeWindow: 10 * time.Minute,
		Blocks:       []time.Duration{15 * time.Minute, time.Hour, 24 * time.Hour},
	}
}

// Guard is the middleware's state.
type Guard struct {
	rdb    redis.Scripter
	ips    *clientip.Resolver
	logger *slog.Logger
	cfg    Config
	prefix string

	lastWarn atomic.Int64 // unix seconds of the last "redis unavailable" warning
}

// New returns a Guard whose Redis keys start with prefix.
func New(rdb redis.Scripter, ips *clientip.Resolver, logger *slog.Logger, cfg Config, prefix string) *Guard {
	return &Guard{rdb: rdb, ips: ips, logger: logger, cfg: cfg, prefix: prefix}
}

// gate answers whether the address is blocked, and otherwise counts the request in its minute.
// Reply: {-1, block ms left} when blocked, else {requests this window, window ms left}.
var gate = redis.NewScript(`
local blocked = redis.call('PTTL', KEYS[1])
if blocked > 0 then
  return {-1, blocked}
end
local n = redis.call('INCR', KEYS[2])
if n == 1 then
  redis.call('PEXPIRE', KEYS[2], ARGV[1])
end
local ttl = redis.call('PTTL', KEYS[2])
if ttl < 0 then
  redis.call('PEXPIRE', KEYS[2], ARGV[1])
  ttl = tonumber(ARGV[1])
end
return {n, ttl}
`)

// strike adds one strike, and blocks the address when there are enough. The block's length grows
// with the number of blocks in the last day. Reply: {0, 0}, or {1, block ms} when it blocked.
var strike = redis.NewScript(`
local s = redis.call('INCR', KEYS[1])
if s == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
if s < tonumber(ARGV[2]) then
  return {0, 0}
end
redis.call('DEL', KEYS[1])
local n = redis.call('INCR', KEYS[3])
if n == 1 then
  redis.call('PEXPIRE', KEYS[3], 86400000)
end
local i = math.min(n, #ARGV - 2)
local ms = tonumber(ARGV[2 + i])
redis.call('SET', KEYS[2], '1', 'PX', ms)
return {1, ms}
`)

func (g *Guard) keys(addrKey string) (block, window, strikes, blocks string) {
	sum := sha256.Sum256([]byte(addrKey))
	h := hex.EncodeToString(sum[:16])
	return g.prefix + "block:" + h, g.prefix + "min:" + h, g.prefix + "strikes:" + h, g.prefix + "blocks:" + h
}

// Middleware applies the guard. It must run inside the router, so it can tell an unknown path
// from a route that answered 404.
func (g *Guard) Middleware(next http.Handler) http.Handler {
	if g == nil || g.cfg.PerMinute <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		addr := g.ips.Addr(r)
		if g.ips.IsProxy(addr) {
			next.ServeHTTP(w, r)
			return
		}
		addrKey := clientip.RateKey(addr)
		block, window, strikes, blocks := g.keys(addrKey)
		res, err := gate.Run(r.Context(), g.rdb, []string{block, window}, time.Minute.Milliseconds()).Int64Slice()
		if err != nil || len(res) != 2 {
			g.warn(r.Context(), err)
			next.ServeHTTP(w, r)
			return
		}
		if res[0] < 0 {
			refuse(w, r, time.Duration(res[1])*time.Millisecond)
			return
		}
		if res[0] > g.cfg.PerMinute {
			refuse(w, r, time.Duration(res[1])*time.Millisecond)
			g.addStrike(r.Context(), addrKey, strikes, block, blocks)
			return
		}
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if isStrike(rec.status, r) {
			g.addStrike(r.Context(), addrKey, strikes, block, blocks)
		}
	})
}

func isStrike(status int, r *http.Request) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusForbidden, http.StatusMethodNotAllowed:
		return true
	case http.StatusNotFound:
		// Only for a path no route matched: a real route's 404 (a deleted project) is normal. A path
		// under a mounted group that matched nothing inside it ends at the mount's "/*".
		rc := chi.RouteContext(r.Context())
		if rc == nil {
			return true
		}
		p := rc.RoutePattern()
		return p == "" || strings.HasSuffix(p, "/*")
	}
	return false
}

func (g *Guard) addStrike(ctx context.Context, addrKey, strikes, block, blocks string) {
	args := make([]any, 0, 2+len(g.cfg.Blocks))
	args = append(args, g.cfg.StrikeWindow.Milliseconds(), g.cfg.Strikes)
	for _, d := range g.cfg.Blocks {
		args = append(args, d.Milliseconds())
	}
	// The request is already answered: count the strike even if the client has gone.
	ctx = context.WithoutCancel(ctx)
	res, err := strike.Run(ctx, g.rdb, []string{strikes, block, blocks}, args...).Int64Slice()
	if err != nil || len(res) != 2 {
		g.warn(ctx, err)
		return
	}
	if res[0] == 1 {
		// The one moment the address itself is logged: it is what an operator needs to act on.
		g.logger.WarnContext(ctx, "address blocked", "address", addrKey,
			"for", (time.Duration(res[1]) * time.Millisecond).String(), "strikes", g.cfg.Strikes)
	}
}

// warn logs a Redis failure at most once a minute, so an outage does not flood the log.
func (g *Guard) warn(ctx context.Context, err error) {
	now := time.Now().Unix()
	last := g.lastWarn.Load()
	if now-last < 60 || !g.lastWarn.CompareAndSwap(last, now) {
		return
	}
	g.logger.WarnContext(ctx, "request guard skipped", "reason", "redis unavailable", "error", err)
}

func refuse(w http.ResponseWriter, r *http.Request, wait time.Duration) {
	secs := max(int((wait+time.Second-1)/time.Second), 1)
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited")
}

// statusRecorder notes the status a handler wrote, passing everything through.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer (flushing, deadlines).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
