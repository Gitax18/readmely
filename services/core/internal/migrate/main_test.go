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
		{"down in production", []string{"down"}, map[string]string{"APP_ENV": "production", "MIGRATE_DATABASE_URL": "postgres://x"}, "refusing"},
		{"reset in production, any case", []string{"reset"}, map[string]string{"APP_ENV": " Production ", "MIGRATE_DATABASE_URL": "postgres://x"}, "refusing"},
		{"roles without passwords", []string{"roles"}, map[string]string{"MIGRATE_DATABASE_URL": "postgres://x"}, "at least 16 characters"},
		{"roles with a short password", []string{"roles"}, map[string]string{
			"MIGRATE_DATABASE_URL": "postgres://x", "APP_DB_PASSWORD": "change-me-local-only", "AI_DB_PASSWORD": "short",
		}, "readmely_ai must be at least 16"},
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

func TestSeedOnlyRunsInDevelopmentOrTest(t *testing.T) {
	const url = "postgres://owner:pw@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"
	refused := []string{"production", "Production", " PRODUCTION ", "staging", "prod", "live"}
	for _, env := range refused {
		var out, errOut bytes.Buffer
		code := Main(context.Background(), []string{"seed"}, envOf(map[string]string{"APP_ENV": env, "MIGRATE_DATABASE_URL": url}), &out, &errOut)
		if code != 2 || !strings.Contains(errOut.String(), "refusing to load seed data") {
			t.Errorf("APP_ENV=%q: exit %d, stderr %q; want a refusal", env, code, errOut.String())
		}
	}
	// Allowed environments get past the guard and fail only because nothing listens on the port.
	for _, env := range []string{"", "development", "Development", "test"} {
		var out, errOut bytes.Buffer
		code := Main(context.Background(), []string{"seed"}, envOf(map[string]string{"APP_ENV": env, "MIGRATE_DATABASE_URL": url}), &out, &errOut)
		if code != 1 {
			t.Errorf("APP_ENV=%q: exit %d, want 1 (cannot connect); stderr %q", env, code, errOut.String())
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
	if err := ValidatePassword("r", strings.Repeat("a", MinPasswordLength)); err != nil {
		t.Fatalf("minimum length rejected: %v", err)
	}
}
