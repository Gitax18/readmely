package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/github"
	"github.com/Gitax18/readmely/services/core/internal/githubaccess"
	"github.com/Gitax18/readmely/services/core/internal/projects"
	"github.com/Gitax18/readmely/services/core/internal/worker"
)

// Pipeline runs one analysis.
type Pipeline struct {
	Pool   *pgxpool.Pool
	GitHub *github.Client
	// Access holds the owners' tokens for private repositories; nil when that is switched off.
	Access *githubaccess.Store
}

// analysisTimeout bounds the whole run, download included.
const analysisTimeout = 4 * time.Minute

// Handle is the worker handler for an ingest job.
func (p *Pipeline) Handle(ctx context.Context, job store.Job, report worker.Reporter) error {
	ctx, cancel := context.WithTimeout(ctx, analysisTimeout)
	defer cancel()
	if job.ProjectID == nil {
		return &worker.Failure{Code: "project_missing", Err: errors.New("job has no project"), Permanent: true}
	}
	q := store.New(p.Pool)
	proj, err := q.GetProject(ctx, store.GetProjectParams{ID: *job.ProjectID, UserID: job.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return &worker.Failure{Code: "project_missing", Err: err, Permanent: true}
	}
	if err != nil {
		return err
	}
	if proj.RepoOwner == nil || proj.RepoName == nil || proj.DefaultBranch == nil {
		return &worker.Failure{Code: "project_missing", Err: errors.New("project has no repository"), Permanent: true}
	}
	ref := github.Ref{Owner: *proj.RepoOwner, Name: *proj.RepoName}

	// A private repository is read with its owner's token, and only the owner's.
	gh := p.GitHub
	if proj.IsPrivate {
		var failure error
		if gh, failure = p.ownerClient(ctx, job.UserID); failure != nil {
			return failure
		}
	}

	report("fetching", 8)
	sha, err := gh.CommitSHA(ctx, ref, *proj.DefaultBranch)
	if err != nil {
		return p.githubFailure(ctx, job.UserID, proj.IsPrivate, err)
	}
	body, err := gh.Archive(ctx, ref, sha)
	if err != nil {
		return p.githubFailure(ctx, job.UserID, proj.IsPrivate, err)
	}
	files, stats, err := Extract(body)
	_ = body.Close()
	switch {
	case errors.Is(err, ErrTooLarge), errors.Is(err, github.ErrArchiveTooLarge):
		return &worker.Failure{Code: "repository_too_large", Err: err, Permanent: true}
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		return &worker.Failure{Code: "archive_unreadable", Err: err}
	}
	if len(files) == 0 {
		return &worker.Failure{Code: "repository_empty", Err: errors.New("no files in the archive"), Permanent: true}
	}

	report("scanning", 35)
	var findings []Finding
	for i := range files {
		f := &files[i]
		if f.Content == nil {
			continue
		}
		clean, found := Scan(f.Path, f.Content)
		f.Content = clean
		findings = append(findings, found...)
	}

	report("parsing", 55)
	parsed := map[string]Parsed{}
	for _, f := range files {
		if f.Content == nil {
			continue
		}
		if pr := Parse(f.Language, f.Content); len(pr.Symbols) > 0 || len(pr.Imports) > 0 || pr.IsMain {
			parsed[f.Path] = pr
		}
	}

	report("mapping", 75)
	facts, d := buildFacts(files, parsed, findings)
	edges := buildGraph(files, parsed, d)
	factsJSON, err := json.Marshal(facts)
	if err != nil {
		return fmt.Errorf("encode facts: %w", err)
	}

	report("saving", 88)
	if err := p.save(ctx, job, proj, sha, files, stats, parsed, edges, factsJSON); err != nil {
		return err
	}
	return nil
}

// ownerClient is the GitHub client acting as the project's owner. Without a usable token the job
// fails for good with a code that tells the person to grant access again.
func (p *Pipeline) ownerClient(ctx context.Context, userID uuid.UUID) (*github.Client, error) {
	need := &worker.Failure{Code: "github_access_required", Err: githubaccess.ErrNone, Permanent: true}
	if p.Access == nil {
		return nil, need
	}
	token, err := p.Access.Token(ctx, userID)
	if errors.Is(err, githubaccess.ErrNone) {
		return nil, need
	}
	if err != nil {
		return nil, err
	}
	return p.GitHub.WithToken(token), nil
}

func (p *Pipeline) githubFailure(ctx context.Context, userID uuid.UUID, private bool, err error) error {
	if errors.Is(err, github.ErrUnauthorized) {
		// GitHub no longer accepts the token: forget it so the screen offers to grant it again.
		if private && p.Access != nil {
			_ = p.Access.Remove(context.WithoutCancel(ctx), userID)
		}
		return &worker.Failure{Code: "github_access_required", Err: err, Permanent: true}
	}
	return githubFailure(err)
}

func githubFailure(err error) error {
	switch {
	case errors.Is(err, github.ErrNotFound):
		return &worker.Failure{Code: "repository_not_found", Err: err, Permanent: true}
	case errors.Is(err, github.ErrArchiveTooLarge):
		return &worker.Failure{Code: "repository_too_large", Err: err, Permanent: true}
	default:
		return &worker.Failure{Code: "github_unavailable", Err: err}
	}
}

// save replaces the project's earlier analysis in one transaction, so a failure leaves the old
// result and a success never shows half of the new one.
func (p *Pipeline) save(ctx context.Context, ingestJob store.Job, proj store.Project, sha string, files []File, _ ExtractStats, parsed map[string]Parsed, edges []Edge, factsJSON []byte) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	q := store.New(tx)
	if err := q.ClearProjectFiles(ctx, proj.ID); err != nil {
		return err
	}
	if err := q.ClearProjectGraph(ctx, proj.ID); err != nil {
		return err
	}

	fileIDs := make(map[string]uuid.UUID, len(files))
	rows := make([][]any, 0, len(files))
	for _, f := range files {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		fileIDs[f.Path] = id
		var lang, skipped, content any
		if f.Language != "" {
			lang = f.Language
		}
		if f.Skipped != "" {
			skipped = f.Skipped
		} else {
			content = string(f.Content)
		}
		rows = append(rows, []any{id, proj.ID, f.Path, lang, int32(min(f.Size, 1<<31-1)), int32(f.Lines), content, skipped}) //nolint:gosec // bounded by the limits above
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"files"},
		[]string{"id", "project_id", "path", "language", "size_bytes", "line_count", "content", "skipped_reason"}, pgx.CopyFromRows(rows)); err != nil {
		return fmt.Errorf("save files: %w", err)
	}

	var symRows [][]any
	paths := make([]string, 0, len(parsed))
	for path := range parsed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		for _, s := range parsed[path].Symbols {
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			symRows = append(symRows, []any{id, proj.ID, fileIDs[path], truncate(s.Name, 200), s.Kind, int32(s.Line)}) //nolint:gosec // line numbers fit
		}
	}
	if len(symRows) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"symbols"},
			[]string{"id", "project_id", "file_id", "name", "kind", "line"}, pgx.CopyFromRows(symRows)); err != nil {
			return fmt.Errorf("save symbols: %w", err)
		}
	}

	nodeIDs := map[Node]uuid.UUID{}
	var nodeRows [][]any
	node := func(n Node) (uuid.UUID, bool, error) {
		if id, ok := nodeIDs[n]; ok {
			return id, true, nil
		}
		if len(nodeIDs) >= maxNodes {
			return uuid.Nil, false, nil
		}
		id, err := uuid.NewV7()
		if err != nil {
			return uuid.Nil, false, err
		}
		nodeIDs[n] = id
		nodeRows = append(nodeRows, []any{id, proj.ID, n.Kind, truncate(n.Key, 400)})
		return id, true, nil
	}
	var edgeRows [][]any
	for _, e := range edges {
		from, ok1, err := node(e.From)
		if err != nil {
			return err
		}
		to, ok2, err := node(e.To)
		if err != nil {
			return err
		}
		if !ok1 || !ok2 {
			continue
		}
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		edgeRows = append(edgeRows, []any{id, proj.ID, from, to, "imports"})
	}
	if len(nodeRows) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"graph_nodes"}, []string{"id", "project_id", "kind", "key"}, pgx.CopyFromRows(nodeRows)); err != nil {
			return fmt.Errorf("save graph nodes: %w", err)
		}
	}
	if len(edgeRows) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"graph_edges"}, []string{"id", "project_id", "from_node", "to_node", "kind"}, pgx.CopyFromRows(edgeRows)); err != nil {
			return fmt.Errorf("save graph edges: %w", err)
		}
	}

	if err := q.UpsertProjectFacts(ctx, store.UpsertProjectFactsParams{ProjectID: proj.ID, CommitSha: sha, Facts: factsJSON}); err != nil {
		return err
	}
	if err := q.SetProjectCommit(ctx, store.SetProjectCommitParams{ID: proj.ID, UserID: proj.UserID, CommitSha: &sha}); err != nil {
		return err
	}
	if err := q.ClearProjectSummary(ctx, proj.ID); err != nil {
		return err
	}
	if err := queueSummary(ctx, tx, q, proj, ingestJob, sha); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SummaryCost is what writing a project's summary holds, in credits. The AI worker settles it to
