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
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gitax18/readmely/services/core/internal/contextpack"
	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/github"
	"github.com/Gitax18/readmely/services/core/internal/githubaccess"
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
	ruleRepos   = ratelimit.Rule{Name: "github_repos_user", Max: 30, Window: 10 * time.Minute}
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
	// ErrAccessRequired means the person has not granted GitHub access, or GitHub no longer accepts it.
	ErrAccessRequired = errors.New("github access required")
)

// RateLimitedError carries how long the caller should wait.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "rate limited" }

// Repos looks up a public repository.
type Repos interface {
	Repository(ctx context.Context, ref github.Ref) (github.Repository, error)
}

// UserRepos reads repositories as one person, with the token they granted.
type UserRepos interface {
	Repos
	ListRepositories(ctx context.Context) ([]github.Repository, error)
}

// Service is the project logic. It has no HTTP in it.
type Service struct {
	pool    *pgxpool.Pool
	q       *store.Queries
	limiter *ratelimit.Limiter
	repos   Repos
	logger  *slog.Logger

	// Private repositories: the stored tokens and a way to act as a person. Both nil when off.
	access  *githubaccess.Store
	forUser func(token string) UserRepos
}

// NewService wires the service.
func NewService(pool *pgxpool.Pool, limiter *ratelimit.Limiter, repos Repos, logger *slog.Logger) *Service {
	return &Service{pool: pool, q: store.New(pool), limiter: limiter, repos: repos, logger: logger}
}

// EnablePrivate lets people add their private repositories: the person's stored token is used
// when a repository is not public, and to list what they can see.
func (s *Service) EnablePrivate(access *githubaccess.Store, forUser func(token string) UserRepos) {
	s.access, s.forUser = access, forUser
}

// userClient returns a GitHub client acting as the person, or ok=false when they have not granted access.
func (s *Service) userClient(ctx context.Context, userID uuid.UUID) (UserRepos, bool, error) {
	if s.access == nil || s.forUser == nil {
		return nil, false, nil
	}
	token, err := s.access.Token(ctx, userID)
	if errors.Is(err, githubaccess.ErrNone) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("projects: %w", err)
	}
	return s.forUser(token), true, nil
}

// dropAccess forgets a token GitHub no longer accepts, so the screen offers to grant it again.
func (s *Service) dropAccess(ctx context.Context, userID uuid.UUID) {
	if err := s.access.Remove(ctx, userID); err != nil {
		s.logger.ErrorContext(ctx, "remove rejected github token", "error", err)
	}
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

// lookup parses the address and asks GitHub, enforcing the size limit. A repository that is not
// public is asked for again with the person's own token, if they granted one.
func (s *Service) lookup(ctx context.Context, userID uuid.UUID, rawURL string) (github.Repository, error) {
	ref, err := github.ParseURL(rawURL)
	if err != nil {
		return github.Repository{}, ErrInvalidURL
	}
	repo, err := s.repos.Repository(ctx, ref)
	if errors.Is(err, github.ErrNotFound) {
		uc, ok, uerr := s.userClient(ctx, userID)
		if uerr != nil {
			return github.Repository{}, uerr
		}
		if ok {
			repo, err = uc.Repository(ctx, ref)
			if errors.Is(err, github.ErrUnauthorized) {
				s.dropAccess(ctx, userID)
				err = github.ErrNotFound
			}
		}
	}
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
	return s.lookup(ctx, userID, rawURL)
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
	repo, err := s.lookup(ctx, userID, rawURL)
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
		IsPrivate:       repo.Private,
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

// ContextPack is the built pack and whether the summary went into it.
type ContextPack struct {
	CommitSHA   string
	WithSummary bool
	Presets     []contextpack.Preset
}

// ContextPack builds the context pack from the stored analysis. It reads only; building is
// deterministic and takes no credits. Until an analysis has finished it is ErrNotFound.
func (s *Service) ContextPack(ctx context.Context, userID, id uuid.UUID) (ContextPack, error) {
	a, err := s.Analysis(ctx, userID, id)
	if err != nil {
		return ContextPack{}, err
	}
	p, err := s.q.GetProject(ctx, store.GetProjectParams{ID: id, UserID: userID})
	if err != nil {
		return ContextPack{}, fmt.Errorf("context pack: %w", err)
	}
	files, err := s.q.ListProjectFiles(ctx, store.ListProjectFilesParams{ProjectID: id, UserID: userID, RowLimit: 20000})
	if err != nil {
		return ContextPack{}, fmt.Errorf("context pack: %w", err)
	}
	in := contextpack.Input{Name: p.Name}
	if p.RepoDescription != nil {
		in.Description = *p.RepoDescription
	}
	if err := json.Unmarshal(a.Facts, &in.Facts); err != nil {
		return ContextPack{}, fmt.Errorf("context pack: read facts: %w", err)
	}
	if len(a.Summary) > 0 {
		var sum contextpack.Summary
		if err := json.Unmarshal(a.Summary, &sum); err == nil {
			in.Summary = &sum
		}
	}
	for _, h := range a.Hubs {
		in.Hubs = append(in.Hubs, contextpack.Hub{Kind: h.Kind, Key: h.Key, ImportedBy: int(h.ImportedBy)})
	}
	for _, f := range files {
		if f.SkippedReason == nil {
			in.Paths = append(in.Paths, f.Path)
		}
	}
	return ContextPack{CommitSHA: a.CommitSHA, WithSummary: in.Summary != nil, Presets: contextpack.Build(in)}, nil
}

// GitHubRepo is one repository the person's GitHub access can see.
type GitHubRepo struct {
	github.Repository
	Added    bool // already one of their projects
	TooLarge bool // over the size limit
}

// GitHubRepos lists the repositories the person's granted access can see, marking those already
// added. Without access (or when GitHub no longer accepts the token) it is ErrAccessRequired.
func (s *Service) GitHubRepos(ctx context.Context, userID uuid.UUID) ([]GitHubRepo, error) {
	if err := s.allow(ctx, ruleRepos, userID); err != nil {
		return nil, err
	}
	uc, ok, err := s.userClient(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAccessRequired
	}
	repos, err := uc.ListRepositories(ctx)
	if errors.Is(err, github.ErrUnauthorized) {
		s.dropAccess(ctx, userID)
		return nil, ErrAccessRequired
	}
	if err != nil {
		return nil, err
	}
	mine, err := s.q.ListProjects(ctx, store.ListProjectsParams{UserID: userID, RowLimit: listLimit})
	if err != nil {
		return nil, fmt.Errorf("list github repos: %w", err)
	}
	added := make(map[string]bool, len(mine))
	for _, p := range mine {
		if p.RepoOwner != nil && p.RepoName != nil {
			added[strings.ToLower(*p.RepoOwner+"/"+*p.RepoName)] = true
		}
	}
	out := make([]GitHubRepo, 0, len(repos))
	for _, r := range repos {
		out = append(out, GitHubRepo{Repository: r, Added: added[strings.ToLower(r.String())], TooLarge: r.SizeKB > MaxRepoKB})
	}
	return out, nil
}
