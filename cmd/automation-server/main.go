package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KPO-Tech/seshat/internal/automation"
	"github.com/KPO-Tech/seshat/internal/db"
	pkgautomation "github.com/KPO-Tech/seshat/pkg/automation"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seshat-automation: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := log.New(os.Stderr, "[seshat-automation] ", log.LstdFlags)

	if cfg.APIKey == "" {
		return fmt.Errorf("SESHAT_AUTOMATION_API_KEY is not set — refusing to start without a master key (admin endpoints would be unauthenticated)")
	}

	// ─── Database ──────────────────────────────────────────────────────────────
	database, err := db.Open(context.Background(), db.Config{
		Driver: db.DriverSQLite,
		DSN:    cfg.DBPath,
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	if err := database.Initialize(context.Background()); err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	if err := database.InitializeAutomationDaemon(context.Background()); err != nil {
		return fmt.Errorf("initialize automation daemon schema: %w", err)
	}

	// ─── Runner ────────────────────────────────────────────────────────────────
	runnerCfg, err := pkgautomation.RunnerConfigFromEnv(cfg.Model)
	if err != nil {
		return fmt.Errorf("load runner config: %w", err)
	}
	runner, err := automation.NewRunner(runnerCfg)
	if err != nil {
		return fmt.Errorf("create runner: %w", err)
	}
	defer runner.Close()

	// ─── Scheduler ─────────────────────────────────────────────────────────────
	store := automation.NewDBJobStore(database)
	sched := automation.NewJobScheduler(store, runner)
	sched.SetLogger(logger)

	// If SESHAT_AI_URL is set, resolve per-user LLM creds from seshat-ai at
	// execution time instead of using the daemon's own env/config.
	if rcc := NewRuntimeConfigClient(cfg.SeshatAIURL, cfg.APIKey); rcc != nil {
		logger.Printf("runtime-config resolver enabled → %s", cfg.SeshatAIURL)
		sched.SetRunnerResolver(func(ctx context.Context, ownerID, agentSlug, modelOverride string) (*automation.Runner, automation.AgentConfig, error) {
			rc, err := rcc.Fetch(ctx, ownerID, agentSlug)
			if err != nil {
				logger.Printf("runtime-config fetch failed for %s: %v — falling back to env config", ownerID, err)
				return runner, automation.AgentConfig{}, nil // graceful fallback
			}
			resolvedCfg, agentCfg, err := rc.ToRunnerConfig(modelOverride)
			if err != nil {
				return nil, automation.AgentConfig{}, err
			}
			r, err := automation.NewRunner(resolvedCfg)
			return r, agentCfg, err
		})
	}

	// ─── HTTP server ───────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      newServer(sched, database, cfg.APIKey),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run scheduler in background
	schedErrCh := make(chan error, 1)
	go func() {
		logger.Printf("scheduler started")
		schedErrCh <- sched.Run(ctx)
	}()

	// Run HTTP server in background
	httpErrCh := make(chan error, 1)
	go func() {
		logger.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			httpErrCh <- err
		}
		close(httpErrCh)
	}()

	// Wait for signal or fatal error
	select {
	case <-ctx.Done():
		logger.Printf("shutting down...")
	case err := <-httpErrCh:
		return fmt.Errorf("http server: %w", err)
	case err := <-schedErrCh:
		if err != nil && err != context.Canceled {
			return fmt.Errorf("scheduler: %w", err)
		}
	}

	// Graceful HTTP shutdown (10s max)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("graceful shutdown error: %v", err)
	}
	return nil
}
