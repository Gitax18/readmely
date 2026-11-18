// Package httpx holds the HTTP building blocks shared by every handler:
// request IDs, JSON errors, and the standard middleware chain pieces.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/Gitax18/readmely/services/core/internal/contract/corepublic"
)

// RequestIDHeader is read from the client when well-formed and always set on responses.
const RequestIDHeader = "X-Request-ID"

type ctxKey struct{}

// Only short, plain identifiers are accepted from clients, so a request ID can
// never carry log-injection or header-injection payloads.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

// RequestID attaches a request ID to the context and the response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if !validRequestID.MatchString(id) {
			id = rand.Text()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

// RequestIDFrom returns the request ID stored by RequestID, or "".
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// WriteJSON writes v as a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a generic error body. code is a stable machine-readable
// string; nothing about the underlying failure is ever sent to the client.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code string) {
	WriteJSON(w, status, corepublic.Error{Error: code, RequestId: RequestIDFrom(r.Context())})
}

// Recover turns a handler panic into a generic 500 and logs the details.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler { //nolint:errorlint // sentinel panic value, not an error chain
					panic(rec)
				}
				logger.ErrorContext(r.Context(), "panic recovered",
					"request_id", RequestIDFrom(r.Context()),
					"panic", fmt.Sprint(rec),
					"stack", string(debug.Stack()))
				WriteError(w, r, http.StatusInternalServerError, "internal_error")
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// LivenessPath is probed every few seconds by container healthchecks. A successful probe is
// logged at debug level so it does not bury real traffic (audit F8); a failing one stays at info.
const LivenessPath = "/healthz"

// AccessLog logs one line per request. The query string is left out on
// purpose, because it can carry tokens.
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			if r.URL.Path == LivenessPath && status < http.StatusBadRequest {
				level = slog.LevelDebug
			}
			logger.LogAttrs(r.Context(), level, "request",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)))
		})
	}
}

// SecureHeaders sets conservative headers suited to a JSON API.
func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// MaxBody caps the size of request bodies. Reads past the limit fail with *http.MaxBytesError.
func MaxBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
