package health

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakePinger func(ctx context.Context) error

func (f fakePinger) Ping(ctx context.Context) error { return f(ctx) }

var (
	up   = fakePinger(func(context.Context) error { return nil })
	down = fakePinger(func(context.Context) error { return errors.New("dial tcp secret-host:5432: connection refused") })
)

func handler(timeout time.Duration, checks ...Check) *Handler {
	return New(slog.New(slog.NewJSONHandler(io.Discard, nil)), timeout, checks...)
}

func readyz(t *testing.T, h *Handler) (int, readyBody, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Readyz(rec, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
	var body readyBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body.String())
	}
	return rec.Code, body, rec.Body.String()
}

func TestHealthzIgnoresDependencies(t *testing.T) {
	h := handler(time.Second, Check{"postgres", down}, Check{"redis", down})
	rec := httptest.NewRecorder()
	h.Healthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestReadyzAllUp(t *testing.T) {
	code, body, _ := readyz(t, handler(time.Second, Check{"postgres", up}, Check{"redis", up}))
	if code != http.StatusOK || body.Status != "ready" || body.Checks["postgres"] != "ok" || body.Checks["redis"] != "ok" {
		t.Fatalf("got %d %+v", code, body)
	}
}

func TestReadyzOneDown(t *testing.T) {
	for _, tc := range []struct{ name, failing string }{{"postgres down", "postgres"}, {"redis down", "redis"}} {
		t.Run(tc.name, func(t *testing.T) {
			pg, rd := Pinger(up), Pinger(up)
			if tc.failing == "postgres" {
				pg = down
			} else {
				rd = down
			}
			code, body, _ := readyz(t, handler(time.Second, Check{"postgres", pg}, Check{"redis", rd}))
			if code != http.StatusServiceUnavailable || body.Status != "unavailable" {
				t.Fatalf("got %d %+v", code, body)
			}
			if body.Checks[tc.failing] != "down" {
				t.Fatalf("%s should be down: %+v", tc.failing, body)
			}
		})
	}
}

func TestReadyzBothDown(t *testing.T) {
	code, body, _ := readyz(t, handler(time.Second, Check{"postgres", down}, Check{"redis", down}))
	if code != http.StatusServiceUnavailable || body.Checks["postgres"] != "down" || body.Checks["redis"] != "down" {
		t.Fatalf("got %d %+v", code, body)
	}
}

func TestReadyzNeverReturnsErrorDetails(t *testing.T) {
	_, _, raw := readyz(t, handler(time.Second, Check{"postgres", down}))
	if strings.Contains(raw, "secret-host") || strings.Contains(raw, "refused") {
		t.Fatalf("response leaks the failure reason: %s", raw)
	}
}

func TestReadyzTimesOutOnAHungDependency(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	// This probe ignores its context entirely, the worst case.
	hung := fakePinger(func(context.Context) error { <-release; return nil })

	start := time.Now()
	code, body, _ := readyz(t, handler(100*time.Millisecond, Check{"postgres", hung}, Check{"redis", up}))
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("readyz took %v despite a 100ms timeout", elapsed)
	}
	if code != http.StatusServiceUnavailable || body.Checks["postgres"] != "down" || body.Checks["redis"] != "ok" {
		t.Fatalf("got %d %+v", code, body)
	}
}

func TestReadyzRunsChecksConcurrently(t *testing.T) {
	slow := fakePinger(func(ctx context.Context) error {
		select {
		case <-time.After(150 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	start := time.Now()
	code, _, _ := readyz(t, handler(time.Second, Check{"a", slow}, Check{"b", slow}, Check{"c", slow}))
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("checks look sequential: %v", elapsed)
	}
}

func TestReadyzWithNoChecksIsReady(t *testing.T) {
	code, body, _ := readyz(t, handler(time.Second))
	if code != http.StatusOK || body.Status != "ready" {
		t.Fatalf("got %d %+v", code, body)
	}
}
