//go:build integration

package health_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/Gitax18/readmely/services/core/internal/health"
)

// These tests need the local stack: `make up`, then `make test-integration`.

type redisPinger struct{ c *redis.Client }

func (p redisPinger) Ping(ctx context.Context) error { return p.c.Ping(ctx).Err() }

func requireEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("%s is not set; run through `make test-integration`", key)
	}
	return v
}

func pool(t *testing.T, password string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(requireEnv(t, "DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if password != "" {
		cfg.ConnConfig.Password = password
	}
	p, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func rdb(t *testing.T, password string) *redis.Client {
	t.Helper()
	opts, err := redis.ParseURL(requireEnv(t, "REDIS_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if password != "" {
		opts.Password = password
	}
	c := redis.NewClient(opts)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func readyz(t *testing.T, pg *pgxpool.Pool, rd *redis.Client) (int, map[string]string) {
	t.Helper()
	h := health.New(slog.New(slog.NewJSONHandler(io.Discard, nil)), 2*time.Second,
		health.Check{Name: "postgres", Pinger: pg},
		health.Check{Name: "redis", Pinger: redisPinger{rd}},
	)
	rec := httptest.NewRecorder()
	h.Readyz(rec, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
	var body struct {
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return rec.Code, body.Checks
}

func TestReadyzAgainstTheRealStack(t *testing.T) {
	code, checks := readyz(t, pool(t, ""), rdb(t, ""))
	if code != http.StatusOK || checks["postgres"] != "ok" || checks["redis"] != "ok" {
		t.Fatalf("got %d %v", code, checks)
	}
}

func TestReadyzWithWrongPostgresPassword(t *testing.T) {
	code, checks := readyz(t, pool(t, "definitely-wrong"), rdb(t, ""))
	if code != http.StatusServiceUnavailable || checks["postgres"] != "down" || checks["redis"] != "ok" {
		t.Fatalf("got %d %v", code, checks)
	}
}

func TestReadyzWithWrongRedisPassword(t *testing.T) {
	code, checks := readyz(t, pool(t, ""), rdb(t, "definitely-wrong"))
	if code != http.StatusServiceUnavailable || checks["redis"] != "down" || checks["postgres"] != "ok" {
		t.Fatalf("got %d %v", code, checks)
	}
}