// what the model calls really cost, priced at 1.75 times (app/meter.py), and returns the rest.
// 18 is 10 x 1.75, rounded up so the hold covers any charge.
const SummaryCost int64 = 18

// queueSummary holds the credits and queues the summary job, in the analysis's own transaction. A
// balance too small to pay for it is not an error: the project is analysed, and a finished,
// refused job tells the page why there is no summary.
func queueSummary(ctx context.Context, tx pgx.Tx, q *store.Queries, proj store.Project, ingestJob store.Job, sha string) error {
	key := "summarise:" + ingestJob.ID.String()
	sp, err := tx.Begin(ctx) // a savepoint: a refused hold must not abort the analysis
	if err != nil {
		return err
	}
	sq := q.WithTx(sp)
	payload, err := json.Marshal(map[string]string{"commit_sha": sha})
	if err != nil {
		_ = sp.Rollback(ctx)
		return err
	}
	job, err := sq.EnqueueJob(ctx, store.EnqueueJobParams{
		UserID: proj.UserID, ProjectID: &proj.ID, Kind: projects.SummariseKind, Payload: payload, IdempotencyKey: key, MaxAttempts: 2,
	})
	if err == nil {
		_, err = sq.ReserveCredits(ctx, store.ReserveCreditsParams{UserID: proj.UserID, Amount: SummaryCost, JobID: &job.ID, IdempotencyKey: "hold:" + job.ID.String()})
	}
	if err == nil {
		return sp.Commit(ctx)
	}
	_ = sp.Rollback(ctx)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "CR001" {
		_, err = q.RecordRefusedJob(ctx, store.RecordRefusedJobParams{
			UserID: proj.UserID, ProjectID: &proj.ID, Kind: projects.SummariseKind, IdempotencyKey: key, ErrorCode: "insufficient_credits",
		})
	}
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
