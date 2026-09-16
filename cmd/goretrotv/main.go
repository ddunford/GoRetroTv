// Command goretrotv runs the Pace 2500N emulator and serves it to a browser.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ddunford/goretrotv/internal/app"
	"github.com/ddunford/goretrotv/internal/config"
	"github.com/ddunford/goretrotv/internal/httpx"
	"github.com/ddunford/goretrotv/internal/logging"
	"github.com/ddunford/goretrotv/internal/version"
)

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet when config or logging setup is what failed, so this
		// path deliberately uses the default logger rather than assuming ours is installed.
		slog.Error("goretrotv failed to start", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger, err := logging.New(os.Stderr, cfg.LogLevel, logging.Format(cfg.LogFormat))
	if err != nil {
		return err
	}
	logger = logger.With("service", cfg.ServiceName)
	slog.SetDefault(logger)

	logger.Info("starting",
		"version", version.Version,
		"commit", version.Commit,
		"built", version.Date,
		"env", cfg.Env,
		"pprof", cfg.EnablePprof,
	)

	handler, err := app.New(cfg, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := httpx.NewServer(cfg.HTTPAddr, handler, logger).Run(ctx); err != nil {
		return err
	}

	logger.Info("stopped")
	return nil
}
