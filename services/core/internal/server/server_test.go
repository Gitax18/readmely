package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Gitax18/readmely/services/core/internal/health"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func newRouter() http.Handler {
	return NewInternalRouter(discardLogger(), health.New(discardLogger(), time.Second), stubStatus)
}

func do(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	newRouter().ServeHTTP(rec, httptest.NewRequest(method, path, http.NoBody))
	return rec
}

func TestRoutes(t *testing.T) {
	if rec := do(t, http.MethodGet, "/healthz"); rec.Code != http.StatusOK {
		t.Errorf("/healthz = %d", rec.Code)
	}
	if rec := do(t, http.MethodGet, "/readyz"); rec.Code != http.StatusOK {
		t.Errorf("/readyz = %d", rec.Code)
	}
}

func TestUnknownRouteReturnsJSON404WithRequestID(t *testing.T) {
	rec := do(t, http.MethodGet, "/nope")
	if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("got %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	var body struct {
		Error     string `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "not_found" || body.RequestID == "" || body.RequestID != rec.Header().Get("X-Request-ID") {
		t.Fatalf("unexpected body %+v", body)
	}
}

func TestWrongMethodReturnsJSON405(t *testing.T) {
	rec := do(t, http.MethodPost, "/healthz")
	if rec.Code != http.StatusMethodNotAllowed || !strings.Contains(rec.Body.String(), "method_not_allowed") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestRouterSetsSecurityHeaders(t *testing.T) {
	rec := do(t, http.MethodGet, "/healthz")
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing")
	}
}

func TestServerTimeoutsAreSet(t *testing.T) {
	s := New(newRouter())
	if s.ReadHeaderTimeout == 0 || s.ReadTimeout == 0 || s.WriteTimeout == 0 || s.IdleTimeout == 0 || s.MaxHeaderBytes == 0 {
		t.Fatalf("a server limit is unset: %+v", s)
	}
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func TestRunStopsCleanlyWhenContextIsCancelled(t *testing.T) {
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, New(newRouter()), ln, time.Second) }()

	resp, err := get(t.Context(), "http://"+ln.Addr().String()+"/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// SIGTERM during a request: the request must finish, then the server exits.
func TestRunFinishesInFlightRequestsBeforeExiting(t *testing.T) {
	started := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, "finished")
	})

	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, New(slow), ln, 2*time.Second) }()

	type result struct {
		body string
		err  error
	}
	res := make(chan result, 1)
	go func() {
		resp, err := get(t.Context(), "http://"+ln.Addr().String()+"/")
		if err != nil {
			res <- result{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		res <- result{body: string(b), err: err}
	}()

	<-started
	cancel() // shutdown begins while the request is still running

	r := <-res
	if r.err != nil || r.body != "finished" {
		t.Fatalf("in-flight request was cut off: %q, %v", r.body, r.err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v", err)
	}
}

func TestRunStopsAcceptingNewConnectionsAfterShutdown(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, New(newRouter()), ln, time.Second) }()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if resp, err := get(t.Context(), "http://"+addr+"/healthz"); err == nil {
		_ = resp.Body.Close()
		t.Fatal("server still accepts requests after shutdown")
	}
}

func TestRunReportsAnErrorWhenShutdownTimesOut(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	stuck := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})

	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, New(stuck), ln, 100*time.Millisecond) }()

	go func() {
		if resp, err := get(t.Context(), "http://"+ln.Addr().String()+"/"); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	cancel()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "shutdown") {
			t.Fatalf("expected a shutdown error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the shutdown timeout")
	}
}

// A connection that is open but has sent no request holds no work and must
// not make a shutdown fail or stall.
func TestRunIgnoresConnectionsThatNeverSentARequest(t *testing.T) {
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, New(newRouter()), ln, 200*time.Millisecond) }()

	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	time.Sleep(50 * time.Millisecond) // let the server accept it

	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("an idle connection turned a clean shutdown into an error: %v", err)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("shutdown took %v", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestRunReturnsServeErrors(t *testing.T) {
	ln := listen(t)
	_ = ln.Close() // serving on a closed listener fails immediately
	err := Run(context.Background(), New(newRouter()), ln, time.Second)
	if err == nil || !strings.Contains(err.Error(), "serve") {
		t.Fatalf("expected a serve error, got %v", err)
	}
}

func TestPublicRouterHasNoReadiness(t *testing.T) {
	r := NewPublicRouter(discardLogger(), health.New(discardLogger(), time.Second))
	for path, want := range map[string]int{"/healthz": http.StatusOK, "/readyz": http.StatusNotFound} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		if rec.Code != want {
			t.Errorf("public %s = %d, want %d", path, rec.Code, want)
		}
	}
}

func stubStatus(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{}")) }

func TestStatusIsInternalOnly(t *testing.T) {
	logger, h := discardLogger(), health.New(discardLogger(), time.Second)
	for name, tc := range map[string]struct {
		router http.Handler
		want   int
	}{
		"public":   {NewPublicRouter(logger, h), http.StatusNotFound},
		"internal": {NewInternalRouter(logger, h, stubStatus), http.StatusOK},
	} {
		rec := httptest.NewRecorder()
		tc.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", http.NoBody))
		if rec.Code != tc.want {
			t.Errorf("%s /api/v1/status = %d, want %d", name, rec.Code, tc.want)
		}
	}
}
