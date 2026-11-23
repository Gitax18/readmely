//go:build integration

package ratelimit_test

import (
	"context"
	"crypto/rand"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Gitax18/readmely/services/core/internal/ratelimit"
)

// newLimiter connects to REDIS_URL and isolates the test under its own key prefix.
func newLimiter(t *testing.T) (*ratelimit.Limiter, *redis.Client, string) {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Fatal("REDIS_URL is not set: run through `make test-integration`")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal("REDIS_URL could not be parsed")
	}
	rdb := redis.NewClient(opts)
	prefix := "rl-test:" + rand.Text() + ":"
	t.Cleanup(func() {
		ctx := context.Background()
		keys, _ := rdb.Keys(ctx, prefix+"*").Result()
		if len(keys) > 0 {
			_ = rdb.Del(ctx, keys...).Err()
		}
		_ = rdb.Close()
	})
	return ratelimit.New(rdb, prefix), rdb, prefix
}

func TestAllowsUpToTheLimitThenRefuses(t *testing.T) {
	t.Parallel()
	l, _, _ := newLimiter(t)
	ctx := context.Background()
	rule := ratelimit.Rule{Name: "per_hour", Max: 3, Window: time.Hour}
	for i := range 3 {
		d, err := l.Allow(ctx, ratelimit.Check{Rule: rule, Subject: "ada@example.invalid"})
		if err != nil || !d.Allowed {
			t.Fatalf("request %d: %+v %v", i+1, d, err)
		}
	}
	d, err := l.Allow(ctx, ratelimit.Check{Rule: rule, Subject: "ada@example.invalid"})
	if err != nil || d.Allowed || d.RetryAfter < 59*time.Minute || d.RetryAfter > time.Hour {
		t.Fatalf("4th request: %+v %v", d, err)
	}
	if d, _ := l.Allow(ctx, ratelimit.Check{Rule: rule, Subject: "bob@example.invalid"}); !d.Allowed {
		t.Fatal("one subject's limit blocked another")
	}
}

func TestTheWindowEnds(t *testing.T) {
	t.Parallel()
	l, _, _ := newLimiter(t)
	ctx := context.Background()
	rule := ratelimit.Rule{Name: "per_second", Max: 1, Window: time.Second}
	c := ratelimit.Check{Rule: rule, Subject: "x"}
	if d, _ := l.Allow(ctx, c); !d.Allowed {
		t.Fatal("first request refused")
	}
	if d, _ := l.Allow(ctx, c); d.Allowed || d.RetryAfter != time.Second {
		t.Fatalf("second request: %+v", d)
	}
	time.Sleep(1100 * time.Millisecond)
	if d, _ := l.Allow(ctx, c); !d.Allowed {
		t.Fatal("the window did not end")
	}
}

func TestEveryCheckIsCountedAndTheLongestWaitWins(t *testing.T) {
	t.Parallel()
	l, _, _ := newLimiter(t)
	ctx := context.Background()
	minute := ratelimit.Rule{Name: "per_minute", Max: 1, Window: time.Minute}
	hour := ratelimit.Rule{Name: "per_hour", Max: 1, Window: time.Hour}
	both := []ratelimit.Check{{Rule: minute, Subject: "s"}, {Rule: hour, Subject: "s"}}
	if d, _ := l.Allow(ctx, both...); !d.Allowed {
		t.Fatal("first request refused")
	}
	d, err := l.Allow(ctx, both...)
	if err != nil || d.Allowed || d.RetryAfter < 59*time.Minute {
		t.Fatalf("both full: want the hour's wait, got %+v %v", d, err)
	}
}

// The script is atomic: 50 concurrent requests against a limit of 10 let exactly 10 through.
func TestConcurrentRequestsNeverExceedTheLimit(t *testing.T) {
	t.Parallel()
	l, _, _ := newLimiter(t)
	rule := ratelimit.Rule{Name: "burst", Max: 10, Window: time.Minute}
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			d, err := l.Allow(context.Background(), ratelimit.Check{Rule: rule, Subject: "203.0.113.9"})
			if err != nil {
				t.Error(err)
				return
			}
			if d.Allowed {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 10 {
		t.Fatalf("%d requests allowed, want exactly 10", allowed.Load())
	}
}

// A counter that lost its expiry (for example after a manual edit) must not block forever.
func TestAKeyWithoutExpiryIsRepaired(t *testing.T) {
	t.Parallel()
	l, rdb, prefix := newLimiter(t)
	ctx := context.Background()
	rule := ratelimit.Rule{Name: "repair", Max: 1, Window: time.Minute}
	c := ratelimit.Check{Rule: rule, Subject: "s"}
	if _, err := l.Allow(ctx, c); err != nil {
		t.Fatal(err)
	}
	keys, err := rdb.Keys(ctx, prefix+"repair:*").Result()
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys %v %v", keys, err)
	}
	if err := rdb.Persist(ctx, keys[0]).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Allow(ctx, c); err != nil {
		t.Fatal(err)
	}
	if ttl, _ := rdb.PTTL(ctx, keys[0]).Result(); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("the key's expiry was not repaired: %s", ttl)
	}
}

// When Redis is unreachable the caller gets an error and decides; nothing is silently allowed.
func TestRedisDownIsAnError(t *testing.T) {
	t.Parallel()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	d, err := ratelimit.New(rdb, "x:").Allow(context.Background(), ratelimit.Check{Rule: ratelimit.Rule{Name: "x", Max: 1, Window: time.Second}, Subject: "s"})
	if err == nil || d.Allowed {
		t.Fatalf("got %+v %v, want an error and no permission", d, err)
	}
}

// A burst against several rules must still let the first request through. When each rule was
// counted in a separate call, other requests could raise the slower counters before the first one
// reached them, and every request in the burst, the first included, was refused. Repeated, because
// the interleaving is up to the scheduler.
func TestABurstAcrossRulesStillAllowsExactlyOne(t *testing.T) {
	t.Parallel()
	l, _, _ := newLimiter(t)
	ctx := context.Background()
	minute := ratelimit.Rule{Name: "burst_minute", Max: 1, Window: time.Minute}
	hour := ratelimit.Rule{Name: "burst_hour", Max: 5, Window: time.Hour}
	for round := range 150 {
		subject := "burst-" + strconv.Itoa(round)
		var allowed atomic.Int32
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				d, err := l.Allow(ctx,
					ratelimit.Check{Rule: minute, Subject: subject},
					ratelimit.Check{Rule: hour, Subject: subject},
					ratelimit.Check{Rule: ratelimit.Rule{Name: "burst_ip", Max: 20, Window: time.Hour}, Subject: "ip-" + subject + "-" + strconv.Itoa(i)},
				)
				if err != nil {
					t.Error(err)
					return
				}
				if d.Allowed {
					allowed.Add(1)
				}
			}()
		}
		wg.Wait()
		if got := allowed.Load(); got != 1 {
			t.Fatalf("round %d: %d of 20 simultaneous requests were allowed, want exactly 1", round, got)
		}
	}
}
