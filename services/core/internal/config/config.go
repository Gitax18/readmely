// Package config loads and validates process configuration from the environment.
//
// Validation reports every problem at once and names the variable, but never
// includes a value, because values can contain passwords.
package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Environment names.
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

const (
	maxShutdownTimeout = 60 * time.Second
	minTokenLength     = 32
	maxTrustedProxies  = 16
	// Requests a client address may make each minute (internal/guard). 0 turns the guard off.
	defaultGuardPerMinute = 300
	maxGuardPerMinute     = 100_000
)

// Mailers. Only development may use the log and file mailers: they put codes on disk or in
// logs. Production sends through an SMTP relay (Brevo, Mailgun, Resend and others offer one).
const (
	MailerLog  = "log"  // writes each email to the log, code included (development only)
	MailerFile = "file" // writes each email as a JSON file to MAIL_DIR (development and browser tests)
	MailerSMTP = "smtp" // sends through an SMTP relay; the only mailer production accepts

	defaultSMTPPort = 587
)

// Config is the validated configuration of a Readmely Go process.
type Config struct {
	Env             string
	HTTPAddr        string
	InternalAddr    string // serves /readyz and /api/v1/status; never published to the internet
	AIServiceURL    string
	AIServiceToken  string // optional; empty means the status report says "unconfigured"
	DatabaseURL     string
	RedisURL        string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration

	GitHubAPIToken string // optional; never logged
	// GitHubTokenKey seals people's GitHub tokens (64 hex characters). Empty switches private
	// repositories off. Never logged.
	GitHubTokenKey string

	// Sign-in (Phase 2a).
	AuthCodeKey    string         // HMAC key for login codes and CSRF tokens; never logged
	WebOrigin      string         // the only origin allowed to make state-changing requests
	TrustedProxies []netip.Prefix // peers whose X-Forwarded-For is believed (the web server)
	GuardPerMinute int64          // requests per client address per minute; 0 turns the guard off
	Mailer         string         // MailerLog, MailerFile or MailerSMTP
	MailDir        string         // where MailerFile writes
	SMTPHost       string         // MailerSMTP only
	SMTPPort       int            // 587 (STARTTLS) or 465 (TLS from the start)
	SMTPUsername   string
	SMTPPassword   string // never logged
	MailFrom       string // the sender address; it must be one the provider has verified
	MailFromName   string // shown next to it

	// Provider sign-in (Phase 2b). Each pair is optional; with neither value the provider is off.
	GoogleClientID     string
	GoogleClientSecret string // never logged
	GitHubClientID     string
	GitHubClientSecret string // never logged
}

