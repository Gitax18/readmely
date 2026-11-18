package ratelimit

import (
	"strings"
	"testing"
	"time"
)

func TestRuleValidation(t *testing.T) {
	ok := Rule{Name: "login_start_email", Max: 5, Window: time.Hour}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, r := range []Rule{
		{Name: "Bad Name", Max: 1, Window: time.Minute},
		{Name: "", Max: 1, Window: time.Minute},
		{Name: "x", Max: 0, Window: time.Minute},
		{Name: "x", Max: 1, Window: 500 * time.Millisecond},
		{Name: "x", Max: 1, Window: time.Second + time.Microsecond},
	} {
		if r.Validate() == nil {
			t.Errorf("%+v was accepted", r)
		}
	}
}

func TestKeysHoldNoPersonalData(t *testing.T) {
	l := New(nil, "rl:")
	k := l.key(Rule{Name: "login_start_email"}, "ada@example.invalid")
	if strings.Contains(k, "ada") || strings.Contains(k, "example") || !strings.HasPrefix(k, "rl:login_start_email:") || len(k) != len("rl:login_start_email:")+32 {
		t.Fatalf("key %q", k)
	}
	if k == l.key(Rule{Name: "login_start_email"}, "bob@example.invalid") {
		t.Fatal("two subjects share a key")
	}
	if k == l.key(Rule{Name: "login_start_ip"}, "ada@example.invalid") {
		t.Fatal("two rules share a key")
	}
}

func TestRoundUpToSecond(t *testing.T) {
	for in, want := range map[time.Duration]time.Duration{
		0: time.Second, time.Millisecond: time.Second, time.Second: time.Second,
		1001 * time.Millisecond: 2 * time.Second, 59*time.Second + 1: time.Minute,
	} {
		if got := roundUpToSecond(in); got != want {
			t.Errorf("roundUpToSecond(%s) = %s, want %s", in, got, want)
		}
	}
}
