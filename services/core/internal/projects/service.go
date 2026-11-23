// Package projects holds a user's projects: public GitHub repositories added by address, and the
// analysis jobs that work on them. Every query is scoped to the owner, so someone else's project
// is indistinguishable from one that does not exist.
package projects

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/github"
	"github.com/Gitax18/readmely/services/core/internal/ratelimit"
)

const (
	// MaxProjects per user in version 1.
	MaxProjects = 10
	// MaxRepoKB is the largest repository we accept (GitHub reports size in KB).
	MaxRepoKB = 200_000
	// IngestKind is the job kind that analyses a project.
	IngestKind = "ingest"
	// SummariseKind is the job kind the AI worker runs after an analysis to write the summary.
	SummariseKind = "summarise"
	maxAttempts   = 3
	listLimit     = 50
)

var (
	rulePreview = ratelimit.Rule{Name: "project_preview_user", Max: 30, Window: time.Hour}
	ruleCreate  = ratelimit.Rule{Name: "project_create_user", Max: 10, Window: time.Hour}
	ruleAnalyse = ratelimit.Rule{Name: "project_analyse_user", Max: 20, Window: time.Hour}
)

// Errors the handlers turn into the documented answers.
var (
	ErrInvalidURL   = github.ErrInvalidURL
	ErrRepoNotFound = github.ErrNotFound
	ErrUnavailable  = github.ErrUnavailable
	ErrTooLarge     = errors.New("repository is too large")
	ErrAlreadyAdded = errors.New("repository already added")
	ErrLimit        = errors.New("project limit reached")
	ErrRunning      = errors.New("analysis already running")
	ErrNotFound     = errors.New("project not found")
)

// RateLimitedError carries how long the caller should wait.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "rate limited" }

// Repos looks up a public repository.
type Repos interface {
	Repository(ctx context.Context, ref github.Ref) (github.Repository, error)
}

// Service is the project logic. It has no HTTP in it.
type Service struct {
	pool    *pgxpool.Pool
	q       *store.Queries
	limiter *ratelimit.Limiter
	repos   Repos
	logger  *slog.Logger
}

// NewService wires the service.
func NewService(pool *pgxpool.Pool, limiter *ratelimit.Limiter, repos Repos, logger *slog.Logger) *Service {
	return &Service{pool: pool, q: store.New(pool), limiter: limiter, repos: repos, logger: logger}
}

// View is a project with its latest analysis job (nil before the first analysis).
type View struct {
	Project store.Project
	Job     *store.Job
}

func (s *Service) allow(ctx context.Context, rule ratelimit.Rule, userID uuid.UUID) error {
	d, err := s.limiter.Allow(ctx, ratelimit.Check{Rule: rule, Subject: userID.String()})
	if err != nil {
		return fmt.Errorf("projects: %w", err)
	}
	if !d.Allowed {
		return &RateLimitedError{RetryAfter: d.RetryAfter}
	}
	return nil
}

// lookup parses the address and asks GitHub, enforcing the size limit.
func (s *Service) lookup(ctx context.Context, rawURL string) (github.Repository, error) {
	ref, err := github.ParseURL(rawURL)
	if err != nil {
		return github.Repository{}, ErrInvalidURL
	}
	repo, err := s.repos.Repository(ctx, ref)
	if err != nil {
		return github.Repository{}, err
	}
	if repo.SizeKB > MaxRepoKB {
		return github.Repository{}, ErrTooLarge
	}
	return repo, nil
}

// Preview looks a repository up without creating anything.
func (s *Service) Preview(ctx context.Context, userID uuid.UUID, rawURL string) (github.Repository, error) {
	if err := s.allow(ctx, rulePreview, userID); err != nil {
		return github.Repository{}, err
	}
	return s.lookup(ctx, rawURL)
}

// Create adds the repository as a project (status draft).
func (s *Service) Create(ctx context.Context, userID uuid.UUID, rawURL string) (View, error) {
	if err := s.allow(ctx, ruleCreate, userID); err != nil {
		return View{}, err
	}
	n, err := s.q.CountUserProjects(ctx, userID)
	if err != nil {
		return View{}, fmt.Errorf("create project: %w", err)
	}
	if n >= MaxProjects {
		return View{}, ErrLimit
	}
	repo, err := s.lookup(ctx, rawURL)
	if err != nil {
		return View{}, err
	}
	name := repo.Owner + "/" + repo.Name
	if len(name) > 100 {
		return View{}, ErrInvalidURL
	}
	p, err := s.q.CreateRepositoryProject(ctx, store.CreateRepositoryProjectParams{
		UserID:          userID,
		Name:            name,
		RepoOwner:       &repo.Owner,
		RepoName:        &repo.Name,
		RepoDescription: &repo.Description,
		DefaultBranch:   &repo.DefaultBranch,
		RepoSizeKb:      pgtype.Int4{Int32: int32(repo.SizeKB), Valid: true}, //nolint:gosec // bounded by MaxRepoKB
	})
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return View{}, ErrAlreadyAdded
		}
		return View{}, fmt.Errorf("create project: %w", err)
	}
	return View{Project: p}, nil
}

