package config

import (
	"fmt"
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
		"DATABASE_URL":  "postgres://u:p@localhost:5432/db",
		"REDIS_URL":     "redis://:p@localhost:6379/0",
		"AUTH_CODE_KEY": strings.Repeat("k", 64),
	}
}

func TestAuthSettings(t *testing.T) {
	c, err := Load(env(validEnv()))
	if err != nil || c.WebOrigin != "http://127.0.0.1:3000" || c.Mailer != MailerLog || len(c.TrustedProxies) != 0 {
		t.Fatalf("development defaults: %+v %v", c, err)
	}

	m := validEnv()
	m["TRUSTED_PROXIES"] = " 10.0.0.5, 172.28.0.0/24 ,::ffff:192.0.2.1, 2001:db8::/64"
	m["MAILER"] = "file"
	m["MAIL_DIR"] = "/tmp/mail"
	m["WEB_ORIGIN"] = "https://readmely.example"
	c, err = Load(env(m))
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(c.TrustedProxies)
	if got != "[10.0.0.5/32 172.28.0.0/24 192.0.2.1/32 2001:db8::/64]" {
		t.Fatalf("trusted proxies parsed as %s", got)
	}

	bad := []struct {
		name string
		set  map[string]string
		want string
	}{
		{"missing key", map[string]string{"AUTH_CODE_KEY": ""}, "AUTH_CODE_KEY: is required"},
		{"short key", map[string]string{"AUTH_CODE_KEY": "short"}, "AUTH_CODE_KEY: must be at least 32"},
		{"placeholder key", map[string]string{"AUTH_CODE_KEY": "change-me-local-only-change-me-local-only"}, "AUTH_CODE_KEY: is still the placeholder"},
		{"origin with path", map[string]string{"WEB_ORIGIN": "https://x.example/app"}, "WEB_ORIGIN: must be only scheme"},
		{"origin with credentials", map[string]string{"WEB_ORIGIN": "https://u:p@x.example"}, "WEB_ORIGIN: must be only scheme"},
		{"origin not http", map[string]string{"WEB_ORIGIN": "ftp://x.example"}, "WEB_ORIGIN: scheme must be http or https"},
		{"proxy garbage", map[string]string{"TRUSTED_PROXIES": "10.0.0.1, banana"}, "TRUSTED_PROXIES: must be a comma-separated"},
		{"trust everyone", map[string]string{"TRUSTED_PROXIES": "0.0.0.0/0"}, "TRUSTED_PROXIES: must not trust every address"},
		{"trust every v6", map[string]string{"TRUSTED_PROXIES": "::/0"}, "TRUSTED_PROXIES: must not trust every address"},
		{"unknown mailer", map[string]string{"MAILER": "sendgrid"}, "MAILER: must be log, file or smtp"},
		{"file without dir", map[string]string{"MAILER": "file"}, "MAIL_DIR: is required when MAILER=file"},
		{"production without origin", map[string]string{"APP_ENV": "production", "MAILER": "log"}, "WEB_ORIGIN: is required in production"},
		{"production over http", map[string]string{"APP_ENV": "production", "WEB_ORIGIN": "http://x.example"}, "WEB_ORIGIN: must use https in production"},
		{"production without mailer", map[string]string{"APP_ENV": "production", "WEB_ORIGIN": "https://x.example"}, "MAILER: is required in production"},
		{"production with the log mailer", map[string]string{"APP_ENV": "production", "WEB_ORIGIN": "https://x.example", "MAILER": "log"}, "MAILER: log and file are for development only"},
		{"production with the file mailer", map[string]string{"APP_ENV": "production", "WEB_ORIGIN": "https://x.example", "MAILER": "file", "MAIL_DIR": "/m"}, "MAILER: log and file are for development only"},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			m := validEnv()
			for k, v := range tt.set {
				m[k] = v
			}
			_, err := Load(env(m))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestAuthCodeKeyIsNeverPrinted(t *testing.T) {
	m := validEnv()
	m["AUTH_CODE_KEY"] = strings.Repeat("s3cr3t", 6) + "!" // invalid character
	_, err := Load(env(m))
	if err == nil || strings.Contains(err.Error(), "s3cr3t") {
		t.Fatalf("error must exist and not contain the value: %v", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(validEnv()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Env != EnvDevelopment || c.HTTPAddr != ":8080" || c.InternalAddr != "127.0.0.1:8081" || c.AIServiceURL != "http://ai:8000" || c.AIServiceToken != "" || c.LogLevel != slog.LevelInfo || c.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestLoadOverrides(t *testing.T) {
	// Production needs a real mailer, which does not exist yet (see TestAuthSettings), so the
	// environment name is checked through LoadBase and the other overrides in development.
	if base, err := LoadBase(env(map[string]string{"APP_ENV": "production"})); err != nil || base.Env != EnvProduction {
		t.Fatalf("APP_ENV=production: %+v %v", base, err)
	}
	m := validEnv()
	m["HTTP_ADDR"] = "127.0.0.1:9090"
	m["LOG_LEVEL"] = "debug"
	m["SHUTDOWN_TIMEOUT"] = "3s"
	c, err := Load(env(m))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Env != EnvDevelopment || c.HTTPAddr != "127.0.0.1:9090" || c.LogLevel != slog.LevelDebug || c.ShutdownTimeout != 3*time.Second {
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
		{"ai url wrong scheme", map[string]string{"AI_SERVICE_URL": "ftp://ai:8000"}, "AI_SERVICE_URL: scheme must be"},
		{"ai url with credentials", map[string]string{"AI_SERVICE_URL": "http://user:pw@ai:8000"}, "AI_SERVICE_URL: must not contain credentials"},
		{"ai url with query", map[string]string{"AI_SERVICE_URL": "http://ai:8000/?x=1"}, "AI_SERVICE_URL: must not contain"},
		{"ai token too short", map[string]string{"AI_SERVICE_TOKEN": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, "AI_SERVICE_TOKEN: must be at least 32"},
		{"ai token unsafe chars", map[string]string{"AI_SERVICE_TOKEN": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@:/"}, "AI_SERVICE_TOKEN: may only contain"},
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

func TestLoadAcceptsAIServiceSettings(t *testing.T) {
	token := strings.Repeat("ab", 16)
	m := validEnv()
	m["AI_SERVICE_URL"] = "http://127.0.0.1:8000"
	m["AI_SERVICE_TOKEN"] = token
	c, err := Load(env(m))
	if err != nil || c.AIServiceURL != "http://127.0.0.1:8000" || c.AIServiceToken != token {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestLoadNeverPrintsTheToken(t *testing.T) {
	secret := "bad token with spaces " + strings.Repeat("z", 20)
	m := validEnv()
	m["AI_SERVICE_TOKEN"] = secret
	_, err := Load(env(m))
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "zzzz") {
		t.Fatalf("error must exist and not contain the value: %v", err)
	}
}

func TestOAuthCredentials(t *testing.T) {
	c, err := Load(env(validEnv()))
	if err != nil || c.GoogleClientID != "" || c.GitHubClientID != "" {
		t.Fatalf("providers must default to off: %+v %v", c, err)
	}

	m := validEnv()
	m["GOOGLE_CLIENT_ID"] = "1234-abc.apps.googleusercontent.com"
	m["GOOGLE_CLIENT_SECRET"] = "GOCSPX-abc_DEF-123456"
	m["GITHUB_CLIENT_ID"] = "Iv1.0123456789abcdef"
	m["GITHUB_CLIENT_SECRET"] = strings.Repeat("a1", 20)
	c, err = Load(env(m))
	if err != nil || c.GoogleClientID == "" || c.GoogleClientSecret == "" || c.GitHubClientID == "" || c.GitHubClientSecret == "" {
		t.Fatalf("both pairs: %+v %v", c, err)
	}

	for _, tt := range []struct {
		name string
		set  map[string]string
		want string
	}{
		{"google id only", map[string]string{"GOOGLE_CLIENT_ID": "abcdefgh12345"}, "GOOGLE_CLIENT_SECRET: is required when GOOGLE_CLIENT_ID is set"},
		{"google secret only", map[string]string{"GOOGLE_CLIENT_SECRET": "abcdefgh12345"}, "GOOGLE_CLIENT_ID: is required when GOOGLE_CLIENT_SECRET is set"},
		{"github id only", map[string]string{"GITHUB_CLIENT_ID": "abcdefgh12345"}, "GITHUB_CLIENT_SECRET: is required when GITHUB_CLIENT_ID is set"},
		{"github secret only", map[string]string{"GITHUB_CLIENT_SECRET": "abcdefgh12345"}, "GITHUB_CLIENT_ID: is required when GITHUB_CLIENT_SECRET is set"},
		{"short secret", map[string]string{"GITHUB_CLIENT_ID": "abcdefgh12345", "GITHUB_CLIENT_SECRET": "short"}, "GITHUB_CLIENT_SECRET: must be 8 to 256"},
		{"placeholder", map[string]string{"GITHUB_CLIENT_ID": "abcdefgh12345", "GITHUB_CLIENT_SECRET": "change-me-change-me"}, "GITHUB_CLIENT_SECRET: is still the placeholder"},
		{"quote in id", map[string]string{"GOOGLE_CLIENT_ID": `"abcdefgh12345"`, "GOOGLE_CLIENT_SECRET": "abcdefgh12345"}, "GOOGLE_CLIENT_ID: contains a character"},
		{"space in secret", map[string]string{"GOOGLE_CLIENT_ID": "abcdefgh12345", "GOOGLE_CLIENT_SECRET": "abcd efgh 12345"}, "GOOGLE_CLIENT_SECRET: contains a character"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := validEnv()
			for k, v := range tt.set {
				m[k] = v
			}
			_, err := Load(env(m))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
			for _, v := range tt.set {
				if len(v) >= 8 && strings.Contains(err.Error(), v) {
					t.Fatalf("the error contains a configured value: %v", err)
				}
			}
		})
	}
}
