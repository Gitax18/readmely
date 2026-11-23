package readme

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

// Handler serves the README routes.
type Handler struct{ svc *Service }

// NewHandler returns a Handler over svc.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registers the README routes inside the session group.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects/{id}/readme", h.get)
	r.Post("/projects/{id}/readme", h.generate)
	r.Put("/projects/{id}/readme", h.save)
	r.Post("/projects/{id}/readme/sections", h.section)
	r.Get("/projects/{id}/readme/versions", h.versions)
	r.Post("/projects/{id}/readme/versions/{version}/restore", h.restore)
}

func decode(r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, MaxBytes+4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return false
	}
	_, err := dec.Token()
	return errors.Is(err, io.EOF)
}

func projectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var rl *RateLimitedError
	switch {
	case errors.As(err, &rl):
		w.Header().Set("Retry-After", strconv.Itoa(max(int(rl.RetryAfter/time.Second), 1)))
		httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited")
	case errors.Is(err, ErrInvalid):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrNoSuchSection):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
	case errors.Is(err, ErrInsufficientCredits):
		httpx.WriteError(w, r, http.StatusPaymentRequired, "insufficient_credits")
	case errors.Is(err, ErrNotAnalysed):
		httpx.WriteError(w, r, http.StatusConflict, "not_analysed")
	case errors.Is(err, ErrRunning):
		httpx.WriteError(w, r, http.StatusConflict, "readme_running")
	case errors.Is(err, ErrStaleVersion):
		httpx.WriteError(w, r, http.StatusConflict, "stale_version")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error")
	}
}

type readmeBody struct {
	Version   int32     `json:"version"`
	Content   string    `json:"content"`
	Source    string    `json:"source"`
	Template  *string   `json:"template"`
	CreatedAt time.Time `json:"created_at"`
}

func readmeOf(v store.ReadmeVersion) *readmeBody {
	return &readmeBody{Version: v.Version, Content: v.Content, Source: v.Source, Template: v.Template, CreatedAt: v.CreatedAt}
}

func jobOf(j store.Job) *corepublic.ProjectJob {
	return &corepublic.ProjectJob{
		Id: j.ID, Status: corepublic.ProjectJobStatus(j.Status), Step: j.Step, Progress: int(j.Progress),
		ErrorCode: j.ErrorCode, CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
	}
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
	body := struct {
		Readme *readmeBody            `json:"readme"`
		Score  *Score                 `json:"score"`
		Job    *corepublic.ProjectJob `json:"job"`
	}{Score: st.Score}
	if st.Readme != nil {
		body.Readme = readmeOf(*st.Readme)
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
		Template string `json:"template"`
	}
	if !decode(r, &req) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
		return
	}
	job, err := h.svc.Generate(r.Context(), auth.PrincipalFrom(r).User.ID, id, req.Template)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, jobOf(job))
}

func (h *Handler) section(w http.ResponseWriter, r *http.Request) {
	id, ok := projectID(w, r)
	if !ok {
		return
	}
	var req struct {
		Heading     string `json:"heading"`
		Instruction string `json:"instruction"`
	}
	if !decode(r, &req) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
		return
	}
	job, err := h.svc.RegenerateSection(r.Context(), auth.PrincipalFrom(r).User.ID, id, req.Heading, req.Instruction)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, jobOf(job))
}

func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	id, ok := projectID(w, r)
	if !ok {
		return
	}
	var req struct {
		Content     string `json:"content"`
		BaseVersion int    `json:"base_version"`
	}
	if !decode(r, &req) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
		return
	}
	v, err := h.svc.Save(r.Context(), auth.PrincipalFrom(r).User.ID, id, req.Content, req.BaseVersion)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, readmeOf(v))
}

func (h *Handler) versions(w http.ResponseWriter, r *http.Request) {
	id, ok := projectID(w, r)
	if !ok {
		return
	}
	rows, err := h.svc.Versions(r.Context(), auth.PrincipalFrom(r).User.ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	type info struct {
		Version   int32     `json:"version"`
		Source    string    `json:"source"`
		Template  *string   `json:"template"`
		CreatedAt time.Time `json:"created_at"`
	}
	out := make([]info, 0, len(rows))
	for _, v := range rows {
		out = append(out, info{Version: v.Version, Source: v.Source, Template: v.Template, CreatedAt: v.CreatedAt})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string][]info{"versions": out})
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	id, ok := projectID(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || n < 1 || n > 1<<30 {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
		return
	}
	v, err := h.svc.Restore(r.Context(), auth.PrincipalFrom(r).User.ID, id, n)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, readmeOf(v))
}