// Load reads and validates the full configuration needed by the API.
func Load(getenv func(string) string) (Config, error) {
	var c Config
	var errs []error

	c.loadBase(getenv, &errs)

	c.HTTPAddr = value(getenv, "HTTP_ADDR", ":8080")
	if err := validateAddr(c.HTTPAddr); err != nil {
		errs = append(errs, fieldError("HTTP_ADDR", err))
	}

	c.InternalAddr = value(getenv, "INTERNAL_ADDR", "127.0.0.1:8081")
	if err := validateAddr(c.InternalAddr); err != nil {
		errs = append(errs, fieldError("INTERNAL_ADDR", err))
	} else if c.InternalAddr == c.HTTPAddr {
		errs = append(errs, fieldError("INTERNAL_ADDR", errors.New("must differ from HTTP_ADDR")))
	}

	c.AIServiceURL = value(getenv, "AI_SERVICE_URL", "http://ai:8000")
	if err := validateServiceURL(c.AIServiceURL); err != nil {
		errs = append(errs, fieldError("AI_SERVICE_URL", err))
	}
	c.AIServiceToken = value(getenv, "AI_SERVICE_TOKEN", "")
	if c.AIServiceToken != "" {
		if err := validateToken(c.AIServiceToken); err != nil {
			errs = append(errs, fieldError("AI_SERVICE_TOKEN", err))
		}
	}

	c.DatabaseURL = value(getenv, "DATABASE_URL", "")
	if c.DatabaseURL == "" {
		errs = append(errs, fieldError("DATABASE_URL", errors.New("is required")))
	} else if err := validateURL(c.DatabaseURL, "postgres", "postgresql"); err != nil {
		errs = append(errs, fieldError("DATABASE_URL", err))
	}

	c.RedisURL = value(getenv, "REDIS_URL", "")
	if c.RedisURL == "" {
		errs = append(errs, fieldError("REDIS_URL", errors.New("is required")))
	} else if err := validateURL(c.RedisURL, "redis", "rediss"); err != nil {
		errs = append(errs, fieldError("REDIS_URL", err))
	}

	c.loadAuth(getenv, &errs)
	c.loadOAuth(getenv, &errs)

	c.loadGitHubTokenKey(getenv, &errs)

	// Optional: lets the API read public repository details at GitHub's higher rate limit.
	c.GitHubAPIToken = value(getenv, "GITHUB_API_TOKEN", "")
	if c.GitHubAPIToken != "" {
		if err := validateToken(c.GitHubAPIToken); err != nil {
			errs = append(errs, fieldError("GITHUB_API_TOKEN", err))
		}
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return c, nil
}

func (c *Config) loadAuth(getenv func(string) string, errs *[]error) {
	production := c.Env == EnvProduction

	c.AuthCodeKey = value(getenv, "AUTH_CODE_KEY", "")
	if c.AuthCodeKey == "" {
		*errs = append(*errs, fieldError("AUTH_CODE_KEY", errors.New("is required (generate with: openssl rand -hex 32)")))
	} else if err := validateToken(c.AuthCodeKey); err != nil {
		*errs = append(*errs, fieldError("AUTH_CODE_KEY", err))
	} else if strings.Contains(strings.ToLower(c.AuthCodeKey), "change-me") {
		*errs = append(*errs, fieldError("AUTH_CODE_KEY", errors.New("is still the placeholder from .env.example")))
	}

	fallbackOrigin := "http://127.0.0.1:3000"
	if production {
		fallbackOrigin = ""
	}
	c.WebOrigin = value(getenv, "WEB_ORIGIN", fallbackOrigin)
	if c.WebOrigin == "" {
		*errs = append(*errs, fieldError("WEB_ORIGIN", errors.New("is required in production")))
	} else if err := validateOrigin(c.WebOrigin, production); err != nil {
		*errs = append(*errs, fieldError("WEB_ORIGIN", err))
	}

	if raw := value(getenv, "TRUSTED_PROXIES", ""); raw != "" {
		prefixes, err := parseProxies(raw)
		if err != nil {
			*errs = append(*errs, fieldError("TRUSTED_PROXIES", err))
		}
		c.TrustedProxies = prefixes
	}

	c.GuardPerMinute = defaultGuardPerMinute
	if raw := value(getenv, "GUARD_PER_MINUTE", ""); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 || n > maxGuardPerMinute {
			*errs = append(*errs, fieldError("GUARD_PER_MINUTE", fmt.Errorf("must be 0 (off) to %d", maxGuardPerMinute)))
		} else {
			c.GuardPerMinute = n
		}
	}

	fallbackMailer := MailerLog
	if production {
		fallbackMailer = ""
	}
	c.Mailer = value(getenv, "MAILER", fallbackMailer)
	switch {
	case c.Mailer == "":
		*errs = append(*errs, fieldError("MAILER", errors.New("is required in production (use smtp)")))
	case c.Mailer != MailerLog && c.Mailer != MailerFile && c.Mailer != MailerSMTP:
		*errs = append(*errs, fieldError("MAILER", errors.New("must be log, file or smtp")))
	case production && c.Mailer != MailerSMTP:
		*errs = append(*errs, fieldError("MAILER", errors.New("log and file are for development only: they expose sign-in codes")))
	}
	c.MailDir = value(getenv, "MAIL_DIR", "")
	if c.Mailer == MailerFile && c.MailDir == "" {
		*errs = append(*errs, fieldError("MAIL_DIR", errors.New("is required when MAILER=file")))
	}
	if c.Mailer == MailerSMTP {
		c.loadSMTP(getenv, errs)
	}
}

