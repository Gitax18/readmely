package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Gitax18/readmely/services/core/internal/contract/corepublic"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func echoID() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, RequestIDFrom(r.Context()))
	})
}

func TestRequestIDGeneratedWhenAbsent(t *testing.T) {
	rec := httptest.NewRecorder()
	RequestID(echoID()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	id := rec.Header().Get(RequestIDHeader)
	if len(id) < 8 || rec.Body.String() != id {
		t.Fatalf("header %q, context %q", id, rec.Body.String())
	}
}

func TestRequestIDAcceptsWellFormedClientValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set(RequestIDHeader, "client-id-12345")
	rec := httptest.NewRecorder()
	RequestID(echoID()).ServeHTTP(rec, req)
	if got := rec.Header().Get(RequestIDHeader); got != "client-id-12345" {
		t.Fatalf("got %q", got)
	}
}

func TestRequestIDRejectsUnsafeClientValues(t *testing.T) {
	for _, bad := range []string{
		"short",
		strings.Repeat("a", 65),
		"has space in it",
		"line\nbreak-injection",
		`<script>alert(1)</script>`,
		"semi;colon-value",
	} {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set(RequestIDHeader, bad)
		rec := httptest.NewRecorder()
		RequestID(echoID()).ServeHTTP(rec, req)
		if got := rec.Header().Get(RequestIDHeader); got == bad || !validRequestID.MatchString(got) {
			t.Errorf("client value %q was not replaced (got %q)", bad, got)
		}
	}
}

func TestRecoverReturnsGenericErrorAndLogsDetails(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := RequestID(Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("db password is hunter2")
	})))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "hunter2") || strings.Contains(rec.Body.String(), "goroutine") {
		t.Fatalf("response leaks panic details: %s", rec.Body.String())
	}
	var body corepublic.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Error != "internal_error" || body.RequestId != rec.Header().Get(RequestIDHeader) || body.RequestId == "" {
		t.Fatalf("unexpected body %+v", body)
	}
	if !strings.Contains(logs.String(), "hunter2") || !strings.Contains(logs.String(), "stack") {
		t.Fatalf("details should be in the log: %s", logs.String())
	}
}

func TestRecoverRepanicsOnAbortHandler(t *testing.T) {
	h := Recover(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if r := recover(); r != http.ErrAbortHandler { //nolint:errorlint // sentinel panic value
			t.Fatalf("expected http.ErrAbortHandler to propagate, got %v", r)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", http.NoBody))
}

func TestAccessLogOmitsQueryString(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := RequestID(AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x?token=abc123", http.NoBody))

	out := logs.String()
	if strings.Contains(out, "abc123") || strings.Contains(out, "token") {
		t.Fatalf("query string leaked into the log: %s", out)
	}
	if !strings.Contains(out, `"status":418`) || !strings.Contains(out, `"path":"/x"`) {
		t.Fatalf("unexpected log line: %s", out)
	}
}

func TestSuccessfulLivenessProbesAreLoggedAtDebug(t *testing.T) {
	for _, tc := range []struct {
		path   string
		status int
		want   string
	}{
		{"/healthz", http.StatusOK, "DEBUG"},
		{"/healthz", http.StatusMethodNotAllowed, "INFO"},
		{"/readyz", http.StatusOK, "INFO"},
		{"/healthz/x", http.StatusOK, "INFO"},
	} {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tc.path, http.NoBody))
		if !strings.Contains(logs.String(), `"level":"`+tc.want+`"`) {
			t.Errorf("%s %d logged as %s, want %s", tc.path, tc.status, logs.String(), tc.want)
		}
	}
}

func TestAccessLogDefaultsStatusTo200(t *testing.T) {
	var logs bytes.Buffer
	h := AccessLog(slog.New(slog.NewJSONHandler(&logs, nil)))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "hi")
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if !strings.Contains(logs.String(), `"status":200`) {
		t.Fatalf("unexpected log line: %s", logs.String())
	}
}

func TestSecureHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	SecureHeaders(echoID()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	for k, v := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Cache-Control":           "no-store",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestMaxBodyRejectsOversizedBodies(t *testing.T) {
	var readErr error
	h := MaxBody(10)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 11))))

	var tooBig *http.MaxBytesError
	if !errors.As(readErr, &tooBig) {
		t.Fatalf("expected *http.MaxBytesError, got %v", readErr)
	}

	readErr = nil
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader("small")))
	if readErr != nil {
		t.Fatalf("a body within the limit failed: %v", readErr)
	}
}