func (s *Service) latestJob(ctx context.Context, q *store.Queries, p store.Project) (*store.Job, error) {
	j, err := q.GetLatestProjectJob(ctx, store.GetLatestProjectJobParams{ProjectID: &p.ID, UserID: p.UserID, Kind: IngestKind})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no job yet is a normal state, not an error
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// List returns the user's projects, newest first, each with its latest job.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]View, error) {
	rows, err := s.q.ListProjects(ctx, store.ListProjectsParams{UserID: userID, RowLimit: listLimit})
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	out := make([]View, 0, len(rows))
	for _, p := range rows {
		j, err := s.latestJob(ctx, s.q, p)
		if err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		out = append(out, View{Project: p, Job: j})
	}
	return out, nil
}

// Get returns one of the user's projects with its latest job.
func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (View, error) {
	p, err := s.q.GetProject(ctx, store.GetProjectParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, fmt.Errorf("get project: %w", err)
	}
	j, err := s.latestJob(ctx, s.q, p)
	if err != nil {
		return View{}, fmt.Errorf("get project: %w", err)
	}
	return View{Project: p, Job: j}, nil
}

// Delete removes the project and, through the foreign keys, its jobs and everything generated.
func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	n, err := s.q.DeleteProject(ctx, store.DeleteProjectParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Analyse queues an analysis job and marks the project importing. One at a time per project.
func (s *Service) Analyse(ctx context.Context, userID, id uuid.UUID) (View, error) {
	if err := s.allow(ctx, ruleAnalyse, userID); err != nil {
		return View{}, err
	}
	key, err := newKey()
	if err != nil {
		return View{}, err
	}
	var out View
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		p, err := q.GetProject(ctx, store.GetProjectParams{ID: id, UserID: userID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		active, err := q.CountActiveProjectJobs(ctx, store.CountActiveProjectJobsParams{ProjectID: &p.ID, UserID: userID, Kind: IngestKind})
		if err != nil {
			return err
		}
		if active > 0 {
			return ErrRunning
		}
		job, err := q.EnqueueJob(ctx, store.EnqueueJobParams{
			UserID: userID, ProjectID: &p.ID, Kind: IngestKind, Payload: []byte("{}"),
			IdempotencyKey: key, MaxAttempts: maxAttempts,
		})
		if err != nil {
			return err
		}
		if p, err = q.SetProjectStatus(ctx, store.SetProjectStatusParams{ID: p.ID, UserID: userID, Status: "importing"}); err != nil {
			return err
		}
		out = View{Project: p, Job: &job}
		return nil
	})
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrRunning) {
		return View{}, err
	}
	if err != nil {
		return View{}, fmt.Errorf("analyse project: %w", err)
	}
	return out, nil
}

// newKey makes an idempotency key for a request that is deliberately not a replay.
func newKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("projects: %w", err)
	}
	return "ingest-" + hex.EncodeToString(b), nil
}

// Analysis is what the last finished analysis found.
type Analysis struct {
	CommitSHA  string
	AnalysedAt time.Time
	Facts      json.RawMessage
	Hubs       []store.ListProjectHubsRow
	Summary    json.RawMessage // nil until the summary is written
	SummaryJob *store.Job      // nil when no summary was ever queued for this project
}

// Analysis returns the stored facts and the most imported files. Until an analysis has finished,
// and for someone else's project, it is ErrNotFound.
func (s *Service) Analysis(ctx context.Context, userID, id uuid.UUID) (Analysis, error) {
	f, err := s.q.GetProjectFacts(ctx, store.GetProjectFactsParams{ProjectID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Analysis{}, ErrNotFound
	}
	if err != nil {
		return Analysis{}, fmt.Errorf("get analysis: %w", err)
	}
	hubs, err := s.q.ListProjectHubs(ctx, store.ListProjectHubsParams{ProjectID: id, UserID: userID})
	if err != nil {
		return Analysis{}, fmt.Errorf("get analysis: %w", err)
	}
	out := Analysis{CommitSHA: f.CommitSha, AnalysedAt: f.AnalysedAt, Facts: f.Facts, Hubs: hubs}
	switch sum, err := s.q.GetProjectSummary(ctx, store.GetProjectSummaryParams{ProjectID: id, UserID: userID}); {
	case err == nil:
		out.Summary = sum.Summary
	case !errors.Is(err, pgx.ErrNoRows):
		return Analysis{}, fmt.Errorf("get analysis: %w", err)
	}
	switch j, err := s.q.GetLatestProjectJob(ctx, store.GetLatestProjectJobParams{ProjectID: &id, UserID: userID, Kind: SummariseKind}); {
	case err == nil:
		out.SummaryJob = &j
	case !errors.Is(err, pgx.ErrNoRows):
		return Analysis{}, fmt.Errorf("get analysis: %w", err)
	}
	return out, nil
}
