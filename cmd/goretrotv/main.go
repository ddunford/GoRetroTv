// Command goretrotv runs the Pace 2500N emulator and serves it to a browser.
package main

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ddunford/goretrotv/internal/app"
	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/config"
	"github.com/ddunford/goretrotv/internal/firmware"
	"github.com/ddunford/goretrotv/internal/httpx"
	"github.com/ddunford/goretrotv/internal/logging"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
	"github.com/ddunford/goretrotv/internal/version"
	"github.com/ddunford/goretrotv/internal/web"
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

	transport := web.NewTransport()
	box, err := board.New(fw, true)
	if err != nil {
		return err
	}
	ready := false
	if cfg.SnapshotPath != "" {
		file, err := os.Open(cfg.SnapshotPath) // #nosec G304 -- operator explicitly supplies this private snapshot path.
		if err != nil {
			return fmt.Errorf("open machine snapshot: %w", err)
		}
		err = box.Restore(file)
		closeErr := file.Close()
		if err != nil {
			return fmt.Errorf("restore machine snapshot: %w", err)
		}
		if closeErr != nil {
			return closeErr
		}
		ready, err = acquiredSnapshot(box)
		if err != nil {
			return err
		}
		if !ready {
			return fmt.Errorf("machine snapshot is not the verified post-acquisition state")
		}
	}
	if err := publishFrame(box, transport); err != nil {
		return err
	}
	if ready {
		if err := transport.PushState("ready", "The box is ready. Press sky on the handset."); err != nil {
			return err
		}
	} else if err := transport.PushState("booting", "The box is starting its firmware."); err != nil {
		return err
	}
	handler, err := app.New(cfg, logger, transport)
	if err != nil {
		return err
	}
	go runMachine(ctx, box, transport, ready, logger)

	if err := httpx.NewServer(cfg.HTTPAddr, handler, logger).Run(ctx); err != nil {
		return err
	}

	logger.Info("stopped")
	return nil
}

func acquiredSnapshot(box *board.Runtime) (bool, error) {
	if box.Machine.Retired != 1_100_000_000 || !box.Machine.Handoff.Done() || !box.Machine.SkyGates.Done() {
		return false, nil
	}
	hasher, err := statehash.New(box.RAM)
	if err != nil {
		return false, err
	}
	got := hasher.Hash(box.Machine.Core.State())
	if err := hasher.Err(); err != nil {
		return false, err
	}
	return got == 0x04E99A24, nil
}

func publishFrame(box *board.Runtime, transport *web.Transport) error {
	frame, err := box.Compose()
	if err != nil {
		return err
	}
	if frame.Rect != image.Rect(0, 0, web.FrameWidth, web.FrameHeight) {
		full := image.NewPaletted(image.Rect(0, 0, web.FrameWidth, web.FrameHeight), frame.Palette)
		for y := 0; y < frame.Rect.Dy() && y < web.FrameHeight; y++ {
			copy(full.Pix[y*full.Stride:y*full.Stride+min(frame.Rect.Dx(), web.FrameWidth)],
				frame.Pix[y*frame.Stride:y*frame.Stride+min(frame.Rect.Dx(), web.FrameWidth)])
		}
		frame = full
	}
	return transport.PushFrame(frame)
}

func runMachine(ctx context.Context, box *board.Runtime, transport *web.Transport, ready bool, logger *slog.Logger) {
	const inputInterval = 1024
	const frameInterval = 500_000
	const stateInterval = 4_000_000
	for {
		count := box.Machine.Retired
		if count%inputInterval == 0 {
			if err := ctx.Err(); err != nil {
				return
			}
			if err := transport.DrainKeys(func(raw, source uint8) error {
				logger.Debug("handset key queued on CSI", "retired", count, "raw", raw, "source", source)
				return box.CSI.Key(raw, source)
			}); err != nil {
				haltMachine(transport, logger, err)
				return
			}
		}
		if err := box.Step(); err != nil {
			haltMachine(transport, logger, err)
			return
		}
		count = box.Machine.Retired
		if count%5_000_000 == 0 {
			logger.Debug("guest progress", "retired", count, "pc", box.Machine.Core.PC)
		}
		if count%frameInterval == 0 {
			if err := publishFrame(box, transport); err != nil {
				haltMachine(transport, logger, err)
				return
			}
		}
		if !ready && count%stateInterval == 0 && box.Machine.Handoff.Done() {
			if err := transport.PushState("channel-list", "The firmware is rebuilding its channel list."); err != nil {
				haltMachine(transport, logger, err)
				return
			}
		}
	}
}

func haltMachine(transport *web.Transport, logger *slog.Logger, err error) {
	logger.Error("guest halted", "err", err)
	if stateErr := transport.PushState("halted", err.Error()); stateErr != nil {
		logger.Error("publish guest halt", "err", stateErr)
	}
}
