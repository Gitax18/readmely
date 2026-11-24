package readme

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/ratelimit"
)

// What the requests hold, in credits. The AI worker settles a job to what its model calls cost,
// priced at 1.75 times (app/meter.py), and returns the rest, or refunds all of it when the job fails
// for good. The holds are the earlier 15 and 2 times 1.75, rounded up so they cover any charge.
const (
	GenerateCost int64 = 27
	SectionCost  int64 = 4

	// GenerateKind and SectionKind are the job kinds the AI worker runs.
	GenerateKind = "generate_readme"
	SectionKind  = "readme_section"

	// MaxBytes is the largest README stored, matching the database constraint.
	MaxBytes       = 100 * 1024
	maxInstruction = 300
	maxHeading     = 80
	maxAttempts    = 2
)

var (
	ruleGenerate = ratelimit.Rule{Name: "readme_generate_user", Max: 10, Window: time.Hour}
	ruleSection  = ratelimit.Rule{Name: "readme_section_user", Max: 30, Window: time.Hour}
	ruleSave     = ratelimit.Rule{Name: "readme_save_user", Max: 120, Window: time.Hour}
)

// Errors the handlers turn into the documented answers.
var (
	ErrNotFound            = errors.New("project not found")
	ErrNotAnalysed         = errors.New("project has not been analysed")
	ErrRunning             = errors.New("a README job is already running")
	ErrInsufficientCredits = errors.New("not enough credits")
	ErrStaleVersion        = errors.New("a newer version exists")
	ErrInvalid             = errors.New("invalid request")
	ErrNoSuchSection       = errors.New("no such section")
)

// RateLimitedError carries how long the caller should wait.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "rate limited" }

// Service is the README logic. It has no HTTP in it.
type Service struct {
	pool    *pgxpool.Pool
	q       *store.Queries
	limiter *ratelimit.Limiter
}

// NewService wires the service.
func NewService(pool *pgxpool.Pool, limiter *ratelimit.Limiter) *Service {
	return &Service{pool: pool, q: store.New(pool), limiter: limiter}
}

func (s *Service) allow(ctx context.Context, rule ratelimit.Rule, userID uuid.UUID) error {
	d, err := s.limiter.Allow(ctx, ratelimit.Check{Rule: rule, Subject: userID.String()})
	if err != nil {
		return fmt.Errorf("readme: %w", err)
	}
	if !d.Allowed {
		return &RateLimitedError{RetryAfter: d.RetryAfter}
	}
	return nil
}

// State is what the README page shows.
type State struct {
	Readme *store.ReadmeVersion
	Score  *Score
	Job    *store.Job
}

func (s *Service) project(ctx context.Context, q *store.Queries, userID, id uuid.UUID) (store.Project, error) {
	p, err := q.GetProject(ctx, store.GetProjectParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Project{}, ErrNotFound
	}
	return p, err
}

// Get returns the current README with its score and the newest job that writes it.
func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (State, error) {
	if _, err := s.project(ctx, s.q, userID, id); err != nil {
		return State{}, err
	}
	var out State
	switch r, err := s.q.GetLatestReadme(ctx, id); {
	case err == nil:
		out.Readme = &r
		sc := ScoreOf(r.Content)
		out.Score = &sc
	case !errors.Is(err, pgx.ErrNoRows):
		return State{}, fmt.Errorf("get readme: %w", err)
	}
	for _, kind := range []string{GenerateKind, SectionKind} {
		j, err := s.q.GetLatestProjectJob(ctx, store.GetLatestProjectJobParams{ProjectID: &id, UserID: userID, Kind: kind})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return State{}, fmt.Errorf("get readme: %w", err)
		}
		if out.Job == nil || j.CreatedAt.After(out.Job.CreatedAt) {
			jj := j
			out.Job = &jj
		}
	}
	return out, nil
}

// Generate holds credits and queues a job that writes the README from the analysis.
func (s *Service) Generate(ctx context.Context, userID, id uuid.UUID, template string) (store.Job, error) {
	switch template {
	case "standard", "minimal", "detailed":
	default:
		return store.Job{}, ErrInvalid
	}
	return s.queue(ctx, userID, id, GenerateKind, GenerateCost, ruleGenerate, map[string]string{"template": template}, nil)
}

// RegenerateSection holds credits and queues a job that rewrites one section.
func (s *Service) RegenerateSection(ctx context.Context, userID, id uuid.UUID, heading, instruction string) (store.Job, error) {
	heading = strings.TrimSpace(heading)
	instruction = strings.TrimSpace(instruction)
	if heading == "" || utf8.RuneCountInString(heading) > maxHeading || utf8.RuneCountInString(instruction) > maxInstruction {
		return store.Job{}, ErrInvalid
	}
	check := func(q *store.Queries) error {
		r, err := q.GetLatestReadme(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !HasHeading(r.Content, heading)) {
			return ErrNoSuchSection
		}
		return err
	}
	return s.queue(ctx, userID, id, SectionKind, SectionCost, ruleSection,
		map[string]string{"heading": heading, "instruction": instruction}, check)
}

