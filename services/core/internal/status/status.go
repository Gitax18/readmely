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
	"sync"
	"time"

	"github.com/Gitax18/readmely/services/core/internal/contract/ai"
	"github.com/Gitax18/readmely/services/core/internal/contract/coreinternal"
	"github.com/Gitax18/readmely/services/core/internal/httpx"
)

// Per-service states and overall states come from the generated contract
// (contracts/core-internal.yaml), so the compiler enforces the spec.
const (
	StateOK           = coreinternal.ServiceReportStatusOk
	StateDown         = coreinternal.ServiceReportStatusDown
	StateUnreachable  = coreinternal.ServiceReportStatusUnreachable
	StateUnconfigured = coreinternal.ServiceReportStatusUnconfigured

	Operational = coreinternal.StatusReportStatusOperational
	Degraded    = coreinternal.StatusReportStatusDegraded
	Unavailable = coreinternal.StatusReportStatusUnavailable
)

const (
	aiTimeout     = 2 * time.Second
	maxAIBody     = 64 << 10 // 64 KiB
	maxAIChecks   = 8
	apiDBCheckKey = "postgres"
)

var checkName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// Service and Report are the generated response types.
type (
	Service = coreinternal.ServiceReport
	Report  = coreinternal.StatusReport
)

// Checker runs the API's own dependency checks.
type Checker interface {
	RunChecks(ctx context.Context) (coreinternal.CheckMap, bool)
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
		return Service{Status: StateUnconfigured, Checks: coreinternal.CheckMap{}}
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
		return Service{Status: state, Checks: coreinternal.CheckMap{}}
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

	var in ai.Readiness
	if err := json.Unmarshal(raw, &in); err != nil {
		return Service{}, errors.New("response is not the expected JSON")
	}
	if len(in.Checks) > maxAIChecks {
		return Service{}, errors.New("too many checks in response")
	}

	svc := Service{Status: StateOK, Checks: make(coreinternal.CheckMap, len(in.Checks))}
	for name, v := range in.Checks {
		if !checkName.MatchString(name) {
			return Service{}, errors.New("invalid check name in response")
		}
		if v == ai.CheckStateOk {
			svc.Checks[name] = coreinternal.CheckStateOk
		} else {
			svc.Checks[name] = coreinternal.CheckStateDown // anything unknown counts as down
		}
	}
	if resp.StatusCode != http.StatusOK || in.Status != ai.ReadinessStatusReady {
		svc.Status = StateDown
	}
	for _, v := range svc.Checks {
		if v != coreinternal.CheckStateOk {
			svc.Status = StateDown
		}
	}
	return svc, nil
}

// Handler serves GET /api/v1/status.
type Handler struct {
	api Checker
	ai  *AIClient

	// Caching (audit F3): the web app's public home page calls this on every view, and each
	// report pings Postgres, Redis and the AI service. With a ttl, one report is shared for
	// that long, and concurrent callers wait for a single refresh instead of each starting one.
	ttl      time.Duration
	now      func() time.Time
	mu       sync.Mutex
	cached   Report
	cachedAt time.Time
	refresh  chan struct{} // non-nil while a refresh runs; closed when it ends
}

// NewHandler combines the API's own checks with the AI service's. It does not cache.
func NewHandler(api Checker, ai *AIClient) *Handler { return &Handler{api: api, ai: ai, now: time.Now} }

// Cached makes the handler reuse one report for ttl. It returns h.
func (h *Handler) Cached(ttl time.Duration) *Handler {
	h.ttl = ttl
	return h
}

// Get always answers 200: it is a report, and "the AI service is down" is a
// normal thing for it to say.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, h.report(r.Context()))
}

func (h *Handler) report(ctx context.Context) Report {
	if h.ttl <= 0 {
		return h.build(ctx)
	}
	h.mu.Lock()
	if !h.cachedAt.IsZero() && h.now().Sub(h.cachedAt) < h.ttl {
		defer h.mu.Unlock()
		return h.cached
	}
	if wait := h.refresh; wait != nil {
		h.mu.Unlock()
		<-wait // bounded: every probe in build has its own timeout
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.cached
	}
	done := make(chan struct{})
	h.refresh = done
	h.mu.Unlock()

	// The shared refresh must not be cut short because the caller that started it went away.
	rep := h.build(context.WithoutCancel(ctx))

	h.mu.Lock()
	h.cached, h.cachedAt, h.refresh = rep, h.now(), nil
	h.mu.Unlock()
	close(done)
	return rep
}

// build runs both sides in parallel.
func (h *Handler) build(ctx context.Context) Report {
	aiCh := make(chan Service, 1)
	go func() { aiCh <- h.ai.Fetch(ctx) }()

	checks, allOK := h.api.RunChecks(ctx)
	apiSvc := Service{Status: StateOK, Checks: checks}
	if !allOK {
		apiSvc.Status = StateDown
	}
	aiSvc := <-aiCh

	report := Report{Status: overall(apiSvc, aiSvc)}
	report.Services.Api = apiSvc
	report.Services.Ai = aiSvc
	return report
}

// overall: unavailable when the API cannot reach its own database (it cannot
// do its job), operational when everything is ok, otherwise degraded.
func overall(api, aiSvc Service) coreinternal.StatusReportStatus {
	if api.Checks[apiDBCheckKey] != coreinternal.CheckStateOk {
		return Unavailable
	}
	if api.Status == StateOK && aiSvc.Status == StateOK {
		return Operational
	}
	return Degraded
}
