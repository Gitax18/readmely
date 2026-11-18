//go:build integration

// Package store_test exercises the generated queries against real Postgres: the job queue
// under concurrency, lease loss and recovery, retries, tenancy and the credit queries.
package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/dbtest"
)

func ctxT(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return ctx
}

type env struct {
	db *dbtest.DB
	q  *store.Queries
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := dbtest.New(t)
	return &env{db: db, q: store.New(db.Pool)}
}

func (e *env) user(t *testing.T, email string) store.User {
	t.Helper()
	u, err := e.q.CreateUser(ctxT(t), store.CreateUserParams{Email: email})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func (e *env) project(t *testing.T, owner uuid.UUID, name string) store.Project {
	t.Helper()
	p, err := e.q.CreateProject(ctxT(t), store.CreateProjectParams{UserID: owner, Name: name})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p
}

type jobOpt func(*store.EnqueueJobParams)

func withKind(k string) jobOpt         { return func(p *store.EnqueueJobParams) { p.Kind = k } }
func withMaxAttempts(n int32) jobOpt   { return func(p *store.EnqueueJobParams) { p.MaxAttempts = n } }
func withRunAfter(at time.Time) jobOpt { return func(p *store.EnqueueJobParams) { p.RunAfter = &at } }
func withPayload(json string) jobOpt {
	return func(p *store.EnqueueJobParams) { p.Payload = []byte(json) }
}
func inProject(id uuid.UUID) jobOpt { return func(p *store.EnqueueJobParams) { p.ProjectID = &id } }

func (e *env) enqueue(t *testing.T, owner uuid.UUID, key string, opts ...jobOpt) store.Job {
	t.Helper()
	p := store.EnqueueJobParams{UserID: owner, Kind: "ingest", Payload: []byte("{}"), IdempotencyKey: key, MaxAttempts: 3}
	for _, o := range opts {
		o(&p)
	}
	j, err := e.q.EnqueueJob(ctxT(t), p)
	if err != nil {
		t.Fatalf("enqueue %s: %v", key, err)
	}
	return j
}

func (e *env) claim(ctx context.Context, worker string, kinds ...string) (store.Job, error) {
	return e.q.ClaimJob(ctx, store.ClaimJobParams{Worker: worker, LeaseSeconds: 60, Kinds: kinds})
}

func (e *env) expireLease(t *testing.T, id uuid.UUID) {
	t.Helper()
	if _, err := e.db.Pool.Exec(ctxT(t), `UPDATE jobs SET lease_expires_at = now() - interval '1 minute' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

func (e *env) reload(t *testing.T, j store.Job) store.Job {
	t.Helper()
	got, err := e.q.GetJob(ctxT(t), store.GetJobParams{ID: j.ID, UserID: j.UserID})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func wantNoRows(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected no rows, got %v", err)
	}
}

func str(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestEnqueueReplayReturnsTheOriginal(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")

	first := e.enqueue(t, ada.ID, "request-key-0001", withPayload(`{"repo":"a"}`))
	time.Sleep(20 * time.Millisecond)
	again := e.enqueue(t, ada.ID, "request-key-0001", withPayload(`{"repo":"CHANGED"}`))

	if again.ID != first.ID {
		t.Fatalf("replay created a second job: %s vs %s", again.ID, first.ID)
	}
	if string(again.Payload) != `{"repo": "a"}` {
		t.Fatalf("replay must keep the original payload, got %s", again.Payload)
	}
	if !again.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatal("a replay must not count as a modification")
	}
}

func TestEnqueueReplaysRaceToOneJob(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")

	const workers = 20
	ids := make(chan uuid.UUID, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := e.q.EnqueueJob(ctxT(t), store.EnqueueJobParams{UserID: ada.ID, Kind: "ingest", Payload: []byte("{}"), IdempotencyKey: "racing-key-0001", MaxAttempts: 3})
			if err != nil {
				t.Errorf("enqueue: %v", err)
				return
			}
			ids <- j.ID
		}()
	}
	wg.Wait()
	close(ids)
	distinct := map[uuid.UUID]bool{}
	for id := range ids {
		distinct[id] = true
	}
	if len(distinct) != 1 {
		t.Fatalf("%d distinct jobs for one request, want 1", len(distinct))
	}
}

func TestEveryJobIsClaimedExactlyOnce(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")
	const jobs, workers = 200, 20
	want := map[uuid.UUID]bool{}
	for i := range jobs {
		want[e.enqueue(t, ada.ID, fmt.Sprintf("bulk-key-%05d", i)).ID] = true
	}

	var mu sync.Mutex
	claimedBy := map[uuid.UUID][]string{}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for w := range workers {
		wg.Add(1)
		name := fmt.Sprintf("worker-%02d", w)
		go func() {
			defer wg.Done()
			<-start
			for {
				j, err := e.claim(ctxT(t), name)
				if errors.Is(err, pgx.ErrNoRows) {
					return
				}
				if err != nil {
					t.Errorf("%s: claim: %v", name, err)
					return
				}
				if j.Status != "running" || j.Attempts != 1 || j.LockedBy == nil || *j.LockedBy != name || j.LeaseExpiresAt == nil {
					t.Errorf("claimed job in a wrong state: %+v", j)
				}
				mu.Lock()
				claimedBy[j.ID] = append(claimedBy[j.ID], name)
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(claimedBy) != jobs {
		t.Fatalf("%d of %d jobs were claimed", len(claimedBy), jobs)
	}
	for id, by := range claimedBy {
		if !want[id] {
			t.Errorf("claimed an unknown job %s", id)
		}
		if len(by) != 1 {
			t.Errorf("job %s was claimed %d times: %v", id, len(by), by)
		}
	}
}

func TestClaimOrderAndSchedule(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")
	now := time.Now()
	later := e.enqueue(t, ada.ID, "order-key-later", withRunAfter(now.Add(-1*time.Minute)))
	earlier := e.enqueue(t, ada.ID, "order-key-earlier", withRunAfter(now.Add(-1*time.Hour)))
	e.enqueue(t, ada.ID, "order-key-future", withRunAfter(now.Add(1*time.Hour)))

	for i, want := range []uuid.UUID{earlier.ID, later.ID} {
		got, err := e.claim(ctxT(t), "w")
		if err != nil || got.ID != want {
			t.Fatalf("claim %d: got %v (%v), want %s", i, got.ID, err, want)
		}
	}
	_, err := e.claim(ctxT(t), "w")
	wantNoRows(t, err) // the only job left is scheduled for later
}

func TestClaimFiltersByKind(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")
	readme := e.enqueue(t, ada.ID, "kind-key-readme", withKind("generate_readme"))
	ingest := e.enqueue(t, ada.ID, "kind-key-ingest", withKind("ingest"))

	got, err := e.claim(ctxT(t), "ai-worker", "generate_readme", "generate_diagram")
	if err != nil || got.ID != readme.ID {
		t.Fatalf("the AI worker got %v (%v), want the README job", got.ID, err)
	}
	_, err = e.claim(ctxT(t), "ai-worker", "generate_readme", "generate_diagram")
	wantNoRows(t, err) // the remaining job is not its kind
	got, err = e.claim(ctxT(t), "ingest-worker", "ingest")
	if err != nil || got.ID != ingest.ID {
		t.Fatalf("the ingest worker got %v (%v)", got.ID, err)
	}
}

func TestACrashedWorkersJobIsRecoveredAndTheOldWorkerIsFenced(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")
	job := e.enqueue(t, ada.ID, "crash-key-0001")

	if _, err := e.claim(ctxT(t), "worker-a"); err != nil {
		t.Fatal(err)
	}
	// A live lease is left alone.
	if got, err := e.q.RequeueExpiredJobs(ctxT(t)); err != nil || len(got) != 0 {
		t.Fatalf("requeued %d live jobs (%v)", len(got), err)
	}

	e.expireLease(t, job.ID) // worker-a dies
	recovered, err := e.q.RequeueExpiredJobs(ctxT(t))
	if err != nil || len(recovered) != 1 || recovered[0].Status != "queued" || str(recovered[0].ErrorCode) != "lease_expired" {
		t.Fatalf("recovery: %+v (%v)", recovered, err)
	}

	b, err := e.claim(ctxT(t), "worker-b")
	if err != nil || b.ID != job.ID || b.Attempts != 2 {
		t.Fatalf("worker-b claim: %+v (%v)", b, err)
	}

	// worker-a wakes up and tries to report: it no longer owns the job.
	_, err = e.q.CompleteJob(ctxT(t), store.CompleteJobParams{ID: job.ID, Worker: "worker-a"})
	wantNoRows(t, err)
	_, err = e.q.ExtendJobLease(ctxT(t), store.ExtendJobLeaseParams{ID: job.ID, Worker: "worker-a", LeaseSeconds: 60})
	wantNoRows(t, err)
	_, err = e.q.ReleaseJob(ctxT(t), store.ReleaseJobParams{ID: job.ID, Worker: "worker-a", ErrorCode: "boom"})
	wantNoRows(t, err)
	if got := e.reload(t, job); got.Status != "running" || str(got.LockedBy) != "worker-b" {
		t.Fatalf("the stale worker changed the job: %+v", got)
	}

	done, err := e.q.CompleteJob(ctxT(t), store.CompleteJobParams{ID: job.ID, Worker: "worker-b"})
	if err != nil || done.Status != "succeeded" || done.FinishedAt == nil || done.LockedBy != nil || done.LeaseExpiresAt != nil {
		t.Fatalf("complete: %+v (%v)", done, err)
	}
}

func TestExtendJobLease(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")
	job := e.enqueue(t, ada.ID, "lease-key-0001")
	claimed, err := e.q.ClaimJob(ctxT(t), store.ClaimJobParams{Worker: "w", LeaseSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	extended, err := e.q.ExtendJobLease(ctxT(t), store.ExtendJobLeaseParams{ID: job.ID, Worker: "w", LeaseSeconds: 600})
	if err != nil || !extended.LeaseExpiresAt.After(*claimed.LeaseExpiresAt) {
		t.Fatalf("lease was not extended: %v -> %v (%v)", claimed.LeaseExpiresAt, extended.LeaseExpiresAt, err)
	}
	e.expireLease(t, job.ID)
	_, err = e.q.ExtendJobLease(ctxT(t), store.ExtendJobLeaseParams{ID: job.ID, Worker: "w", LeaseSeconds: 60})
	wantNoRows(t, err) // too late: the job may already have been given away
}

func TestRetriesBackOffAndEventuallyFail(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")
	job := e.enqueue(t, ada.ID, "retry-key-0001", withMaxAttempts(2))

	if _, err := e.claim(ctxT(t), "w"); err != nil {
		t.Fatal(err)
	}
	// First failure with a long backoff: queued again, but not before the backoff has passed.
	r, err := e.q.ReleaseJob(ctxT(t), store.ReleaseJobParams{ID: job.ID, Worker: "w", ErrorCode: "provider_timeout", BackoffSeconds: 3600})
	if err != nil || r.Status != "queued" || r.Attempts != 1 || str(r.ErrorCode) != "provider_timeout" || r.LockedBy != nil {
		t.Fatalf("first release: %+v (%v)", r, err)
	}
	if !r.RunAfter.After(time.Now().Add(55 * time.Minute)) {
		t.Fatalf("run_after %v does not reflect the backoff", r.RunAfter)
	}
	_, err = e.claim(ctxT(t), "w")
	wantNoRows(t, err)

	// Backoff over (simulated): the second attempt is the last.
	if _, err := e.db.Pool.Exec(ctxT(t), `UPDATE jobs SET run_after = now() WHERE id = $1`, job.ID); err != nil {
		t.Fatal(err)
	}
	second, err := e.claim(ctxT(t), "w")
	if err != nil || second.Attempts != 2 {
		t.Fatalf("second claim: %+v (%v)", second, err)
	}
	final, err := e.q.ReleaseJob(ctxT(t), store.ReleaseJobParams{ID: job.ID, Worker: "w", ErrorCode: "provider_timeout"})
	if err != nil || final.Status != "failed" || final.FinishedAt == nil || str(final.ErrorCode) != "provider_timeout" {
		t.Fatalf("final release: %+v (%v)", final, err)
	}
	_, err = e.claim(ctxT(t), "w")
	wantNoRows(t, err) // a failed job is never claimed again
}

func TestALostWorkerOnTheLastAttemptFailsTheJob(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")
	job := e.enqueue(t, ada.ID, "last-key-00001", withMaxAttempts(1))
	if _, err := e.claim(ctxT(t), "w"); err != nil {
		t.Fatal(err)
	}
	e.expireLease(t, job.ID)
	got, err := e.q.RequeueExpiredJobs(ctxT(t))
	if err != nil || len(got) != 1 || got[0].Status != "failed" || got[0].FinishedAt == nil || str(got[0].ErrorCode) != "lease_expired" {
		t.Fatalf("recovery: %+v (%v)", got, err)
	}
	_, err = e.claim(ctxT(t), "w")
	wantNoRows(t, err)
}

func TestCancelOnlyByTheOwnerAndOnlyBeforeStart(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")
	bob := e.user(t, "bob@example.invalid")
	queued := e.enqueue(t, ada.ID, "cancel-key-0001")
	running := e.enqueue(t, ada.ID, "cancel-key-0002")
	if _, err := e.claim(ctxT(t), "w", "ingest"); err != nil { // claims the oldest: queued
		t.Fatal(err)
	}
	// Whichever was claimed is now running; cancel the other one by its state, not by guessing.
	jobs := []store.Job{e.reload(t, queued), e.reload(t, running)}
	var stillQueued, nowRunning store.Job
	for _, j := range jobs {
		if j.Status == "queued" {
			stillQueued = j
		} else {
			nowRunning = j
		}
	}

	_, err := e.q.CancelJob(ctxT(t), store.CancelJobParams{ID: stillQueued.ID, UserID: bob.ID})
	wantNoRows(t, err)
	_, err = e.q.CancelJob(ctxT(t), store.CancelJobParams{ID: nowRunning.ID, UserID: ada.ID})
	wantNoRows(t, err)
	c, err := e.q.CancelJob(ctxT(t), store.CancelJobParams{ID: stillQueued.ID, UserID: ada.ID})
	if err != nil || c.Status != "cancelled" || c.FinishedAt == nil {
		t.Fatalf("cancel: %+v (%v)", c, err)
	}
	_, err = e.claim(ctxT(t), "w")
	wantNoRows(t, err)
}

func TestOneUsersDataIsInvisibleToAnother(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")
	bob := e.user(t, "bob@example.invalid")
	p := e.project(t, ada.ID, "Readmely")
	j := e.enqueue(t, ada.ID, "tenancy-key-001", inProject(p.ID))

	_, err := e.q.GetProject(ctxT(t), store.GetProjectParams{ID: p.ID, UserID: bob.ID})
	wantNoRows(t, err)
	_, err = e.q.RenameProject(ctxT(t), store.RenameProjectParams{ID: p.ID, UserID: bob.ID, Name: "stolen"})
	wantNoRows(t, err)
	_, err = e.q.SetProjectStatus(ctxT(t), store.SetProjectStatusParams{ID: p.ID, UserID: bob.ID, Status: "ready"})
	wantNoRows(t, err)
	if n, err := e.q.DeleteProject(ctxT(t), store.DeleteProjectParams{ID: p.ID, UserID: bob.ID}); err != nil || n != 0 {
		t.Fatalf("another user deleted %d projects (%v)", n, err)
	}
	_, err = e.q.GetJob(ctxT(t), store.GetJobParams{ID: j.ID, UserID: bob.ID})
	wantNoRows(t, err)
	if got, err := e.q.ListProjectJobs(ctxT(t), store.ListProjectJobsParams{ProjectID: &p.ID, UserID: bob.ID, RowLimit: 10}); err != nil || len(got) != 0 {
		t.Fatalf("another user listed %d jobs (%v)", len(got), err)
	}
	if list, err := e.q.ListProjects(ctxT(t), store.ListProjectsParams{UserID: bob.ID, RowLimit: 10}); err != nil || len(list) != 0 {
		t.Fatalf("another user listed %d projects (%v)", len(list), err)
	}

	// The owner still has everything, and deleting the project removes its jobs.
	if got, err := e.q.GetProject(ctxT(t), store.GetProjectParams{ID: p.ID, UserID: ada.ID}); err != nil || got.Name != "Readmely" {
		t.Fatalf("the owner lost the project: %+v (%v)", got, err)
	}
	if n, err := e.q.DeleteProject(ctxT(t), store.DeleteProjectParams{ID: p.ID, UserID: ada.ID}); err != nil || n != 1 {
		t.Fatalf("owner delete: %d (%v)", n, err)
	}
	_, err = e.q.GetJob(ctxT(t), store.GetJobParams{ID: j.ID, UserID: ada.ID})
	wantNoRows(t, err)
}

func TestUserQueries(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "Ada@Example.invalid")

	got, err := e.q.GetUserByEmail(ctxT(t), "ADA@example.INVALID")
	if err != nil || got.ID != ada.ID {
		t.Fatalf("lookup by email, any case: %+v (%v)", got, err)
	}
	v, err := e.q.MarkUserEmailVerified(ctxT(t), ada.ID)
	if err != nil || v.EmailVerifiedAt == nil {
		t.Fatalf("verify: %+v (%v)", v, err)
	}
	time.Sleep(20 * time.Millisecond)
	again, err := e.q.MarkUserEmailVerified(ctxT(t), ada.ID)
	if err != nil || !again.EmailVerifiedAt.Equal(*v.EmailVerifiedAt) {
		t.Fatal("verifying twice must keep the first verification time")
	}
	if _, err := e.q.SetUserStatus(ctxT(t), store.SetUserStatusParams{ID: ada.ID, Status: "banned"}); err == nil {
		t.Fatal("an unknown status must be refused")
	}
	if n, err := e.q.DeleteUser(ctxT(t), ada.ID); err != nil || n != 1 {
		t.Fatalf("delete: %d (%v)", n, err)
	}
	_, err = e.q.GetUser(ctxT(t), ada.ID)
	wantNoRows(t, err)
}

func TestCreditQueries(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")
	ctx := ctxT(t)

	if b, err := e.q.GetCreditBalance(ctx, ada.ID); err != nil || b != 0 {
		t.Fatalf("a new user has %d credits (%v)", b, err)
	}
	if _, err := e.q.GrantCredits(ctx, store.GrantCreditsParams{UserID: ada.ID, Amount: 100, IdempotencyKey: "grant-key-0001", Note: "welcome"}); err != nil {
		t.Fatal(err)
	}
	first, err := e.q.ReserveCredits(ctx, store.ReserveCreditsParams{UserID: ada.ID, Amount: 30, IdempotencyKey: "reserve-key-001"})
	if err != nil || first.Delta != -30 || first.Kind != "reserve" {
		t.Fatalf("reserve: %+v (%v)", first, err)
	}
	again, err := e.q.ReserveCredits(ctx, store.ReserveCreditsParams{UserID: ada.ID, Amount: 30, IdempotencyKey: "reserve-key-001"})
	if err != nil || again.ID != first.ID {
		t.Fatalf("replay: %+v (%v)", again, err)
	}
	if b, _ := e.q.GetCreditBalance(ctx, ada.ID); b != 70 {
		t.Fatalf("balance %d, want 70", b)
	}

	_, err = e.q.ReserveCredits(ctx, store.ReserveCreditsParams{UserID: ada.ID, Amount: 71, IdempotencyKey: "reserve-key-002"})
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "CR001" {
		t.Fatalf("expected insufficient credits (CR001), got %v", err)
	}
	entries, err := e.q.ListCreditEntries(ctx, store.ListCreditEntriesParams{UserID: ada.ID, RowLimit: 10})
	if err != nil || len(entries) != 2 || entries[0].Kind != "reserve" {
		t.Fatalf("history (newest first): %+v (%v)", entries, err)
	}
}
