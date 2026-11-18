package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var code = Message{To: "ada@example.invalid", Subject: "Your Readmely sign-in code", Text: "Your code is 013842"}

func TestLogMailerShowsTheEmail(t *testing.T) {
	var logs bytes.Buffer
	if err := (LogMailer{Logger: slog.New(slog.NewJSONHandler(&logs, nil))}).Send(context.Background(), code); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "013842") || !strings.Contains(logs.String(), "ada@example.invalid") {
		t.Fatalf("log: %s", logs.String())
	}
}

func TestFileMailerWritesPrivateCompleteFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mail")
	m := FileMailer{Dir: dir}
	for range 3 {
		if err := m.Send(context.Background(), code); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode %v (%v), want 0700", info.Mode().Perm(), err)
	}
	root, err := os.OpenRoot(dir) // reads below are confined to the mail directory
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Fatalf("%d files, want 3 (no temporary files left)", len(entries))
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			t.Fatalf("unexpected file %s", e.Name())
		}
		fi, _ := e.Info()
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s has mode %v, want 0600", e.Name(), fi.Mode().Perm())
		}
		raw, _ := root.ReadFile(e.Name())
		var got fileMessage
		if err := json.Unmarshal(raw, &got); err != nil || got.Message != code || got.SentAt.IsZero() {
			t.Fatalf("file %s: %+v %v", e.Name(), got, err)
		}
	}
}

func TestNew(t *testing.T) {
	if m, err := New("log", "", slog.New(slog.DiscardHandler)); err != nil || m == nil {
		t.Fatal(err)
	}
	if _, err := New("file", "", nil); err == nil {
		t.Fatal("file mailer without a directory")
	}
	if _, err := New("smtp", "", nil); !errors.Is(err, ErrUnknownMailer) {
		t.Fatal("unknown mailer accepted")
	}
}

// recorder is a Mailer that remembers what it sent, optionally slowly or failing.
type recorder struct {
	mu    sync.Mutex
	sent  []Message
	delay time.Duration
	fail  bool
	gate  chan struct{} // when set, every send waits for it to close
}

func (r *recorder) Send(ctx context.Context, m Message) error {
	if r.gate != nil {
		select {
		case <-r.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	time.Sleep(r.delay)
	if r.fail {
		return errors.New("provider said no")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, m)
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sent)
}

func TestQueueDeliversEverythingAndDrainsOnClose(t *testing.T) {
	r := &recorder{delay: 2 * time.Millisecond}
	q := NewQueue(r, slog.New(slog.DiscardHandler), 3, 100, time.Second)
	for range 50 {
		if !q.Enqueue(code) {
			t.Fatal("refused below capacity")
		}
	}
	if err := q.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.count() != 50 {
		t.Fatalf("sent %d of 50 before Close returned", r.count())
	}
	if q.Enqueue(code) {
		t.Fatal("accepted after Close")
	}
	if err := q.Close(context.Background()); err != nil {
		t.Fatal("a second Close must be harmless")
	}
}

// A full queue answers at once, drops, and logs nothing personal.
func TestAFullQueueDropsWithoutBlockingOrLeaking(t *testing.T) {
	var logs bytes.Buffer
	r := &recorder{gate: make(chan struct{})}
	q := NewQueue(r, slog.New(slog.NewJSONHandler(&logs, nil)), 1, 2, time.Minute)

	start := time.Now()
	accepted := 0
	for range 10 {
		if q.Enqueue(code) {
			accepted++
		}
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("Enqueue blocked on a full queue")
	}
	// One message is held by the blocked sender, two wait in the queue.
	if accepted > 3 || q.Dropped() < 7 {
		t.Fatalf("accepted %d, dropped %d", accepted, q.Dropped())
	}
	if strings.Contains(logs.String(), "ada@") || strings.Contains(logs.String(), "013842") {
		t.Fatalf("the drop log leaked the address or code: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "send queue is full") {
		t.Fatalf("drops must be logged: %s", logs.String())
	}
	close(r.gate)
	if err := q.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFailuresAreLoggedWithoutTheAddress(t *testing.T) {
	var logs bytes.Buffer
	q := NewQueue(&recorder{fail: true}, slog.New(slog.NewJSONHandler(&logs, nil)), 1, 10, time.Second)
	q.Enqueue(code)
	if err := q.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if q.Failed() != 1 || !strings.Contains(logs.String(), "provider said no") || strings.Contains(logs.String(), "ada@") || strings.Contains(logs.String(), "013842") {
		t.Fatalf("failed=%d log=%s", q.Failed(), logs.String())
	}
}

func TestCloseGivesUpAtTheDeadline(t *testing.T) {
	r := &recorder{gate: make(chan struct{})}
	q := NewQueue(r, slog.New(slog.DiscardHandler), 1, 10, time.Minute)
	for range 3 {
		q.Enqueue(code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := q.Close(ctx)
	if err == nil || !strings.Contains(err.Error(), "unsent") {
		t.Fatalf("got %v", err)
	}
	close(r.gate)
}

// Enqueue racing Close must never panic (send on a closed channel).
func TestEnqueueRacingCloseIsSafe(t *testing.T) {
	for range 50 {
		q := NewQueue(&recorder{}, slog.New(slog.NewTextHandler(io.Discard, nil)), 2, 4, time.Second)
		var wg sync.WaitGroup
		var ok atomic.Int64
		for range 8 {
			wg.Go(func() {
				if q.Enqueue(code) {
					ok.Add(1)
				}
			})
		}
		wg.Go(func() { _ = q.Close(context.Background()) })
		wg.Wait()
	}
}