// loadSMTP reads the relay settings. Each missing one is named, so a half-filled .env is
// explained in one go instead of one restart at a time.
func (c *Config) loadSMTP(getenv func(string) string, errs *[]error) {
	c.SMTPHost = value(getenv, "SMTP_HOST", "")
	c.SMTPUsername = value(getenv, "SMTP_USERNAME", "")
	c.SMTPPassword = value(getenv, "SMTP_PASSWORD", "")
	c.MailFrom = value(getenv, "MAIL_FROM", "")
	c.MailFromName = value(getenv, "MAIL_FROM_NAME", "Readmely")
	for _, req := range []struct{ name, v string }{
		{"SMTP_HOST", c.SMTPHost}, {"SMTP_USERNAME", c.SMTPUsername},
		{"SMTP_PASSWORD", c.SMTPPassword}, {"MAIL_FROM", c.MailFrom},
	} {
		if req.v == "" {
			*errs = append(*errs, fieldError(req.name, errors.New("is required when MAILER=smtp")))
		}
	}
	if c.SMTPHost != "" && strings.ContainsAny(c.SMTPHost, " /\\@:") {
		*errs = append(*errs, fieldError("SMTP_HOST", errors.New("must be a host name only, for example smtp-relay.brevo.com")))
	}
	c.SMTPPort = defaultSMTPPort
	if raw := value(getenv, "SMTP_PORT", ""); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 65535 {
			*errs = append(*errs, fieldError("SMTP_PORT", errors.New("must be a port number, usually 587")))
		} else {
			c.SMTPPort = n
		}
	}
	if c.MailFrom != "" {
		if a, err := mail.ParseAddress(c.MailFrom); err != nil || a.Name != "" || strings.ContainsAny(c.MailFromName, "\r\n") {
			*errs = append(*errs, fieldError("MAIL_FROM", errors.New("must be one email address, for example hello@example.com")))
		}
	}
}

// loadOAuth reads the provider credentials. A provider is on only when both its id and secret
// are set; one without the other is a mistake worth stopping for, not a quietly disabled button.
func (c *Config) loadOAuth(getenv func(string) string, errs *[]error) {
	pair := func(idName, secretName string) (string, string) {
		id, secret := value(getenv, idName, ""), value(getenv, secretName, "")
		switch {
		case id == "" && secret == "":
			return "", ""
		case id == "":
			*errs = append(*errs, fieldError(idName, errors.New("is required when "+secretName+" is set")))
		case secret == "":
			*errs = append(*errs, fieldError(secretName, errors.New("is required when "+idName+" is set")))
		default:
			if err := validateCredential(id); err != nil {
				*errs = append(*errs, fieldError(idName, err))
			}
			if err := validateCredential(secret); err != nil {
				*errs = append(*errs, fieldError(secretName, err))
			}
		}
		return id, secret
	}
	c.GoogleClientID, c.GoogleClientSecret = pair("GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET")
	c.GitHubClientID, c.GitHubClientSecret = pair("GITHUB_CLIENT_ID", "GITHUB_CLIENT_SECRET")
}

// validateCredential accepts what providers issue (letters, digits and . _ - ~ + / =) and refuses
// spaces, quotes, control characters and the .env.example placeholder. Never echoes the value.
func validateCredential(v string) error {
	if len(v) < 8 || len(v) > 256 {
		return errors.New("must be 8 to 256 characters")
	}
	if strings.Contains(strings.ToLower(v), "change-me") {
		return errors.New("is still the placeholder from .env.example")
	}
	for _, r := range v {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-~+/=", r)
		if !ok {
			return errors.New("contains a character a provider credential does not use")
		}
	}
	return nil
}

// validateOrigin accepts scheme://host[:port] and nothing else. Production needs https.
func validateOrigin(v string, production bool) error {
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		return errors.New("must be an origin such as https://readmely.example")
	}
	if u.Scheme != "https" && (production || u.Scheme != "http") {
		if production {
			return errors.New("must use https in production")
		}
		return errors.New("scheme must be http or https")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("must be only scheme, host and port, with no path, query or credentials")
	}
	return nil
}

// parseProxies reads a comma-separated list of addresses or CIDR prefixes.
func parseProxies(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		var p netip.Prefix
		if strings.Contains(item, "/") {
			parsed, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, errors.New("must be a comma-separated list of IP addresses or CIDR prefixes")
			}
			p = parsed.Masked()
		} else {
			a, err := netip.ParseAddr(item)
			if err != nil {
				return nil, errors.New("must be a comma-separated list of IP addresses or CIDR prefixes")
			}
			a = a.Unmap()
			p = netip.PrefixFrom(a, a.BitLen())
		}
		if p.Bits() == 0 {
			return nil, errors.New("must not trust every address (/0)")
		}
		out = append(out, p)
	}
	if len(out) > maxTrustedProxies {
		return nil, fmt.Errorf("at most %d entries", maxTrustedProxies)
	}
	return out, nil
}

