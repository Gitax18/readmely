//go:build integration

package oauth_test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Gitax18/readmely/services/core/internal/oauth"
)

func newStore(t *testing.T) (*oauth.Store, *redis.Client, string) {
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
	prefix := "oauth-test:" + rand.Text() + ":"
	t.Cleanup(func() {
		ctx := context.Background()
		if keys, _ := rdb.Keys(ctx, prefix+"*").Result(); len(keys) > 0 {
			_ = rdb.Del(ctx, keys...).Err()
		}
		_ = rdb.Close()
	})
	return oauth.NewStore(rdb, prefix), rdb, prefix
}

func TestAnAttemptRoundTrips(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t)
	ctx := context.Background()
	want := oauth.Attempt{Provider: "google", Intent: oauth.IntentLink, Verifier: "v-123", UserID: "u-1", SessionID: "s-1"}
	state, binding, err := s.Begin(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Take(ctx, state, binding)
	if err != nil || *got != want {
		t.Fatalf("take = %+v, %v; want %+v", got, err, want)
	}
}

func TestAStateWorksOnce(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t)
	ctx := context.Background()
	state, binding, _ := s.Begin(ctx, oauth.Attempt{Provider: "github", Intent: oauth.IntentLogin, Verifier: "v"})
	if _, err := s.Take(ctx, state, binding); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Take(ctx, state, binding); !errors.Is(err, oauth.ErrInvalidState) {
		t.Fatalf("second use = %v, want ErrInvalidState", err)
	}
}

func TestOneOfManyRacingCallbacksWins(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t)
	ctx := context.Background()
	state, binding, _ := s.Begin(ctx, oauth.Attempt{Provider: "github", Intent: oauth.IntentLogin, Verifier: "v"})
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Take(ctx, state, binding); err == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d callbacks accepted one state, want 1", wins.Load())
	}
}

// The state alone is not enough: the browser that started the attempt holds the binding.
func TestAnotherBrowserCannotUseTheState(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t)
	ctx := context.Background()
	state, binding, _ := s.Begin(ctx, oauth.Attempt{Provider: "google", Intent: oauth.IntentLogin, Verifier: "v"})
	_, otherBinding, _ := s.Begin(ctx, oauth.Attempt{Provider: "google", Intent: oauth.IntentLogin, Verifier: "v"})

	for name, b := range map[string]string{"another browser": otherBinding, "no cookie": "", "garbage": "x"} {
		if _, err := s.Take(ctx, state, b); !errors.Is(err, oauth.ErrInvalidState) {
			t.Errorf("%s: %v, want ErrInvalidState", name, err)
		}
		// A failed binding spends the attempt, so guessing cannot be retried.
		state, binding, _ = s.Begin(ctx, oauth.Attempt{Provider: "google", Intent: oauth.IntentLogin, Verifier: "v"})
	}
	if _, err := s.Take(ctx, state, binding); err != nil {
		t.Fatalf("the right browser was refused: %v", err)
	}
	// And a spent state cannot be rescued by the right binding afterwards.
	state, binding, _ = s.Begin(ctx, oauth.Attempt{Provider: "google", Intent: oauth.IntentLogin, Verifier: "v"})
	_, _ = s.Take(ctx, state, "wrong")
	if _, err := s.Take(ctx, state, binding); !errors.Is(err, oauth.ErrInvalidState) {
		t.Fatalf("a state survived a wrong-binding attempt: %v", err)
	}
}

func TestUnknownAndOversizedStatesAreRefused(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t)
	ctx := context.Background()
	for _, state := range []string{"", "nope", strings.Repeat("a", 300)} {
		if _, err := s.Take(ctx, state, "b"); !errors.Is(err, oauth.ErrInvalidState) {
			t.Errorf("state %.10q: %v", state, err)
		}
	}
}

func TestAttemptsExpireAndStoreOnlyHashes(t *testing.T) {
	t.Parallel()
	s, rdb, prefix := newStore(t)
	ctx := context.Background()
	state, binding, _ := s.Begin(ctx, oauth.Attempt{Provider: "google", Intent: oauth.IntentLogin, Verifier: "verifier-xyz"})
	keys, _ := rdb.Keys(ctx, prefix+"*").Result()
	if len(keys) != 1 {
		t.Fatalf("keys = %v", keys)
	}
	if ttl, _ := rdb.TTL(ctx, keys[0]).Result(); ttl <= 0 || ttl > oauth.AttemptTTL {
		t.Errorf("ttl = %v, want within (0, %v]", ttl, oauth.AttemptTTL)
	}
	if strings.Contains(keys[0], state) {
		t.Error("the state is in the Redis key")
	}
	raw, _ := rdb.Get(ctx, keys[0]).Result()
	if strings.Contains(raw, binding) || strings.Contains(raw, state) {
		t.Error("the binding or state is stored in the clear")
	}
	// Expiry: shorten the key and wait for it to go.
	_ = rdb.PExpire(ctx, keys[0], 50*time.Millisecond).Err()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := s.Take(ctx, state, binding); errors.Is(err, oauth.ErrInvalidState) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("the attempt did not expire")
}

func TestStatesAreUnique(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t)
	seen := map[string]bool{}
	for range 100 {
		st, b, err := s.Begin(context.Background(), oauth.Attempt{Provider: "google", Intent: oauth.IntentLogin, Verifier: "v"})
		if err != nil || len(st) < 43 || len(b) < 43 || seen[st] || seen[b] {
			t.Fatalf("state %q binding %q: %v", st, b, err)
		}
		seen[st], seen[b] = true, true
	}
}

func TestARedisOutageFailsClosedWithoutLookingLikeABadState(t *testing.T) {
	t.Parallel()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	defer func() { _ = rdb.Close() }()
	s := oauth.NewStore(rdb, "x:")
	if _, _, err := s.Begin(context.Background(), oauth.Attempt{Provider: "google"}); err == nil {
		t.Error("Begin succeeded without Redis")
	}
	if _, err := s.Take(context.Background(), "state", "binding"); err == nil || errors.Is(err, oauth.ErrInvalidState) {
		t.Errorf("Take = %v, want a storage error", err)
	}
}
