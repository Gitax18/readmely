// Package status builds the combined status report for the API and the AI
// service. It is served on the internal listener only; the web app is its
// consumer. Failure reasons go to the log, never into the response, and the
// service token never leaves this package except in the request header.
package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Gitax18/readmely/services/core/internal/httpx"
)

// Per-service and per-check states.
const (
	StateOK           = "ok"
	StateDown         = "down"
	StateUnreachable  = "unreachable"
	StateUnconfigured = "unconfigured"
)

// Overall states.
const (
	Operational = "operational"
	Degraded    = "degraded"
	Unavailable = "unavailable"
)

const (
	aiTimeout     = 2 * time.Second
	maxAIBody     = 64 << 10 // 64 KiB
	maxAIChecks   = 8
	apiDBCheckKey = "postgres"
)

var checkName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// Service is the state of one service and its dependencies.
type Service struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Report is the response body of GET /api/v1/status.
type Report struct {
	Status   string             `json:"status"`
	Services map[string]Service `json:"services"`
}

// Checker runs the API's own dependency checks.
type Checker interface {
	RunChecks(ctx context.Context) (map[string]string, bool)
}

// AIClient reads readiness from the AI service.
type AIClient struct {
	url    string
	token  string
	logger *slog.Logger
	http   *http.Client
}

// NewAIClient returns a client for the AI service at baseURL. An empty token
// means "not configured": Fetch reports that and makes no request.
func NewAIClient(logger *slog.Logger, baseURL, token string) *AIClient {
	return &AIClient{
		url:    strings.TrimRight(baseURL, "/") + "/readyz",
		token:  token,
		logger: logger,
		http: &http.Client{
			Timeout: aiTimeout,
			// Never follow redirects: the bearer token must not be sent to
			// any host other than the configured one.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Fetch asks the AI service for its readiness and never returns an error: any
// failure becomes a state in the report.
func (c *AIClient) Fetch(ctx context.Context) Service {
	if c.token == "" {
		return Service{Status: StateUnconfigured, Checks: map[string]string{}}
	}
	svc, err := c.fetch(ctx)
	if err != nil {
		c.logger.WarnContext(ctx, "ai service status check failed",
			"request_id", httpx.RequestIDFrom(ctx), "error", err)
		state := StateDown
		var ue *unreachableError
		if errors.As(err, &ue) {
			state = StateUnreachable
		}
		return Service{Status: state, Checks: map[string]string{}}
	}
	return svc
}

type unreachableError struct{ err error }

func (e *unreachableError) Error() string { return "unreachable: " + e.err.Error() }
func (e *unreachableError) Unwrap() error { return e.err }

func (c *AIClient) fetch(ctx context.Context) (Service, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, http.NoBody)
	if err != nil {
		return Service{}, errors.New("bad request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if id := httpx.RequestIDFrom(ctx); id != "" {
		req.Header.Set(httpx.RequestIDHeader, id)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error embeds the URL, which holds no secret (the token is a header).
		return Service{}, &unreachableError{err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		return Service{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAIBody+1))
	if err != nil {
		return Service{}, &unreachableError{err}
	}
	if len(raw) > maxAIBody {
		return Service{}, errors.New("response too large")
	}

	var in struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return Service{}, errors.New("response is not the expected JSON")
	}
	if len(in.Checks) > maxAIChecks {
		return Service{}, errors.New("too many checks in response")
	}

	svc := Service{Status: StateOK, Checks: make(map[string]string, len(in.Checks))}
	for name, v := range in.Checks {
		if !checkName.MatchString(name) {
			return Service{}, errors.New("invalid check name in response")
		}
		if v == StateOK {
			svc.Checks[name] = StateOK
		} else {
			svc.Checks[name] = StateDown // anything unknown counts as down
		}
	}
	if resp.StatusCode != http.StatusOK || in.Status != "ready" {
		svc.Status = StateDown
	}
	for _, v := range svc.Checks {
		if v != StateOK {
			svc.Status = StateDown
		}
	}
	return svc, nil
}

// Handler serves GET /api/v1/status.
type Handler struct {
	api Checker
	ai  *AIClient
}

// NewHandler combines the API's own checks with the AI service's.
func NewHandler(api Checker, ai *AIClient) *Handler { return &Handler{api: api, ai: ai} }

// Get runs both sides in parallel and always answers 200: it is a report, and
// "the AI service is down" is a normal thing for it to say.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	aiCh := make(chan Service, 1)
	go func() { aiCh <- h.ai.Fetch(r.Context()) }()

	checks, allOK := h.api.RunChecks(r.Context())
	apiSvc := Service{Status: StateOK, Checks: checks}
	if !allOK {
		apiSvc.Status = StateDown
	}
	aiSvc := <-aiCh

	httpx.WriteJSON(w, http.StatusOK, Report{
		Status:   overall(apiSvc, aiSvc),
		Services: map[string]Service{"api": apiSvc, "ai": aiSvc},
	})
}

// overall: unavailable when the API cannot reach its own database (it cannot
// do its job), operational when everything is ok, otherwise degraded.
func overall(api, ai Service) string {
	if api.Checks[apiDBCheckKey] != StateOK {
		return Unavailable
	}
	if api.Status == StateOK && ai.Status == StateOK {
		return Operational
	}
	return Degraded
}
