package status

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gitax18/readmely/services/core/internal/contract/coreinternal"
	"github.com/Gitax18/readmely/services/core/internal/httpx"
)

var testToken = strings.Repeat("t0k3n", 7) // 35 chars

type fakeChecker struct {
	checks coreinternal.CheckMap
	ok     bool
}

func (f fakeChecker) RunChecks(context.Context) (coreinternal.CheckMap, bool) { return f.checks, f.ok }

func healthyAPI() fakeChecker {
	return fakeChecker{coreinternal.CheckMap{"postgres": "ok", "redis": "ok"}, true}
}

func logTo(buf *bytes.Buffer) *slog.Logger { return slog.New(slog.NewJSONHandler(buf, nil)) }

// fakeAI serves handler and returns a handler wired to it.
func newHandler(t *testing.T, api Checker, ai http.Handler, token string, logs *bytes.Buffer) (*Handler, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(ai)
	t.Cleanup(srv.Close)
	return NewHandler(api, NewAIClient(logTo(logs), srv.URL, token)), srv
}

func aiReply(code int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" || r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"unauthorized","request_id":"x"}`)
			return
		}
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	})
}

func get(t *testing.T, h *Handler) (int, Report, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", http.NoBody))
	var r Report
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("not JSON: %v: %s", err, rec.Body.String())
	}
	return rec.Code, r, rec.Body.String()
}

func TestAllOK(t *testing.T) {
	var logs bytes.Buffer
	h, _ := newHandler(t, healthyAPI(), aiReply(200, `{"status":"ready","checks":{"postgres":"ok"}}`), testToken, &logs)
	code, r, _ := get(t, h)
	if code != 200 || r.Status != Operational || r.Services.Api.Status != StateOK || r.Services.Ai.Status != StateOK {
		t.Fatalf("got %d %+v", code, r)
	}
	if r.Services.Ai.Checks["postgres"] != "ok" || r.Services.Api.Checks["redis"] != "ok" {
		t.Fatalf("detail missing: %+v", r)
	}
}

func TestAIReportsDownWithDetail(t *testing.T) {
	var logs bytes.Buffer
	h, _ := newHandler(t, healthyAPI(), aiReply(503, `{"status":"unavailable","checks":{"postgres":"down"}}`), testToken, &logs)
	code, r, _ := get(t, h)
	ai := r.Services.Ai
	if code != 200 || r.Status != Degraded || ai.Status != StateDown || ai.Checks["postgres"] != "down" {
		t.Fatalf("got %d %+v", code, r)
	}
}

func TestAPIPostgresDownIsUnavailable(t *testing.T) {
	var logs bytes.Buffer
	api := fakeChecker{coreinternal.CheckMap{"postgres": "down", "redis": "ok"}, false}
	h, _ := newHandler(t, api, aiReply(200, `{"status":"ready","checks":{"postgres":"ok"}}`), testToken, &logs)
	_, r, _ := get(t, h)
	if r.Status != Unavailable || r.Services.Api.Status != StateDown {
		t.Fatalf("got %+v", r)
	}
}

func TestAPIRedisDownIsDegraded(t *testing.T) {
	var logs bytes.Buffer
	api := fakeChecker{coreinternal.CheckMap{"postgres": "ok", "redis": "down"}, false}
	h, _ := newHandler(t, api, aiReply(200, `{"status":"ready","checks":{"postgres":"ok"}}`), testToken, &logs)
	if _, r, _ := get(t, h); r.Status != Degraded {
		t.Fatalf("got %+v", r)
	}
}

func TestAIHangsAndTheReportStillArrivesInTime(t *testing.T) {
	var logs bytes.Buffer
	release := make(chan struct{})
	hang := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release })
	h, _ := newHandler(t, healthyAPI(), hang, testToken, &logs)
	// Registered after the server's own cleanup, so it runs first (LIFO) and
	// frees the blocked handler before the server waits for it.
	t.Cleanup(func() { close(release) })
	start := time.Now()
	_, r, _ := get(t, h)
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("took %s, want under 3s", took)
	}
	if r.Services.Ai.Status != StateUnreachable || r.Status != Degraded {
		t.Fatalf("got %+v", r)
	}
}

