// Package server wires the router and runs the HTTP server with graceful shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Gitax18/readmely/services/core/internal/health"
	"github.com/Gitax18/readmely/services/core/internal/httpx"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// NewPublicRouter is the internet-facing router. It exposes liveness only:
// readiness reveals which dependency is down and costs a database round trip
// per call, so it lives on the internal router (audit item A1). Each mount adds a feature's
// routes (the account endpoints under /v1).
//
// guard, when set, runs before every route: a per-address limit and temporary blocks
// (internal/guard). It sits inside the router, so it can tell an unknown path from a route's 404.
func NewPublicRouter(logger *slog.Logger, h *health.Handler, guard func(http.Handler) http.Handler, mounts ...func(chi.Router)) http.Handler {
	r := newBaseRouter(logger)
	if guard != nil {
		r.Use(guard)
	}
	r.Get("/healthz", h.Healthz)
	for _, mount := range mounts {
		mount(r)
	}
	return r
}

// NewInternalRouter is served on the internal address only (probes, the web
// app's server). It carries everything that reveals dependency state.
func NewInternalRouter(logger *slog.Logger, h *health.Handler, status http.HandlerFunc) http.Handler {
	r := newBaseRouter(logger)
	r.Get("/healthz", h.Healthz)
	r.Get("/readyz", h.Readyz)
	r.Get("/api/v1/status", status)
	return r
}

func newBaseRouter(logger *slog.Logger) chi.Router {
	r := chi.NewRouter()
	r.Use(
		httpx.RequestID,
		httpx.AccessLog(logger),
		httpx.Recover(logger),
		httpx.SecureHeaders,
		httpx.MaxBody(maxBodyBytes),
	)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed")
	})
	return r
}

// New returns an http.Server with timeouts that bound slow clients.
func New(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
}

// Run serves on ln until ctx is cancelled, then stops accepting connections
// and waits up to shutdownTimeout for in-flight requests to finish.
//
// http.Server.Shutdown also waits for connections that are open but have not
// sent a request yet (up to 5 seconds), which browsers create speculatively.
// Those hold no work, so when the timeout passes with no request running they
// are closed and the shutdown still counts as clean. Only requests that were
// actually cut off are reported as an error.
//
// Run wraps srv.Handler, so call it once per server.
func Run(ctx context.Context, srv *http.Server, ln net.Listener, shutdownTimeout time.Duration) error {
	var inFlight atomic.Int64
	next := srv.Handler
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inFlight.Add(1)
		defer inFlight.Add(-1)
		next.ServeHTTP(w, r)
	})

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
		if n := inFlight.Load(); n > 0 {
			return fmt.Errorf("shutdown: %d request(s) still running after %s: %w", n, shutdownTimeout, err)
		}
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
