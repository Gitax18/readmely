// Command api is the Readmely core HTTP API.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	netmail "net/mail"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/Gitax18/readmely/services/core/internal/auth"
	"github.com/Gitax18/readmely/services/core/internal/clientip"
	"github.com/Gitax18/readmely/services/core/internal/config"
	"github.com/Gitax18/readmely/services/core/internal/credits"
	"github.com/Gitax18/readmely/services/core/internal/diagram"
	"github.com/Gitax18/readmely/services/core/internal/github"
	"github.com/Gitax18/readmely/services/core/internal/githubaccess"
	"github.com/Gitax18/readmely/services/core/internal/health"
	"github.com/Gitax18/readmely/services/core/internal/logging"
	"github.com/Gitax18/readmely/services/core/internal/mail"
	"github.com/Gitax18/readmely/services/core/internal/oauth"
	"github.com/Gitax18/readmely/services/core/internal/projects"
	"github.com/Gitax18/readmely/services/core/internal/ratelimit"
	"github.com/Gitax18/readmely/services/core/internal/readme"
	"github.com/Gitax18/readmely/services/core/internal/secretbox"
	"github.com/Gitax18/readmely/services/core/internal/server"
	"github.com/Gitax18/readmely/services/core/internal/status"
)

const (
	readinessTimeout = 2 * time.Second
	// The status report is shared for this long (audit F3); /readyz is never cached.
	statusCacheTTL = 5 * time.Second

	// Email sending (audit F15): a few senders, a bounded backlog, and a per-send deadline.
	mailSenders     = 4
	mailQueueSize   = 256
	mailSendTimeout = 15 * time.Second
	// How often ended sessions and old sign-in codes are removed.
	authCleanupEvery = time.Hour
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run is main without the process-level pieces, so it can be tested.
// Exit codes: 0 clean stop, 1 runtime failure, 2 bad configuration.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	// `api healthcheck` lets a container probe itself without curl in the image.
	if len(args) > 0 && args[0] == "healthcheck" {
		return healthcheck(ctx, getenv, stderr)
	}

	cfg, err := config.Load(getenv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "configuration error:\n%v\n", err)
		return 2
	}
	logger := logging.New(stdout, cfg.LogLevel, "api")

	// Neither client connects eagerly, so the API starts even when a
	// dependency is down and reports it through /readyz instead of crashing.
	// Parse errors are not logged: they can echo the connection string.
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("DATABASE_URL could not be used to create a connection pool")
		return 2
	}
	defer pool.Close()

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("REDIS_URL could not be parsed")
		return 2
	}
	rdb := redis.NewClient(redisOpts)
	defer func() { _ = rdb.Close() }()

	checks := health.New(logger, readinessTimeout,
		health.Check{Name: "postgres", Pinger: pool},
		health.Check{Name: "redis", Pinger: redisPinger{rdb}},
	)
	statusHandler := status.NewHandler(checks, status.NewAIClient(logger, cfg.AIServiceURL, cfg.AIServiceToken)).Cached(statusCacheTTL)

	mailer, err := newMailer(cfg, logger)
	if err != nil {
		logger.Error("MAILER could not be used")
		return 2
	}
	mailQueue := mail.NewQueue(mailer, logger, mailSenders, mailQueueSize, mailSendTimeout)
	authService := auth.NewService(pool, ratelimit.New(rdb, "rm:rl:"), mailQueue, auth.NewKeys(cfg.AuthCodeKey), logger)
	authService.SetWebOrigin(cfg.WebOrigin)
	providers := map[string]oauth.Provider{}
	if cfg.GoogleClientID != "" {
		providers[oauth.Google] = oauth.NewGoogle(cfg.GoogleClientID, cfg.GoogleClientSecret, nil)
	}
	if cfg.GitHubClientID != "" {
		providers[oauth.GitHub] = oauth.NewGitHub(cfg.GitHubClientID, cfg.GitHubClientSecret, nil)
	}
	// Private repositories need the key that seals people's GitHub tokens; without it the feature is off.
	var access *githubaccess.Store
	if cfg.GitHubTokenKey != "" {
		box, err := secretbox.NewFromHex(cfg.GitHubTokenKey)
		if err != nil {
			logger.Error("invalid GITHUB_TOKEN_KEY", "error", err)
			return 2
		}
		access = githubaccess.New(pool, box)
	}
	authService.EnableOAuth(auth.OAuthConfig{Providers: providers, Store: oauth.NewStore(rdb, "rm:"), WebOrigin: cfg.WebOrigin, Access: access})
	logger.Info("provider sign-in", "providers", authService.Providers())
	authHandler := auth.NewHandler(authService, clientip.New(cfg.TrustedProxies), cfg.WebOrigin, logger)
	githubClient := github.NewClient(cfg.GitHubAPIToken)
	projectService := projects.NewService(pool, ratelimit.New(rdb, "rm:rl:"), githubClient, logger)
	if access != nil {
		projectService.EnablePrivate(access, func(token string) projects.UserRepos { return githubClient.WithToken(token) })
	}
	projectHandler := projects.NewHandler(projectService)
	authHandler.AddRoutes(projectHandler.Routes)
	authHandler.AddRoutes(credits.NewHandler(pool).Routes)
	authHandler.AddRoutes(diagram.NewHandler(diagram.NewService(pool, ratelimit.New(rdb, "rm:rl:"))).Routes)
	authHandler.AddRoutes(readme.NewHandler(readme.NewService(pool, ratelimit.New(rdb, "rm:rl:"))).Routes)

	var lc net.ListenConfig
	publicLn, err := lc.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		logger.Error("cannot listen", "addr", cfg.HTTPAddr, "error", err)
		return 1
	}
	internalLn, err := lc.Listen(ctx, "tcp", cfg.InternalAddr)
	if err != nil {
		_ = publicLn.Close()
		logger.Error("cannot listen", "addr", cfg.InternalAddr, "error", err)
		return 1
	}
	logger.Info("api listening",
		"addr", publicLn.Addr().String(), "internal_addr", internalLn.Addr().String(), "env", cfg.Env)

	// Both servers stop together: on shutdown, or as soon as either one fails.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go authService.RunCleanup(runCtx, authCleanupEvery)
	errs := make(chan error, 2)
	go func() {
		errs <- server.Run(runCtx, server.New(server.NewPublicRouter(logger, checks, authHandler.Mount)), publicLn, cfg.ShutdownTimeout)
	}()
	go func() {
		errs <- server.Run(runCtx, server.New(server.NewInternalRouter(logger, checks, statusHandler.Get)), internalLn, cfg.ShutdownTimeout)
	}()
	var runErr error
	for range 2 {
		if err := <-errs; err != nil && runErr == nil {
			runErr = err
			cancel()
		}
	}
	// Servers are stopped, so no new email can be queued: send what is waiting.
	drainCtx, drainCancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	defer drainCancel()
	if err := mailQueue.Close(drainCtx); err != nil {
		logger.Error("email queue did not drain", "error", err)
	}
	if runErr != nil {
		logger.Error("api stopped with an error", "error", runErr)
		return 1
	}
	logger.Info("api stopped")
	return 0
}

// newMailer builds the configured mailer. Errors never carry the SMTP password.
func newMailer(cfg config.Config, logger *slog.Logger) (mail.Mailer, error) {
	if cfg.Mailer != config.MailerSMTP {
		return mail.New(cfg.Mailer, cfg.MailDir, logger)
	}
	return mail.NewSMTP(mail.SMTPConfig{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword,
		From: netmail.Address{Name: cfg.MailFromName, Address: cfg.MailFrom},
	})
}

type redisPinger struct{ c *redis.Client }

func (p redisPinger) Ping(ctx context.Context) error { return p.c.Ping(ctx).Err() }

// healthcheck probes the local /healthz and returns the exit code.
func healthcheck(ctx context.Context, getenv func(string) string, stderr io.Writer) int {
	addr := getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "healthcheck: HTTP_ADDR is not host:port")
		return 1
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort("127.0.0.1", port)+"/healthz", http.NoBody)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "healthcheck: bad request")
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "healthcheck: not reachable")
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(stderr, "healthcheck: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
