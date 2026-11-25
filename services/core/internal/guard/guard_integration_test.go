//go:build integration

package guard_test

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/Gitax18/readmely/services/core/internal/clientip"
	"github.com/Gitax18/readmely/services/core/internal/guard"
)

// The web server's address, trusted to say who the visitor is.
var proxy = netip.MustParsePrefix("10.0.0.0/8")

type fixture struct {
	t      *testing.T
	router http.Handler
	rdb    *redis.Client
	prefix string
}

func newFixture(t *testing.T, cfg guard.Config) *fixture {
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
	prefix := "guard-test:" + rand.Text() + ":"
	t.Cleanup(func() {
		ctx := context.Background()
		keys, _ := rdb.Keys(ctx, prefix+"*").Result()
		if len(keys) > 0 {
			_ = rdb.Del(ctx, keys...).Err()
		}
		_ = rdb.Close()
	})
	g := guard.New(rdb, clientip.New([]netip.Prefix{proxy}), slog.New(slog.NewTextHandler(io.Discard, nil)), cfg, prefix)
	r := chi.NewRouter()
	r.Use(g.Middleware)
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	r.Get("/healthz", ok)
	// Mounted the way the API mounts its routes, so unknown paths inside the group are tested too.
	r.Route("/v1", func(r chi.Router) {
		r.Get("/me", ok)
		r.Get("/projects/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
		r.Post("/forbidden", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	})
	return &fixture{t: t, router: r, rdb: rdb, prefix: prefix}
}

// do sends a request as the visitor `client`, through the web server.
func (f *fixture) do(method, path, client string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "10.1.2.3:4567"
	req.Header.Set("X-Forwarded-For", client)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func small() guard.Config {
	return guard.Config{PerMinute: 5, Strikes: 3, StrikeWindow: time.Minute, Blocks: []time.Duration{2 * time.Second, 4 * time.Second}}
}

func TestEachAddressGetsItsOwnShareThenA429(t *testing.T) {
	t.Parallel()
	f := newFixture(t, small())
	for i := range 5 {
		if rec := f.do("GET", "/v1/me", "203.0.113.7"); rec.Code != http.StatusOK {
			t.Fatalf("request %d = %d, want 200", i+1, rec.Code)
		}
	}
	rec := f.do("GET", "/v1/me", "203.0.113.7")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("over the limit = %d, want 429", rec.Code)
	}
	if s, err := strconv.Atoi(rec.Header().Get("Retry-After")); err != nil || s < 1 || s > 60 {
		t.Errorf("Retry-After = %q", rec.Header().Get("Retry-After"))
	}
	// Another visitor behind the same web server is counted apart.
	if rec := f.do("GET", "/v1/me", "198.51.100.9"); rec.Code != http.StatusOK {
		t.Errorf("another visitor = %d, want 200", rec.Code)
	}
}

func TestHealthIsNeverLimited(t *testing.T) {
	t.Parallel()
	f := newFixture(t, small())
	for range 20 {
		if rec := f.do("GET", "/healthz", "203.0.113.8"); rec.Code != http.StatusOK {
			t.Fatalf("/healthz = %d", rec.Code)
		}
	}
}

func TestRepeatedRefusalsBlockTheAddressAndBlocksGrow(t *testing.T) {
	t.Parallel()
	f := newFixture(t, small())
	visitor := "203.0.113.9"
	for _, path := range []string{"/wp-login.php", "/v1/admin", "/.env"} { // probing: three strikes
		if rec := f.do("GET", path, visitor); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown path %s = %d, want 404", path, rec.Code)
		}
	}
	rec := f.do("GET", "/v1/me", visitor)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("a blocked address = %d, want 429", rec.Code)
	}
	if s, _ := strconv.Atoi(rec.Header().Get("Retry-After")); s < 1 || s > 2 {
		t.Errorf("first block Retry-After = %d, want 1 to 2", s)
	}
	time.Sleep(2100 * time.Millisecond)
	if rec := f.do("GET", "/v1/me", visitor); rec.Code != http.StatusOK {
		t.Fatalf("after the block = %d, want 200", rec.Code)
	}
	for range 3 {
		f.do("POST", "/v1/forbidden", visitor) // refused origins are strikes too
	}
	rec = f.do("GET", "/v1/me", visitor)
	if s, _ := strconv.Atoi(rec.Header().Get("Retry-After")); rec.Code != http.StatusTooManyRequests || s < 3 {
		t.Fatalf("second block = %d, Retry-After %d; want 429 and about 4", rec.Code, s)
	}
}

func TestARoutesOwn404IsNotAStrike(t *testing.T) {
	t.Parallel()
	f := newFixture(t, guard.Config{PerMinute: 100, Strikes: 3, StrikeWindow: time.Minute, Blocks: []time.Duration{time.Minute}})
	for range 10 { // polling a deleted project
		if rec := f.do("GET", "/v1/projects/0190", "203.0.113.10"); rec.Code != http.StatusNotFound {
			t.Fatalf("deleted project = %d", rec.Code)
		}
	}
	if rec := f.do("GET", "/v1/me", "203.0.113.10"); rec.Code != http.StatusOK {
		t.Fatalf("after polling a deleted project = %d, want 200", rec.Code)
	}
}

func TestAnAddressCannotPretendToBeSomeoneElse(t *testing.T) {
	t.Parallel()
	f := newFixture(t, small())
	// Straight from the internet, not through the web server: X-Forwarded-For is ignored, so
	// changing it does not reset the limit.
	for i := range 7 {
		req := httptest.NewRequest("GET", "/v1/me", nil)
		req.RemoteAddr = "192.0.2.50:1000"
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i))
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		if i >= 5 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d with a forged header = %d, want 429", i+1, rec.Code)
		}
	}
}

func TestTheWebServerCallingForItselfIsNeverCountedOrBlocked(t *testing.T) {
	t.Parallel()
	f := newFixture(t, small())
	// No visitor address: the request resolves to the web server itself.
	for i := range 30 {
		req := httptest.NewRequest("GET", "/nowhere", nil)
		req.RemoteAddr = "10.1.2.3:4567"
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("request %d = %d, want 404 every time (never 429)", i+1, rec.Code)
		}
	}
	keys, err := f.rdb.Keys(context.Background(), f.prefix+"*").Result()
	if err != nil || len(keys) != 0 {
		t.Fatalf("the web server's own calls left keys: %v %v", keys, err)
	}
}

func TestWithoutRedisRequestsStillGoThrough(t *testing.T) {
	t.Parallel()
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = dead.Close() })
	g := guard.New(dead, clientip.New(nil), slog.New(slog.NewTextHandler(io.Discard, nil)), small(), "x:")
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/me", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("with Redis down = %d, want 200 (the guard fails open)", rec.Code)
	}
}

func TestZeroPerMinuteTurnsTheGuardOff(t *testing.T) {
	t.Parallel()
	g := guard.New(nil, clientip.New(nil), slog.New(slog.NewTextHandler(io.Discard, nil)), guard.Defaults(0), "x:")
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	rec := httptest.NewRecorder()
	g.Middleware(next).ServeHTTP(rec, httptest.NewRequest("GET", "/v1/me", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("an off guard changed the answer: %d", rec.Code)
	}
}