// LoadBase reads only the settings shared by every process (environment, log
// level, shutdown timeout). Workers use it until they need more.
func LoadBase(getenv func(string) string) (Config, error) {
	var c Config
	var errs []error
	c.loadBase(getenv, &errs)
	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return c, nil
}

func (c *Config) loadBase(getenv func(string) string, errs *[]error) {
	c.Env = value(getenv, "APP_ENV", EnvDevelopment)
	if c.Env != EnvDevelopment && c.Env != EnvProduction {
		*errs = append(*errs, fieldError("APP_ENV", errors.New("must be development or production")))
	}

	level := value(getenv, "LOG_LEVEL", "info")
	if err := c.LogLevel.UnmarshalText([]byte(level)); err != nil {
		*errs = append(*errs, fieldError("LOG_LEVEL", errors.New("must be debug, info, warn or error")))
	}

	c.ShutdownTimeout = 10 * time.Second
	if raw := value(getenv, "SHUTDOWN_TIMEOUT", ""); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 || d > maxShutdownTimeout {
			*errs = append(*errs, fieldError("SHUTDOWN_TIMEOUT", errors.New("must be a duration between 1ms and 60s, for example 10s")))
		} else {
			c.ShutdownTimeout = d
		}
	}
}

func value(getenv func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return fallback
}

func fieldError(key string, err error) error {
	return fmt.Errorf("%s: %w", key, err)
}

func validateAddr(v string) error {
	_, port, err := net.SplitHostPort(v)
	if err != nil {
		return errors.New("must be host:port, for example :8080")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 0 || n > 65535 {
		return errors.New("port must be between 0 and 65535")
	}
	return nil
}

// validateURL deliberately never wraps the parse error: net/url errors embed
// the whole input, which may contain a password.
func validateURL(v string, schemes ...string) error {
	u, err := url.Parse(v)
	if err != nil {
		return errors.New("must be a valid URL")
	}
	if !slices.Contains(schemes, u.Scheme) {
		return fmt.Errorf("scheme must be one of: %s", strings.Join(schemes, ", "))
	}
	if u.Host == "" {
		return errors.New("must include a host")
	}
	return nil
}

// validateServiceURL accepts a plain http(s) base URL: no credentials, query
// or fragment, so nothing secret can sit in it and it can be logged safely.
func validateServiceURL(v string) error {
	if err := validateURL(v, "http", "https"); err != nil {
		return err
	}
	u, _ := url.Parse(v) // already parsed successfully above
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("must not contain credentials, a query or a fragment")
	}
	return nil
}

// validateToken matches the AI service: at least 32 characters, and only
// characters that are safe in an HTTP header.
func validateToken(v string) error {
	if len(v) < minTokenLength {
		return fmt.Errorf("must be at least %d characters", minTokenLength)
	}
	for _, r := range v {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !ok {
			return errors.New("may only contain letters, digits, - and _")
		}
	}
	return nil
}

// LoadWorker reads what a background worker needs: the base settings and the database it works on.
func LoadWorker(getenv func(string) string) (Config, error) {
	var c Config
	var errs []error
	c.loadBase(getenv, &errs)
	c.DatabaseURL = value(getenv, "DATABASE_URL", "")
	if c.DatabaseURL == "" {
		errs = append(errs, fieldError("DATABASE_URL", errors.New("is required")))
	} else if err := validateURL(c.DatabaseURL, "postgres", "postgresql"); err != nil {
		errs = append(errs, fieldError("DATABASE_URL", err))
	}
	c.loadGitHubTokenKey(getenv, &errs)
	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return c, nil
}

// loadGitHubTokenKey reads the optional key that seals GitHub tokens: 32 bytes as 64 hex characters
// (openssl rand -hex 32). The same key must reach the API and the analysis worker.
func (c *Config) loadGitHubTokenKey(getenv func(string) string, errs *[]error) {
	c.GitHubTokenKey = value(getenv, "GITHUB_TOKEN_KEY", "")
	if c.GitHubTokenKey == "" {
		return
	}
	if len(c.GitHubTokenKey) != 64 {
		*errs = append(*errs, fieldError("GITHUB_TOKEN_KEY", errors.New("must be 64 hex characters (generate with: openssl rand -hex 32)")))
		return
	}
	if _, err := hex.DecodeString(c.GitHubTokenKey); err != nil {
		*errs = append(*errs, fieldError("GITHUB_TOKEN_KEY", errors.New("must be hex (generate with: openssl rand -hex 32)")))
	}
}
