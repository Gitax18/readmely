package worker

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestMainRunsUntilCancelledThenExitsCleanly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- Main(ctx, "ingest", env(nil), &stdout, &stderr) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
	out := stdout.String()
	if !strings.Contains(out, "worker started") || !strings.Contains(out, "worker stopped") || !strings.Contains(out, `"service":"ingest"`) {
		t.Fatalf("unexpected log output: %s", out)
	}
}

func TestMainRefusesToStartWithBadConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main(context.Background(), "render", env(map[string]string{"LOG_LEVEL": "loud"}), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "LOG_LEVEL") {
		t.Fatalf("stderr should name the bad variable: %s", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("nothing should be logged before config is valid: %s", stdout.String())
	}
}