func (s *Service) queue(ctx context.Context, userID, id uuid.UUID, kind string, cost int64, rule ratelimit.Rule,
	payload map[string]string, pre func(*store.Queries) error,
) (store.Job, error) {
	if err := s.allow(ctx, rule, userID); err != nil {
		return store.Job{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return store.Job{}, err
	}
	key, err := newKey(kind)
	if err != nil {
		return store.Job{}, err
	}
	var job store.Job
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		p, err := s.project(ctx, q, userID, id)
		if err != nil {
			return err
		}
		if _, err := q.GetProjectFacts(ctx, store.GetProjectFactsParams{ProjectID: id, UserID: userID}); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotAnalysed
		} else if err != nil {
			return err
		}
		for _, k := range []string{GenerateKind, SectionKind} {
			active, err := q.CountActiveProjectJobs(ctx, store.CountActiveProjectJobsParams{ProjectID: &p.ID, UserID: userID, Kind: k})
			if err != nil {
				return err
			}
			if active > 0 {
				return ErrRunning
			}
		}
		if pre != nil {
			if err := pre(q); err != nil {
				return err
			}
		}
		job, err = q.EnqueueJob(ctx, store.EnqueueJobParams{
			UserID: userID, ProjectID: &p.ID, Kind: kind, Payload: body, IdempotencyKey: key, MaxAttempts: maxAttempts,
		})
		if err != nil {
			return err
		}
		_, err = q.ReserveCredits(ctx, store.ReserveCreditsParams{UserID: userID, Amount: cost, JobID: &job.ID, IdempotencyKey: "hold:" + job.ID.String()})
		return err
	})
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "CR001":
		return store.Job{}, ErrInsufficientCredits
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrNotAnalysed), errors.Is(err, ErrRunning), errors.Is(err, ErrNoSuchSection):
		return store.Job{}, err
	case err != nil:
		return store.Job{}, fmt.Errorf("queue readme job: %w", err)
	}
	return job, nil
}

func newKey(kind string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("readme: %w", err)
	}
	return kind + "-" + hex.EncodeToString(b), nil
}

// Save adds an edit as a new version, refused when baseVersion is no longer the newest.
func (s *Service) Save(ctx context.Context, userID, id uuid.UUID, content string, baseVersion int) (store.ReadmeVersion, error) {
	if err := validContent(content); err != nil {
		return store.ReadmeVersion{}, err
	}
	if baseVersion < 0 {
		return store.ReadmeVersion{}, ErrInvalid
	}
	if err := s.allow(ctx, ruleSave, userID); err != nil {
		return store.ReadmeVersion{}, err
	}
	return s.add(ctx, userID, id, content, "edited", int32(baseVersion)) //nolint:gosec // bounded by the contract
}

// Restore makes an older version current by adding a new version with its text.
func (s *Service) Restore(ctx context.Context, userID, id uuid.UUID, version int) (store.ReadmeVersion, error) {
	if err := s.allow(ctx, ruleSave, userID); err != nil {
		return store.ReadmeVersion{}, err
	}
	if _, err := s.project(ctx, s.q, userID, id); err != nil {
		return store.ReadmeVersion{}, err
	}
	old, err := s.q.GetReadmeVersion(ctx, store.GetReadmeVersionParams{ProjectID: id, Version: int32(version)}) //nolint:gosec // bounded by the contract
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ReadmeVersion{}, ErrNotFound
	}
	if err != nil {
		return store.ReadmeVersion{}, fmt.Errorf("restore readme: %w", err)
	}
	return s.add(ctx, userID, id, old.Content, "restored", -1)
}

func (s *Service) add(ctx context.Context, userID, id uuid.UUID, content, source string, base int32) (store.ReadmeVersion, error) {
	if _, err := s.project(ctx, s.q, userID, id); err != nil {
		return store.ReadmeVersion{}, err
	}
	v, err := s.q.AddReadmeVersion(ctx, store.AddReadmeVersionParams{ProjectID: id, Content: content, Source: source, BaseVersion: base})
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return store.ReadmeVersion{}, ErrStaleVersion
	case errors.As(err, &pgErr) && pgErr.Code == "23505": // two writers raced for the same number
		return store.ReadmeVersion{}, ErrStaleVersion
	case err != nil:
		return store.ReadmeVersion{}, fmt.Errorf("save readme: %w", err)
	}
	return v, nil
}

// Versions lists the history, newest first.
func (s *Service) Versions(ctx context.Context, userID, id uuid.UUID) ([]store.ListReadmeVersionsRow, error) {
	if _, err := s.project(ctx, s.q, userID, id); err != nil {
		return nil, err
	}
	rows, err := s.q.ListReadmeVersions(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list readme versions: %w", err)
	}
	return rows, nil
}

func validContent(c string) error {
	if strings.TrimSpace(c) == "" || len(c) > MaxBytes || !utf8.ValidString(c) || strings.ContainsRune(c, 0) {
		return ErrInvalid
	}
	return nil
}
