package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer lets the test read logs that the server goroutine is writing.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// Nothing listens on port 1, so both dependencies are unreachable.
func deadDeps() map[string]string {
	return map[string]string{
		"DATABASE_URL":     "postgres://u:secretpw@127.0.0.1:1/db?sslmode=disable&connect_timeout=1",
		"REDIS_URL":        "redis://:secretpw@127.0.0.1:1/0",
		"SHUTDOWN_TIMEOUT": "2s",
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func getJSON(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	return resp.StatusCode, body
}

func TestMissingConfigRefusesToStart(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), nil, env(map[string]string{}), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	for _, key := range []string{"DATABASE_URL", "REDIS_URL"} {
		if !strings.Contains(stderr.String(), key) {
			t.Errorf("stderr should name %s: %s", key, stderr.String())
		}
	}
}

func TestPortInUseFailsClearly(t *testing.T) {
	busy, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()

	m := deadDeps()
	m["HTTP_ADDR"] = busy.Addr().String()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), nil, env(m), &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "cannot listen") || !strings.Contains(stdout.String(), busy.Addr().String()) {
		t.Fatalf("log should say which address failed: %s", stdout.String())
	}
}

// Scenario: Postgres and Redis are down. The API must stay up,
// /healthz must be 200 and /readyz must be 503 with both marked down.
func TestAPIStaysUpWhenDependenciesAreDown(t *testing.T) {
	addr := freeAddr(t)
	m := deadDeps()
	m["HTTP_ADDR"] = addr

	var stdout, stderr syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- run(ctx, nil, env(m), &stdout, &stderr) }()

	base := "http://" + addr
	deadline := time.Now().Add(5 * time.Second)
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", http.NoBody)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("API did not start: %s", stdout.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	if code, _ := getJSON(t, base+"/healthz"); code != http.StatusOK {
		t.Errorf("/healthz = %d, want 200", code)
	}
	code, body := getJSON(t, base+"/readyz")
	if code != http.StatusServiceUnavailable {
		t.Errorf("/readyz = %d, want 503", code)
	}
	checks, _ := body["checks"].(map[string]any)
	if checks["postgres"] != "down" || checks["redis"] != "down" {
		t.Errorf("unexpected checks: %v", body)
	}

	cancel()
	select {
	case c := <-done:
		if c != 0 {
			t.Errorf("exit code %d, want 0\nlogs: %s", c, stdout.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("API did not stop after cancellation")
	}

	if strings.Contains(stdout.String(), "secretpw") || strings.Contains(stderr.String(), "secretpw") {
		t.Errorf("a password reached the logs: %s", stdout.String())
	}
}

func TestHealthcheckSubcommand(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer ok.Close()
	_, port, _ := net.SplitHostPort(ok.Listener.Addr().String())

	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"healthcheck"}, env(map[string]string{"HTTP_ADDR": ":" + port}), io.Discard, &stderr); code != 0 {
		t.Fatalf("healthy server: exit %d, %s", code, stderr.String())
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer bad.Close()
	_, badPort, _ := net.SplitHostPort(bad.Listener.Addr().String())
	if code := run(context.Background(), []string{"healthcheck"}, env(map[string]string{"HTTP_ADDR": ":" + badPort}), io.Discard, io.Discard); code != 1 {
		t.Fatalf("unhealthy server: exit %d, want 1", code)
	}

	closed := freeAddr(t)
	_, closedPort, _ := net.SplitHostPort(closed)
	if code := run(context.Background(), []string{"healthcheck"}, env(map[string]string{"HTTP_ADDR": ":" + closedPort}), io.Discard, io.Discard); code != 1 {
		t.Fatalf("nothing listening: exit %d, want 1", code)
	}

	if code := run(context.Background(), []string{"healthcheck"}, env(map[string]string{"HTTP_ADDR": "nonsense"}), io.Discard, io.Discard); code != 1 {
		t.Fatalf("bad HTTP_ADDR: exit %d, want 1", code)
	}
}
