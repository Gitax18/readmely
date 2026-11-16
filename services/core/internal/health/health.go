// Package health serves the liveness and readiness endpoints.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Gitax18/readmely/services/core/internal/httpx"
)

// Pinger is anything that can report whether it is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Check is a named dependency probed by /readyz.
type Check struct {
	Name   string
	Pinger Pinger
}

// Handler serves /healthz and /readyz.
type Handler struct {
	logger  *slog.Logger
	timeout time.Duration
	checks  []Check
}

// New returns a Handler that gives each readiness probe at most timeout.
func New(logger *slog.Logger, timeout time.Duration, checks ...Check) *Handler {
	return &Handler{logger: logger, timeout: timeout, checks: checks}
}

// Healthz reports that the process is alive. It never touches a dependency.
func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type readyBody struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Readyz probes every dependency concurrently. The response only says
// "ok" or "down" per dependency; the reason is logged, never returned.
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	results := make([]chan error, len(h.checks))
	for i, c := range h.checks {
		results[i] = make(chan error, 1)
		go func() { results[i] <- probe(ctx, c.Pinger) }()
	}

	body := readyBody{Status: "ready", Checks: make(map[string]string, len(h.checks))}
	status := http.StatusOK
	for i, c := range h.checks {
		if err := <-results[i]; err != nil {
			body.Checks[c.Name] = "down"
			body.Status = "unavailable"
			status = http.StatusServiceUnavailable
			h.logger.WarnContext(r.Context(), "readiness check failed",
				"request_id", httpx.RequestIDFrom(r.Context()),
				"check", c.Name,
				"error", err)
			continue
		}
		body.Checks[c.Name] = "ok"
	}
	httpx.WriteJSON(w, status, body)
}

// probe pings p but gives up when ctx ends, even if p ignores its context.
// Each probe is judged on its own, so a hung dependency cannot turn a
// healthy one into "down". An abandoned ping goroutine ends when the ping
// returns; its result channel is buffered, so it never blocks.
func probe(ctx context.Context, p Pinger) error {
	done := make(chan error, 1)
	go func() { done <- p.Ping(ctx) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
