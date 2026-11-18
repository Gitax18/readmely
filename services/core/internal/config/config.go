// Package config loads and validates process configuration from the environment.
//
// Validation reports every problem at once and names the variable, but never
// includes a value, because values can contain passwords.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
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
)

// Mailers. Only development may use the log and file mailers: they put codes on disk or in
// logs. No production mailer exists yet (plans/phase-2a-email-login.md), so production refuses
// to start until one is added.
const (
	MailerLog  = "log"  // writes each email to the log, code included (development only)
	MailerFile = "file" // writes each email as a JSON file to MAIL_DIR (development and browser tests)
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

	// Sign-in (Phase 2a).
	AuthCodeKey    string         // HMAC key for login codes and CSRF tokens; never logged
	WebOrigin      string         // the only origin allowed to make state-changing requests
	TrustedProxies []netip.Prefix // peers whose X-Forwarded-For is believed (the web server)
	Mailer         string         // MailerLog or MailerFile
	MailDir        string         // where MailerFile writes
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

	fallbackMailer := MailerLog
	if production {
		fallbackMailer = ""
	}
	c.Mailer = value(getenv, "MAILER", fallbackMailer)
	switch {
	case c.Mailer == "":
		*errs = append(*errs, fieldError("MAILER", errors.New("is required in production, and no production mailer exists yet")))
	case c.Mailer != MailerLog && c.Mailer != MailerFile:
		*errs = append(*errs, fieldError("MAILER", errors.New("must be log or file")))
	case production:
		*errs = append(*errs, fieldError("MAILER", errors.New("log and file are for development only: they expose sign-in codes")))
	}
	c.MailDir = value(getenv, "MAIL_DIR", "")
	if c.Mailer == MailerFile && c.MailDir == "" {
		*errs = append(*errs, fieldError("MAIL_DIR", errors.New("is required when MAILER=file")))
	}
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
