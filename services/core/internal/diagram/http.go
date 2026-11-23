package diagram

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Gitax18/readmely/services/core/internal/auth"
	"github.com/Gitax18/readmely/services/core/internal/contract/corepublic"
	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/httpx"
)

// Handler serves the diagram routes.
type Handler struct{ svc *Service }

// NewHandler returns a Handler over svc.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registers the diagram routes inside the session group.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects/{id}/diagram", h.get)
	r.Post("/projects/{id}/diagram", h.generate)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var rl *RateLimitedError
	switch {
	case errors.As(err, &rl):
		w.Header().Set("Retry-After", strconv.Itoa(max(int(rl.RetryAfter/time.Second), 1)))
		httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited")
	case errors.Is(err, ErrInvalid):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
	case errors.Is(err, ErrInsufficientCredits):
		httpx.WriteError(w, r, http.StatusPaymentRequired, "insufficient_credits")
	case errors.Is(err, ErrNotAnalysed):
		httpx.WriteError(w, r, http.StatusConflict, "not_analysed")
	case errors.Is(err, ErrRunning):
		httpx.WriteError(w, r, http.StatusConflict, "diagram_running")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error")
	}
}

func jobOf(j store.Job) *corepublic.ProjectJob {
	return &corepublic.ProjectJob{
		Id: j.ID, Status: corepublic.ProjectJobStatus(j.Status), Step: j.Step, Progress: int(j.Progress),
		ErrorCode: j.ErrorCode, CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
	}
}

func projectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := projectID(w, r)
	if !ok {
		return
	}
	st, err := h.svc.Get(r.Context(), auth.PrincipalFrom(r).User.ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	type diagram struct {
		Title     string    `json:"title"`
		SVG       string    `json:"svg"`
		NodeCount int32     `json:"node_count"`
		CreatedAt time.Time `json:"created_at"`
	}
	body := struct {
		Diagram *diagram               `json:"diagram"`
		Job     *corepublic.ProjectJob `json:"job"`
	}{}
	if st.Diagram != nil {
		body.Diagram = &diagram{Title: st.Diagram.Title, SVG: st.Diagram.Svg, NodeCount: st.Diagram.NodeCount, CreatedAt: st.Diagram.CreatedAt}
	}
	if st.Job != nil {
		body.Job = jobOf(*st.Job)
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	id, ok := projectID(w, r)
	if !ok {
		return
	}
	var req struct {
		Focus string `json:"focus"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
		return
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
		return
	}
	job, err := h.svc.Generate(r.Context(), auth.PrincipalFrom(r).User.ID, id, req.Focus)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, jobOf(job))
}