func TestAIConnectionRefusedIsUnreachable(t *testing.T) {
	var logs bytes.Buffer
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	h := NewHandler(healthyAPI(), NewAIClient(logTo(&logs), url, testToken))
	if _, r, _ := get(t, h); r.Services.Ai.Status != StateUnreachable {
		t.Fatalf("got %+v", r)
	}
}

func TestWrongTokenHidesTheReason(t *testing.T) {
	var logs bytes.Buffer
	other := strings.Repeat("x", 40)
	h, _ := newHandler(t, healthyAPI(), aiReply(200, `{}`), other, &logs)
	_, r, body := get(t, h)
	if r.Services.Ai.Status != StateDown {
		t.Fatalf("got %+v", r)
	}
	for _, leak := range []string{"401", "unauthorized", "token", other} {
		if strings.Contains(strings.ToLower(body), leak) {
			t.Errorf("response leaks %q: %s", leak, body)
		}
	}
	if !strings.Contains(logs.String(), "ai service status check failed") {
		t.Errorf("failure was not logged: %s", logs.String())
	}
}

func TestRedirectIsNotFollowedAndTokenNotForwarded(t *testing.T) {
	var logs bytes.Buffer
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(200)
	}))
	t.Cleanup(target.Close)
	redirect := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/readyz", http.StatusFound)
	})
	h, _ := newHandler(t, healthyAPI(), redirect, testToken, &logs)
	_, r, _ := get(t, h)
	if hits.Load() != 0 {
		t.Fatalf("redirect target was contacted %d time(s)", hits.Load())
	}
	if r.Services.Ai.Status != StateDown {
		t.Fatalf("got %+v", r)
	}
}

func TestBadAIResponsesAreDownNotErrors(t *testing.T) {
	for name, body := range map[string]string{
		"not json":         `<html>oops</html>`,
		"wrong shape":      `[1,2,3]`,
		"too many checks":  `{"status":"ready","checks":{"a":"ok","b":"ok","c":"ok","d":"ok","e":"ok","f":"ok","g":"ok","h":"ok","i":"ok"}}`,
		"bad check name":   `{"status":"ready","checks":{"<script>":"ok"}}`,
		"too large":        `{"status":"ready","checks":{"postgres":"ok"},"pad":"` + strings.Repeat("a", 70<<10) + `"}`,
		"status not ready": `{"status":"???","checks":{"postgres":"ok"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			h, _ := newHandler(t, healthyAPI(), aiReply(200, body), testToken, &logs)
			code, r, _ := get(t, h)
			if code != 200 || r.Services.Ai.Status != StateDown {
				t.Fatalf("got %d %+v", code, r)
			}
		})
	}
}

func TestUnknownCheckValuesCountAsDown(t *testing.T) {
	var logs bytes.Buffer
	h, _ := newHandler(t, healthyAPI(), aiReply(200, `{"status":"ready","checks":{"postgres":"maybe"}}`), testToken, &logs)
	_, r, _ := get(t, h)
	ai := r.Services.Ai
	if ai.Status != StateDown || ai.Checks["postgres"] != coreinternal.CheckStateDown {
		t.Fatalf("got %+v", ai)
	}
}

func TestUnexpectedStatusCodeIsDown(t *testing.T) {
	var logs bytes.Buffer
	h, _ := newHandler(t, healthyAPI(), aiReply(500, `{"status":"ready","checks":{"postgres":"ok"}}`), testToken, &logs)
	if _, r, _ := get(t, h); r.Services.Ai.Status != StateDown {
		t.Fatalf("got %+v", r)
	}
}

func TestNoTokenMeansUnconfiguredAndNoRequest(t *testing.T) {
	var logs bytes.Buffer
	var hits atomic.Int32
	counter := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) })
	h, _ := newHandler(t, healthyAPI(), counter, "", &logs)
	_, r, _ := get(t, h)
	if hits.Load() != 0 || r.Services.Ai.Status != StateUnconfigured || r.Status != Degraded {
		t.Fatalf("hits=%d %+v", hits.Load(), r)
	}
	if r.Services.Ai.Checks == nil {
		t.Fatal("checks must be an object, not null")
	}
}

func TestTokenNeverAppearsInResponseOrLogs(t *testing.T) {
	var logs bytes.Buffer
	for _, ai := range []http.Handler{
		aiReply(200, `{"status":"ready","checks":{"postgres":"ok"}}`),
		aiReply(502, `boom `+testToken),
	} {
		h, _ := newHandler(t, healthyAPI(), ai, testToken, &logs)
		_, _, body := get(t, h)
		if strings.Contains(body, testToken) {
			t.Fatalf("response contains the token: %s", body)
		}
	}
	if strings.Contains(logs.String(), testToken) {
		t.Fatalf("logs contain the token: %s", logs.String())
	}
}

func TestRequestIDIsForwardedToTheAIService(t *testing.T) {
	var logs bytes.Buffer
	var got atomic.Value
	spy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("X-Request-ID"))
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"status":"ready","checks":{"postgres":"ok"}}`)
	})
	h, _ := newHandler(t, healthyAPI(), spy, testToken, &logs)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", http.NoBody)
	req.Header.Set("X-Request-ID", "client-id-12345")
	httpx.RequestID(http.HandlerFunc(h.Get)).ServeHTTP(httptest.NewRecorder(), req)
	if v, _ := got.Load().(string); v != "client-id-12345" {
		t.Fatalf("AI service saw request id %q", v)
	}
}

