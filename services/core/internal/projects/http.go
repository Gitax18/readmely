package projects

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

// Handler serves the /v1/projects endpoints. It is mounted inside the auth handler's session
// group, so the Origin, content type, session and CSRF checks have already run.
type Handler struct {
	svc *Service
}

// NewHandler builds the HTTP layer.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registers the routes on a router that already requires a session.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/projects/preview", h.preview)
	r.Get("/projects", h.list)
	r.Post("/projects", h.create)
	r.Get("/projects/{id}", h.get)
	r.Delete("/projects/{id}", h.delete)
	r.Post("/projects/{id}/analyse", h.analyse)
	r.Get("/projects/{id}/analysis", h.analysis)
}

func decode(r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return false
	}
	_, err := dec.Token()
	return errors.Is(err, io.EOF)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var rl *RateLimitedError
	switch {
	case errors.As(err, &rl):
		w.Header().Set("Retry-After", strconv.Itoa(max(int(rl.RetryAfter/time.Second), 1)))
		httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited")
	case errors.Is(err, ErrInvalidURL):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, ErrTooLarge):
		httpx.WriteError(w, r, http.StatusBadRequest, "repository_too_large")
	case errors.Is(err, ErrRepoNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "repository_not_found")
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
	case errors.Is(err, ErrUnavailable):
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "github_unavailable")
	case errors.Is(err, ErrAlreadyAdded):
		httpx.WriteError(w, r, http.StatusConflict, "already_added")
	case errors.Is(err, ErrLimit):
		httpx.WriteError(w, r, http.StatusConflict, "project_limit")
	case errors.Is(err, ErrRunning):
		httpx.WriteError(w, r, http.StatusConflict, "analysis_running")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error")
	}
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	var req corepublic.RepositoryUrlRequest
	if !decode(r, &req) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
		return
	}
	repo, err := h.svc.Preview(r.Context(), auth.PrincipalFrom(r).User.ID, req.Url)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, corepublic.RepositoryPreview{
		Owner: repo.Owner, Name: repo.Name, Description: repo.Description,
		DefaultBranch: repo.DefaultBranch, SizeKb: repo.SizeKB,
	})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	views, err := h.svc.List(r.Context(), auth.PrincipalFrom(r).User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]corepublic.Project, 0, len(views))
	for _, v := range views {
		out = append(out, projectBody(v))
	}
	httpx.WriteJSON(w, http.StatusOK, corepublic.ProjectList{Projects: out})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req corepublic.RepositoryUrlRequest
	if !decode(r, &req) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
		return
	}
	v, err := h.svc.Create(r.Context(), auth.PrincipalFrom(r).User.ID, req.Url)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, projectBody(v))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
		return
	}
	v, err := h.svc.Get(r.Context(), auth.PrincipalFrom(r).User.ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, projectBody(v))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
		return
	}
	if err := h.svc.Delete(r.Context(), auth.PrincipalFrom(r).User.ID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) analyse(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
		return
	}
	v, err := h.svc.Analyse(r.Context(), auth.PrincipalFrom(r).User.ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, projectBody(v))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func projectBody(v View) corepublic.Project {
	p := v.Project
	body := corepublic.Project{
		Id: p.ID, Name: p.Name, Owner: deref(p.RepoOwner), Repo: deref(p.RepoName),
		Description: deref(p.RepoDescription), DefaultBranch: deref(p.DefaultBranch),
		SizeKb: int(p.RepoSizeKb.Int32), Status: corepublic.ProjectStatus(p.Status),
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	if v.Job != nil {
		body.Job = jobBody(*v.Job)
	}
	return body
}

func jobBody(j store.Job) *corepublic.ProjectJob {
	return &corepublic.ProjectJob{
		Id: j.ID, Status: corepublic.ProjectJobStatus(j.Status), Step: j.Step, Progress: int(j.Progress),
		ErrorCode: j.ErrorCode, CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
	}
}

type hubBody struct {
	Kind       string `json:"kind"`
	Key        string `json:"key"`
	ImportedBy int    `json:"imported_by"`
}

type jobView = corepublic.ProjectJob

type analysisBody struct {
	CommitSHA  string          `json:"commit_sha"`
	AnalysedAt time.Time       `json:"analysed_at"`
	Facts      json.RawMessage `json:"facts"`
	Summary    json.RawMessage `json:"summary"`
	SummaryJob *jobView        `json:"summary_job"`
	Hubs       []hubBody       `json:"hubs"`
}

func (h *Handler) analysis(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
		return
	}
	a, err := h.svc.Analysis(r.Context(), auth.PrincipalFrom(r).User.ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	hubs := make([]hubBody, 0, len(a.Hubs))
	for _, hub := range a.Hubs {
		hubs = append(hubs, hubBody{Kind: hub.Kind, Key: hub.Key, ImportedBy: int(hub.ImportedBy)})
	}
	body := analysisBody{CommitSHA: a.CommitSHA, AnalysedAt: a.AnalysedAt, Facts: a.Facts, Summary: a.Summary, Hubs: hubs}
	if len(body.Summary) == 0 {
		body.Summary = json.RawMessage("null")
	}
	if a.SummaryJob != nil {
		body.SummaryJob = jobBody(*a.SummaryJob)
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}
