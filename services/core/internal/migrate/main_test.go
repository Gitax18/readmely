package migrate

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestMainRejectsBadUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"no command", nil, nil, "usage"},
		{"two commands", []string{"up", "down"}, nil, "usage"},
		{"unknown command", []string{"drop"}, nil, "unknown command"},
		{"missing url", []string{"up"}, nil, "MIGRATE_DATABASE_URL is required"},
		{"roles without passwords", []string{"roles"}, map[string]string{"MIGRATE_DATABASE_URL": "postgres://x"}, "at least 16 characters"},
		{"roles with a short password", []string{"roles"}, map[string]string{
			"MIGRATE_DATABASE_URL": "postgres://x", "APP_DB_PASSWORD": "a-real-password-0001", "AI_DB_PASSWORD": "short",
		}, "readmely_ai must be at least 16"},
		{"roles with the placeholder", []string{"roles"}, map[string]string{
			"MIGRATE_DATABASE_URL": "postgres://x", "APP_DB_PASSWORD": "change-me-local-only", "AI_DB_PASSWORD": "a-real-password-0002",
		}, "placeholder"},
		{"roles sharing a password", []string{"roles"}, map[string]string{
			"MIGRATE_DATABASE_URL": "postgres://x", "APP_DB_PASSWORD": "a-real-password-0003", "AI_DB_PASSWORD": "a-real-password-0003",
		}, "different passwords"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Main(context.Background(), tt.args, envOf(tt.env), &out, &errOut)
			if code != 2 {
				t.Fatalf("exit code %d, want 2; stderr: %s", code, errOut.String())
			}
			if !strings.Contains(errOut.String(), tt.want) {
				t.Fatalf("stderr %q does not contain %q", errOut.String(), tt.want)
			}
		})
	}
}

// down, reset and seed delete or fake data: they run only with APP_ENV=development, never by default.
func TestDestructiveCommandsNeedAnExplicitDevelopmentEnv(t *testing.T) {
	const url = "postgres://owner:pw@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"
	refused := []string{"", "  ", "production", "Production", " PRODUCTION ", "staging", "prod", "live", "test", "dev", "developmen"}
	for _, cmd := range []string{"down", "reset", "seed"} {
		for _, env := range refused {
			var out, errOut bytes.Buffer
			code := Main(context.Background(), []string{cmd}, envOf(map[string]string{"APP_ENV": env, "MIGRATE_DATABASE_URL": url}), &out, &errOut)
			if code != 2 || !strings.Contains(errOut.String(), "needs APP_ENV=development") {
				t.Errorf("%s with APP_ENV=%q: exit %d, stderr %q; want a refusal", cmd, env, code, errOut.String())
			}
		}
		// Allowed: past the guard, failing only because nothing listens on the port.
		for _, env := range []string{"development", " Development "} {
			var out, errOut bytes.Buffer
			code := Main(context.Background(), []string{cmd}, envOf(map[string]string{"APP_ENV": env, "MIGRATE_DATABASE_URL": url}), &out, &errOut)
			if code != 1 {
				t.Errorf("%s with APP_ENV=%q: exit %d, want 1 (cannot connect); stderr %q", cmd, env, code, errOut.String())
			}
		}
	}
	// Forward-only commands never need it.
	for _, cmd := range []string{"up", "status"} {
		var out, errOut bytes.Buffer
		if code := Main(context.Background(), []string{cmd}, envOf(map[string]string{"MIGRATE_DATABASE_URL": url}), &out, &errOut); code != 1 {
			t.Errorf("%s without APP_ENV: exit %d, want 1 (cannot connect); stderr %q", cmd, code, errOut.String())
		}
	}
}

func TestMainNeverEchoesTheConnectionString(t *testing.T) {
	var out, errOut bytes.Buffer
	secret := "s3cr3t-owner-password"
	code := Main(context.Background(), []string{"status"}, envOf(map[string]string{
		"MIGRATE_DATABASE_URL": "postgres://owner:" + secret + "@127.0.0.1:1/db?sslmode=disable&connect_timeout=1",
	}), &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code %d, want 1 (cannot connect); stderr: %s", code, errOut.String())
	}
	if strings.Contains(errOut.String(), secret) || strings.Contains(out.String(), secret) {
		t.Fatal("the password leaked into the output")
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("r", strings.Repeat("a", MinPasswordLength-1)); err == nil {
		t.Fatal("short password accepted")
	}
	if err := ValidatePassword("r", "Change-Me-Local-Only"); err == nil {
		t.Fatal("the placeholder was accepted")
	}
	if err := ValidatePasswords([]RolePassword{{"a", "same-password-00001"}, {"b", "same-password-00001"}}); err == nil {
		t.Fatal("two roles sharing a password were accepted")
	}
	if err := ValidatePassword("r", strings.Repeat("a", MinPasswordLength)); err != nil {
		t.Fatalf("minimum length rejected: %v", err)
	}
}
