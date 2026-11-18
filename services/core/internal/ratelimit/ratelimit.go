// Package ratelimit counts requests per subject in fixed windows, in Redis.
//
// One Lua script increments the counter and sets its expiry, so concurrent requests can never
// slip past the limit. Subjects (email addresses, client addresses) are hashed before they become
// keys, so Redis holds no personal data. A fixed window can let up to twice the limit through
// around a window boundary; for sign-in limits that is acceptable and much simpler than a log.
//
// Errors from Redis are returned, not swallowed: callers fail closed, because a limiter that
// waves everything through while Redis is down is no limiter.
package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
)

// Rule is one limit: at most Max requests per Window for one subject.
type Rule struct {
	Name   string // short and stable; part of the key, e.g. "login_start_email_minute"
	Max    int64
	Window time.Duration
}

var ruleName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)

// Validate rejects a rule that would never limit or could not be stored.
func (r Rule) Validate() error {
	switch {
	case !ruleName.MatchString(r.Name):
		return fmt.Errorf("rate limit rule name %q must match %s", r.Name, ruleName)
	case r.Max < 1:
		return fmt.Errorf("rate limit rule %s: Max must be at least 1", r.Name)
	case r.Window < time.Second || r.Window%time.Millisecond != 0:
		return fmt.Errorf("rate limit rule %s: Window must be at least 1s, in whole milliseconds", r.Name)
	}
	return nil
}

// Check is a rule applied to one subject.
type Check struct {
	Rule    Rule
	Subject string
}

// Decision says whether a request may go ahead, and if not, how long to wait.
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration // set when not allowed; whole seconds, at least one
}

// Limiter applies rules using one Redis key per rule, subject and window.
type Limiter struct {
	rdb    redis.Scripter
	prefix string
}

// New returns a Limiter whose keys start with prefix (for example "rl:"), so tests and
// environments sharing one Redis cannot collide.
func New(rdb redis.Scripter, prefix string) *Limiter { return &Limiter{rdb: rdb, prefix: prefix} }

// The counter and its expiry change together. A key that somehow lost its expiry is repaired,
// so it can never block a subject forever.
var incr = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  ttl = tonumber(ARGV[1])
end
return {n, ttl}
`)

func (l *Limiter) key(r Rule, subject string) string {
	sum := sha256.Sum256([]byte(subject))
	return l.prefix + r.Name + ":" + hex.EncodeToString(sum[:16])
}

// Allow counts one request against every check and allows it only if all of them have room.
// Every check is counted even when an earlier one refuses, so hammering one limit cannot be
// used to probe another. RetryAfter is the longest wait among the refusing checks.
func (l *Limiter) Allow(ctx context.Context, checks ...Check) (Decision, error) {
	if len(checks) == 0 {
		return Decision{}, errors.New("ratelimit: no checks")
	}
	d := Decision{Allowed: true}
	for _, c := range checks {
		if err := c.Rule.Validate(); err != nil {
			return Decision{}, err
		}
		res, err := incr.Run(ctx, l.rdb, []string{l.key(c.Rule, c.Subject)}, c.Rule.Window.Milliseconds()).Int64Slice()
		if err != nil {
			return Decision{}, fmt.Errorf("ratelimit %s: %w", c.Rule.Name, err)
		}
		if len(res) != 2 {
			return Decision{}, fmt.Errorf("ratelimit %s: unexpected reply", c.Rule.Name)
		}
		if res[0] > c.Rule.Max {
			d.Allowed = false
			if wait := roundUpToSecond(time.Duration(res[1]) * time.Millisecond); wait > d.RetryAfter {
				d.RetryAfter = wait
			}
		}
	}
	return d, nil
}

func roundUpToSecond(d time.Duration) time.Duration {
	s := (d + time.Second - 1) / time.Second * time.Second
	if s < time.Second {
		return time.Second
	}
	return s
}