// countingChecker counts how often the API's own checks run, and can be slow.
type countingChecker struct {
	runs  atomic.Int64
	delay time.Duration
}

func (c *countingChecker) RunChecks(context.Context) (coreinternal.CheckMap, bool) {
	c.runs.Add(1)
	time.Sleep(c.delay)
	return coreinternal.CheckMap{"postgres": "ok", "redis": "ok"}, true
}

// Audit F3: a burst of page views costs one round of probes, and the cache expires.
func TestCachedReportIsSharedAndRefreshedOnce(t *testing.T) {
	var aiCalls atomic.Int64
	api := &countingChecker{delay: 50 * time.Millisecond}
	var logs bytes.Buffer
	ai := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aiCalls.Add(1)
		aiReply(http.StatusOK, `{"status":"ready","checks":{"postgres":"ok"}}`).ServeHTTP(w, r)
	})
	h, _ := newHandler(t, api, ai, testToken, &logs)
	now := time.Unix(1_000_000, 0)
	var clock atomic.Int64
	clock.Store(now.UnixNano())
	h.now = func() time.Time { return time.Unix(0, clock.Load()) }
	h.Cached(5 * time.Second)

	done := make(chan Report, 100)
	for range 100 {
		go func() { _, r, _ := get(t, h); done <- r }()
	}
	for range 100 {
		if r := <-done; r.Status != Operational {
			t.Fatalf("a waiting caller got %q", r.Status)
		}
	}
	if api.runs.Load() != 1 || aiCalls.Load() != 1 {
		t.Fatalf("100 concurrent views ran the checks %d times and called the AI service %d times, want 1 and 1", api.runs.Load(), aiCalls.Load())
	}

	clock.Add(int64(4 * time.Second))
	get(t, h)
	if api.runs.Load() != 1 {
		t.Fatal("the report was rebuilt before the ttl ran out")
	}
	clock.Add(int64(2 * time.Second))
	get(t, h)
	if api.runs.Load() != 2 || aiCalls.Load() != 2 {
		t.Fatalf("after the ttl the report must be rebuilt once: checks %d, ai %d", api.runs.Load(), aiCalls.Load())
	}
}

// A caller that gives up does not cancel the refresh everyone else is waiting for.
func TestCachedRefreshSurvivesTheFirstCallerLeaving(t *testing.T) {
	api := &countingChecker{delay: 100 * time.Millisecond}
	var logs bytes.Buffer
	h, _ := newHandler(t, api, aiReply(http.StatusOK, `{"status":"ready","checks":{"postgres":"ok"}}`), testToken, &logs)
	h.Cached(time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", http.NoBody).WithContext(ctx))
	var r Report
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Services.Ai.Status != StateOK {
		t.Fatalf("the AI check was cut short by the first caller's cancellation: %q", r.Services.Ai.Status)
	}
}
