package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func validEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL": "postgres://u:p@localhost:5432/db",
		"REDIS_URL":    "redis://:p@localhost:6379/0",
	}
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(validEnv()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Env != EnvDevelopment || c.HTTPAddr != ":8080" || c.InternalAddr != "127.0.0.1:8081" || c.LogLevel != slog.LevelInfo || c.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestLoadOverrides(t *testing.T) {
	m := validEnv()
	m["APP_ENV"] = "production"
	m["HTTP_ADDR"] = "127.0.0.1:9090"
	m["LOG_LEVEL"] = "debug"
	m["SHUTDOWN_TIMEOUT"] = "3s"
	c, err := Load(env(m))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Env != EnvProduction || c.HTTPAddr != "127.0.0.1:9090" || c.LogLevel != slog.LevelDebug || c.ShutdownTimeout != 3*time.Second {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestLoadTrimsWhitespace(t *testing.T) {
	m := validEnv()
	m["HTTP_ADDR"] = "  :9091  "
	c, err := Load(env(m))
	if err != nil || c.HTTPAddr != ":9091" {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	tests := []struct {
		name string
		set  map[string]string
		want string
	}{
		{"missing database url", map[string]string{"DATABASE_URL": ""}, "DATABASE_URL: is required"},
		{"missing redis url", map[string]string{"REDIS_URL": ""}, "REDIS_URL: is required"},
		{"wrong database scheme", map[string]string{"DATABASE_URL": "mysql://u:p@h/db"}, "DATABASE_URL: scheme must be"},
		{"database url without host", map[string]string{"DATABASE_URL": "postgres:///db"}, "DATABASE_URL: must include a host"},
		{"wrong redis scheme", map[string]string{"REDIS_URL": "http://h:6379"}, "REDIS_URL: scheme must be"},
		{"bad addr", map[string]string{"HTTP_ADDR": "8080"}, "HTTP_ADDR: must be host:port"},
		{"addr port out of range", map[string]string{"HTTP_ADDR": ":70000"}, "HTTP_ADDR: port must be"},
		{"addr port not a number", map[string]string{"HTTP_ADDR": ":http"}, "HTTP_ADDR: port must be"},
		{"bad env", map[string]string{"APP_ENV": "staging"}, "APP_ENV: must be"},
		{"bad internal addr", map[string]string{"INTERNAL_ADDR": "8081"}, "INTERNAL_ADDR: must be host:port"},
		{"internal same as public", map[string]string{"HTTP_ADDR": ":9000", "INTERNAL_ADDR": ":9000"}, "INTERNAL_ADDR: must differ"},
		{"bad log level", map[string]string{"LOG_LEVEL": "loud"}, "LOG_LEVEL: must be"},
		{"timeout not a duration", map[string]string{"SHUTDOWN_TIMEOUT": "soon"}, "SHUTDOWN_TIMEOUT: must be"},
		{"timeout zero", map[string]string{"SHUTDOWN_TIMEOUT": "0s"}, "SHUTDOWN_TIMEOUT: must be"},
		{"timeout negative", map[string]string{"SHUTDOWN_TIMEOUT": "-5s"}, "SHUTDOWN_TIMEOUT: must be"},
		{"timeout too long", map[string]string{"SHUTDOWN_TIMEOUT": "5m"}, "SHUTDOWN_TIMEOUT: must be"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := validEnv()
			for k, v := range tc.set {
				m[k] = v
			}
			_, err := Load(env(m))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestLoadReportsAllProblemsAtOnce(t *testing.T) {
	_, err := Load(env(map[string]string{"APP_ENV": "nope", "LOG_LEVEL": "nope"}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, key := range []string{"APP_ENV", "LOG_LEVEL", "DATABASE_URL", "REDIS_URL"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not mention %s: %v", key, err)
		}
	}
}

func TestErrorsNeverContainValues(t *testing.T) {
	const secret = "hunter2-top-secret"
	cases := map[string]string{
		"DATABASE_URL": "mysql://user:" + secret + "@host/db",
		"REDIS_URL":    "ftp://:" + secret + "@host",
	}
	for key, val := range cases {
		m := validEnv()
		m[key] = val
		_, err := Load(env(m))
		if err == nil {
			t.Fatalf("%s: expected an error", key)
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("%s: error leaks the value: %v", key, err)
		}
	}

	// A string url.Parse rejects outright must not be echoed either.
	m := validEnv()
	m["DATABASE_URL"] = "postgres://user:" + secret + "@host:notaport/db"
	_, err := Load(env(m))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error leaks the value: %v", err)
	}
}

func TestLoadBaseNeedsNoURLs(t *testing.T) {
	c, err := LoadBase(env(map[string]string{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Env != EnvDevelopment {
		t.Fatalf("unexpected config: %+v", c)
	}
	if _, err := LoadBase(env(map[string]string{"LOG_LEVEL": "nope"})); err == nil {
		t.Fatal("expected an error for a bad log level")
	}
}
