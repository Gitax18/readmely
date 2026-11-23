package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
)

// Reporter tells the person watching what a job is doing now: a short step name and 0 to 100.
type Reporter func(step string, progress int)

// Handler does one kind of job. Returning nil completes it; an error releases it for a retry (or
// fails it for good when the attempts are used up). Wrap an error in Failure to choose the short
// code the person sees; anything else shows as internal_error and is only logged.
type Handler func(ctx context.Context, job store.Job, report Reporter) error

// Failure is a job error with a stable machine code (a-z, digits and underscores).
type Failure struct {
	Code string
	Err  error
}

func (f *Failure) Error() string { return f.Code + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

// Runner claims jobs from the Postgres queue and runs them. Several runners, in any number of
// processes, can work the same queue: the claim is atomic, and a runner that dies simply stops
// renewing its lease so another picks the job up.
type Runner struct {
	Pool     store.DBTX
	Handlers map[string]Handler
	Logger   *slog.Logger

	// Zero values use the defaults below; tests and local runs may shorten them.
	Lease    time.Duration
	Poll     time.Duration
	Backoff  time.Duration
	workerID string
}

const (
	defaultLease   = 60 * time.Second
	defaultPoll    = time.Second
	defaultBackoff = 15 * time.Second
	requeueEvery   = 30 * time.Second
)

func (r *Runner) lease() time.Duration { return orDefault(r.Lease, defaultLease) }
func (r *Runner) poll() time.Duration  { return orDefault(r.Poll, defaultPoll) }

func orDefault(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

func (r *Runner) kinds() []string {
	out := make([]string, 0, len(r.Handlers))
	for k := range r.Handlers {
		out = append(out, k)
	}
	return out
}

// Run works the queue until ctx ends.
func (r *Runner) Run(ctx context.Context) {
	host, _ := os.Hostname()
	r.workerID = fmt.Sprintf("%s-%d", host, os.Getpid())
	q := store.New(r.Pool)
	lastRequeue := time.Time{}
	for ctx.Err() == nil {
		if time.Since(lastRequeue) > requeueEvery {
			lastRequeue = time.Now()
			r.requeueExpired(ctx, q)
		}
		job, err := q.ClaimJob(ctx, store.ClaimJobParams{Worker: r.workerID, LeaseSeconds: r.lease().Seconds(), Kinds: r.kinds()})
		if errors.Is(err, pgx.ErrNoRows) {
			sleep(ctx, r.poll())
			continue
		}
		if err != nil {
			if ctx.Err() == nil {
				r.Logger.Error("claim failed", "error", err)
			}
			sleep(ctx, 5*time.Second)
			continue
		}
		r.run(ctx, q, job)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// requeueExpired recovers jobs whose runner vanished; one that has used its attempts fails its project.
func (r *Runner) requeueExpired(ctx context.Context, q *store.Queries) {
	jobs, err := q.RequeueExpiredJobs(ctx)
	if err != nil {
		if ctx.Err() == nil {
			r.Logger.Error("requeue expired jobs failed", "error", err)
		}
		return
	}
	for _, j := range jobs {
		r.Logger.Warn("job lease expired", "job_id", j.ID, "kind", j.Kind, "status", j.Status)
		if j.Status == "failed" {
			r.setProjectStatus(ctx, q, j, "failed")
		}
	}
}

func (r *Runner) setProjectStatus(ctx context.Context, q *store.Queries, j store.Job, status string) {
	if j.ProjectID == nil {
		return
	}
	if _, err := q.SetProjectStatus(ctx, store.SetProjectStatusParams{ID: *j.ProjectID, UserID: j.UserID, Status: status}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		r.Logger.Error("set project status failed", "job_id", j.ID, "error", err)
	}
}

func (r *Runner) run(ctx context.Context, q *store.Queries, job store.Job) {
	log := r.Logger.With("job_id", job.ID, "kind", job.Kind, "attempt", job.Attempts)
	log.Info("job started")

	// Keep the lease alive while the handler works; cancel the handler if the lease is lost.
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		t := time.NewTicker(r.lease() / 3)
		defer t.Stop()
		for {
			select {
			case <-jobCtx.Done():
				return
			case <-t.C:
				if _, err := q.ExtendJobLease(jobCtx, store.ExtendJobLeaseParams{ID: job.ID, Worker: r.workerID, LeaseSeconds: r.lease().Seconds()}); err != nil {
					if jobCtx.Err() == nil {
						log.Error("lost the job lease", "error", err)
						cancel()
					}
					return
				}
			}
		}
	}()

	report := func(step string, progress int) {
		progress = max(0, min(progress, 100))
		if _, err := q.SetJobProgress(jobCtx, store.SetJobProgressParams{ID: job.ID, Worker: r.workerID, Step: step, Progress: int16(progress)}); err != nil && jobCtx.Err() == nil { //nolint:gosec // clamped to 0..100
			log.Warn("progress not saved", "error", err)
		}
	}

	err := r.safely(jobCtx, job, report)
	cancel()
	<-heartbeatDone

	// Finish with a fresh context: shutdown must not leave a finished job looking unfinished.
	fin, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()
	if err == nil {
		if _, err := q.CompleteJob(fin, store.CompleteJobParams{ID: job.ID, Worker: r.workerID}); err != nil {
			log.Error("could not complete the job", "error", err)
			return
		}
		r.setProjectStatus(fin, q, job, "ready")
		log.Info("job succeeded")
		return
	}
	code := "internal_error"
	var f *Failure
	if errors.As(err, &f) {
		code = f.Code
	}
	log.Error("job failed", "code", code, "error", err)
	released, rerr := q.ReleaseJob(fin, store.ReleaseJobParams{
		ID: job.ID, Worker: r.workerID, ErrorCode: code,
		BackoffSeconds: (orDefault(r.Backoff, defaultBackoff) * time.Duration(job.Attempts)).Seconds(),
	})
	if rerr != nil {
		log.Error("could not release the job", "error", rerr)
		return
	}
	if released.Status == "failed" {
		r.setProjectStatus(fin, q, job, "failed")
	}
}

// safely turns a panic in a handler into a failed attempt instead of a dead worker.
func (r *Runner) safely(ctx context.Context, job store.Job, report Reporter) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("handler panicked: %v", p)
		}
	}()
	h, ok := r.Handlers[job.Kind]
	if !ok {
		return &Failure{Code: "unknown_kind", Err: errors.New("no handler for " + job.Kind)}
	}
	return h(ctx, job, report)
}
