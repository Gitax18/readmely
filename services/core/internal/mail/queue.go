package mail

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Queue sends messages in the background with a fixed number of senders and a fixed capacity.
// Enqueue never blocks: when the queue is full the message is dropped, logged without its
// address or text, and counted. Close stops new messages and waits for the queued ones.
type Queue struct {
	mailer      Mailer
	logger      *slog.Logger
	sendTimeout time.Duration
	ch          chan Message
	wg          sync.WaitGroup
	mu          sync.RWMutex // guards closed against a send on a closed channel
	closed      bool
	dropped     atomic.Int64
	failed      atomic.Int64
}

// NewQueue starts senders goroutines that deliver through m, each send bounded by sendTimeout.
func NewQueue(m Mailer, logger *slog.Logger, senders, capacity int, sendTimeout time.Duration) *Queue {
	q := &Queue{mailer: m, logger: logger, sendTimeout: sendTimeout, ch: make(chan Message, capacity)}
	for range senders {
		q.wg.Go(q.run)
	}
	return q
}

func (q *Queue) run() {
	for m := range q.ch {
		ctx, cancel := context.WithTimeout(context.Background(), q.sendTimeout)
		err := q.mailer.Send(ctx, m)
		cancel()
		if err != nil {
			q.failed.Add(1)
			// The error may come from a provider and is logged; the address and text never are.
			q.logger.Error("email could not be sent", "subject", m.Subject, "error", err)
		}
	}
}

// Enqueue hands m to the senders and reports whether it was accepted. It returns at once.
func (q *Queue) Enqueue(m Message) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return false
	}
	select {
	case q.ch <- m:
		return true
	default:
		q.dropped.Add(1)
		q.logger.Warn("email dropped: the send queue is full", "subject", m.Subject, "dropped_total", q.dropped.Load())
		return false
	}
}

// Dropped is how many messages were refused because the queue was full.
func (q *Queue) Dropped() int64 { return q.dropped.Load() }

// Failed is how many messages the mailer could not send.
func (q *Queue) Failed() int64 { return q.failed.Load() }

// Close refuses new messages and waits until the queued ones are sent or ctx ends.
func (q *Queue) Close(ctx context.Context) error {
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		close(q.ch)
	}
	q.mu.Unlock()

	done := make(chan struct{})
	go func() { q.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("mail queue: stopped with %d email(s) unsent: %w", len(q.ch), ctx.Err())
	}
}
