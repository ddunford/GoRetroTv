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
	"github.com/ddunford/goretrotv/internal/firmware"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The firmware is verified BEFORE anything else initialises, and a refusal here returns
	// rather than degrading. Running a different ROM is not a degraded mode: it is a machine
	// whose every later disagreement with the oracle is unattributable, and the wasted time
	// lands on whoever is reading those disagreements weeks later rather than here.
	//
	// It happens before the listener opens on purpose. A process that served /health while
	// holding no verified firmware would answer "ok" to the boot gate, which is the one reader
	// that must never be told this machine is fine.
	fw, err := firmware.Load(ctx, cfg.FirmwareDir)
	if err != nil {
		return err
	}
	logger.Info("firmware verified",
		"dir", fw.Dir,
		"manifest", fw.Manifest.Path,
		"images", len(fw.Manifest.Images),
		"u202_bytes", len(fw.U202),
		"u203_bytes", len(fw.U203),
		"application_ram_bytes", len(fw.ApplicationRAM),
	)

	handler, err := app.New(cfg, logger)
	if err != nil {
		return err
	}

	if err := httpx.NewServer(cfg.HTTPAddr, handler, logger).Run(ctx); err != nil {
		return err
	}

	logger.Info("stopped")
	return nil
}
