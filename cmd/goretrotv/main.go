// Command goretrotv runs the Pace 2500N emulator and serves it to a browser.
package main

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ddunford/goretrotv/internal/app"
	"github.com/ddunford/goretrotv/internal/board"
	bcast "github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/config"
	"github.com/ddunford/goretrotv/internal/firmware"
	"github.com/ddunford/goretrotv/internal/httpx"
	"github.com/ddunford/goretrotv/internal/logging"
	"github.com/ddunford/goretrotv/internal/multiplex"
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

	// The broadcast is loaded and validated BEFORE the listener opens, for the
	// same reason the firmware is: a demo that serves a page and then discovers
	// its schedule is malformed has already told the boot gate it is fine.
	air, err := loadBroadcast(cfg, logger)
	if err != nil {
		return err
	}

	transport := web.NewTransport()
	box, ready, err := startBox(fw, cfg.SnapshotPath)
	if err != nil {
		return err
	}
	if err := publishFrame(box, transport); err != nil {
		return err
	}
	if ready {
		if err := transport.PushState("ready", "The box is ready. Press tv guide on the handset."); err != nil {
			return err
		}
	} else if err := transport.PushState("booting", "The box is starting its firmware."); err != nil {
		return err
	}
	handler, err := app.New(cfg, logger, transport)
	if err != nil {
		return err
	}
	go runMachine(ctx, box, ready, fw, cfg.SnapshotPath, transport, logger, air)

	if err := httpx.NewServer(cfg.HTTPAddr, handler, logger).Run(ctx); err != nil {
		return err
	}

	logger.Info("stopped")
	return nil
}

// startBox builds the machine this process serves: the verified
// post-acquisition snapshot where the operator configured one, and a cold
// board from flash where they did not. It is called once at startup and again
// for every reset, so both paths land in exactly the same state by
// construction rather than by two functions agreeing.
func startBox(images *firmware.Set, snapshotPath string) (*board.Runtime, bool, error) {
	box, err := board.New(images, true)
	if err != nil {
		return nil, false, err
	}
	if snapshotPath == "" {
		return box, false, nil
	}
	file, err := os.Open(snapshotPath) // #nosec G304 -- operator explicitly supplies this private snapshot path.
	if err != nil {
		return nil, false, fmt.Errorf("open machine snapshot: %w", err)
	}
	err = box.Restore(file)
	closeErr := file.Close()
	if err != nil {
		return nil, false, fmt.Errorf("restore machine snapshot: %w", err)
	}
	if closeErr != nil {
		return nil, false, closeErr
	}
	ready, err := acquiredSnapshot(box)
	if err != nil {
		return nil, false, err
	}
	if !ready {
		return nil, false, fmt.Errorf("machine snapshot is not the verified post-acquisition state")
	}
	return box, true, nil
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

// broadcast is everything the modelled multiplex needs, loaded once and shared
// by every box this process runs. It is nil when nothing is configured to go on
// air, which is a supported way to run rather than a failure.
type broadcastConfig struct {
	guide    *multiplex.Guide
	dict     *bcast.HuffmanDictionary
	clock    multiplex.InWorldClock
	schedule bcast.Schedule
}

// airSchedule is how often each rung of the carousel repeats, in instructions.
//
// The RATE IS NOT LOAD-BEARING and was measured not to be -- sixteen sections
// pushed by hand acquire a line-up with no rate involved -- so these are chosen
// for how quickly someone watching the demo sees the guide fill. The two
// SETTLES are the part that is load-bearing: the box programs its day-addressed
// listings request once, so a line-up that arrives before the clock has landed
// leaves it asking for the wrong day for ever.
//
// THE INDEX REPEATS FOR THE SAME REASON A CAROUSEL EXISTS AT ALL, and its rate IS load-bearing in
// a way the others are not. One index wave carries ONE LETTER, so a full alphabet takes about
// twenty periods -- and the list head a screen reads is only filled once its own letter has come
// round. Two million instructions puts a whole cycle inside forty million, comfortably before a
// viewer can walk the menus to A-Z LISTINGS.
//
// One letter per wave rather than the alphabet at once is not a throttle, it is the fix: every
// index section goes to the same PID and therefore the same section filter, and a wave is pushed
// inside a single Pump with no guest instructions between the pushes. Nineteen at once filled
// eight of twenty-six list heads and lost the rest, silently.
//
// THE EVENT RUNG COSTS NOTHING UNTIL SOMEONE WATCHES A CHANNEL. It transmits only while PID 0x0012
// is armed, and the box arms that when it TUNES -- not at acquisition, not with the guide open. It
// is here because the box asks for it by name and this port answered nothing on that filter for
// its whole life: eight million instructions is about a section pair per service every quarter of
// a second of guest time, which is the order a real multiplex repeats present/following at.
var airSchedule = bcast.Schedule{
	ClockPeriod:  20_000_000,
	LineupPeriod: 60_000_000,
	TitlePeriod:  60_000_000,
	IndexPeriod:  2_000_000,
	EventPeriod:  8_000_000,
	ClockSettle:  8_000_000,
	LineupSettle: 4_000_000,
	TitleSettle:  4_000_000,
}

// loadBroadcast reads the schedule and the dictionary, or reports that nothing
// will go on air and why.
//
// A schedule with no dictionary is REFUSED rather than run silently. Title
// sections cannot be built without it, so that combination is a box whose guide
// is empty -- indistinguishable, from the outside, from a demo deliberately run
// with no broadcast at all.
func loadBroadcast(cfg *config.Config, logger *slog.Logger) (*broadcastConfig, error) {
	if cfg.ListingsPath == "" {
		logger.Info("no broadcast configured; the box will boot with an empty guide",
			"reason", "GORETROTV_LISTINGS_PATH is empty")
		return nil, nil
	}
	guide, err := multiplex.LoadGuide(cfg.ListingsPath)
	if err != nil {
		return nil, err
	}
	if cfg.DictionaryPath == "" {
		return nil, fmt.Errorf("a schedule is configured at %s but GORETROTV_DICTIONARY_PATH is empty, "+
			"and title sections cannot be built without the dictionary; see dictionaries/MANIFEST.md",
			cfg.ListingsPath)
	}
	dict, err := bcast.LoadHuffmanDictionary(cfg.DictionaryPath)
	if err != nil {
		return nil, err
	}
	clock, err := inWorldClock(cfg.BroadcastDate)
	if err != nil {
		return nil, err
	}
	day := clock.Now()
	listings := guide.On(day)
	programmes := 0
	for _, service := range listings.Services {
		programmes += len(service.Programmes)
	}
	logger.Info("broadcast loaded",
		"schedule", guide.Source, "dated_schedules", guide.Dated(),
		"bouquet", listings.Bouquet, "channels", len(listings.Services), "programmes", programmes,
		"dictionary_entries", dict.Entries(),
		"broadcast_date", cfg.BroadcastDate, "in_world_now", day.Format("2006-01-02 15:04 MST"))
	return &broadcastConfig{guide: guide, dict: dict, clock: clock, schedule: airSchedule}, nil
}

// inWorldClock reads the broadcast date setting.
//
// "now" and a date are the only two answers, and anything else is refused by
// name rather than quietly treated as one of them: a typo that silently became
// "now" would move the demo off its 1998 evening and the only symptom would be
// an empty guide five days a week.
func inWorldClock(setting string) (multiplex.InWorldClock, error) {
	if strings.EqualFold(strings.TrimSpace(setting), "now") {
		return multiplex.LiveClock{}, nil
	}
	day, err := time.Parse("2006-01-02", strings.TrimSpace(setting))
	if err != nil {
		return nil, fmt.Errorf("GORETROTV_BROADCAST_DATE is %q; it must be \"now\" or a YYYY-MM-DD date", setting)
	}
	// 19:00 rather than midnight: a demo opened at any hour should show an
	// evening's television, not the small hours.
	return multiplex.FixedClock{At: day.Add(19 * time.Hour)}, nil
}

// transmitterFor builds the multiplex for one box, or nil when nothing is on
// air. A transmitter belongs to exactly one box: the carousel holds the line-up
// back until the clock has landed, and carrying that state across a reset would
// release it into a machine that had just forgotten the clock.
func transmitterFor(air *broadcastConfig, box *board.Runtime, logger *slog.Logger) *multiplex.Multiplex {
	if air == nil {
		return nil
	}
	transmitter, err := multiplex.New(box, air.guide, air.dict, air.clock, air.schedule)
	if err == nil {
		transmitter.OnReload(func(changed bool, problem error) {
			if problem != nil {
				// The broadcast carries on with the last good schedule, so
				// without this line the only symptom of a typo is that the
				// edit appears to have done nothing.
				logger.Error("the edited schedule was refused; the last good one stays on air",
					"err", problem)
				return
			}
			logger.Info("schedule reloaded", "schedule", air.guide.Source)
		})
		transmitter.OnAir(func(counts multiplex.Counters, requests []multiplex.TitleRequest) {
			pids := make([]string, 0, len(requests))
			for _, request := range requests {
				pids = append(pids, fmt.Sprintf("%#02x(MJD %d)", request.PID, request.MJD()))
			}
			// "filters: none" is the ordinary case rather than a fault: the
			// box only programmes a title filter for a day whose slot is one
			// of three, and the transmitter addresses the rest from its own
			// clock. Saying which of the two happened is the whole value of
			// this line to somebody looking at an empty guide.
			filters := strings.Join(pids, " ")
			if filters == "" {
				filters = "none (addressed from the clock)"
			}
			logger.Info("programmes on air",
				"clock_waves", counts.Clock, "lineup_waves", counts.Lineup,
				"title_waves", counts.Titles, "derived_waves", counts.TitlesDerived,
				"filters", filters)
		})
	}
	if err != nil {
		// The box is still worth running without a broadcast, so this is
		// reported and survived rather than returned: an emulator that refuses
		// to start because its television schedule is wrong has the priorities
		// of this project backwards.
		logger.Error("no broadcast for this box", "err", err)
		return nil
	}
	return transmitter
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

// stopCause says why one machine's instruction loop gave up the board.
type stopCause int

const (
	stopContext stopCause = iota // the process is shutting down
	stopReset                    // a browser asked for the box to be rebuilt
	stopHalt                     // the guest halted
)

// runMachine owns every board this process runs. A machine ends when the
// process stops, when a browser asks for a reset, or when the guest halts, and
// only the first of those ends this goroutine: a halted box that nothing can
// restart is the one case the reset control exists for, so the halt path waits
// for a reset instead of returning.
func runMachine(ctx context.Context, box *board.Runtime, ready bool, images *firmware.Set,
	snapshotPath string, transport *web.Transport, logger *slog.Logger, air *broadcastConfig) {
	for {
		cause, err := runInstructions(ctx, box, ready, transport, logger, transmitterFor(air, box, logger))
		switch cause {
		case stopContext:
			return
		case stopHalt:
			haltMachine(transport, logger, err)
			select {
			case <-ctx.Done():
				return
			case <-transport.Resets():
			}
		case stopReset:
		}
		// Keys queued against the machine that is going away must not arrive at
		// the one replacing it.
		if err := transport.DrainKeys(func(uint8, uint8) error { return nil }); err != nil {
			haltMachine(transport, logger, err)
			return
		}
		next, nextReady, err := startBox(images, snapshotPath)
		if err != nil {
			// Say so and stop rather than leave an unreachable machine behind a
			// control that claims to fix it.
			haltMachine(transport, logger, fmt.Errorf("rebuild the box: %w", err))
			return
		}
		box, ready = next, nextReady
		logger.Info("box reset", "restored", snapshotPath != "", "ready", ready)
		if err := publishFrame(box, transport); err != nil {
			haltMachine(transport, logger, err)
			return
		}
		phase, reason := resetState(ready)
		if err := transport.PushState(phase, reason); err != nil {
			haltMachine(transport, logger, err)
			return
		}
	}
}

// resetState names the host intervention in the words the page shows. A reset
// is the host rebuilding the machine, not the guest doing anything, and this
// project reports host interventions explicitly rather than letting them read
// as firmware behaviour. The two paths also leave the box in visibly different
// places, so the viewer is told which one they got.
func resetState(ready bool) (string, string) {
	if ready {
		return "ready", "The box was reset and restored to its startup state. Press tv guide on the handset."
	}
	return "booting", "The box was reset and is cold-starting from its flash."
}

// runInstructions drives one board until the process stops, a reset is asked
// for, or the guest halts. It is the board's only owner for that lifetime.
func runInstructions(ctx context.Context, box *board.Runtime, ready bool,
	transport *web.Transport, logger *slog.Logger, transmitter *multiplex.Multiplex) (stopCause, error) {
	const inputInterval = 1024
	const frameInterval = 500_000
	const stateInterval = 4_000_000
	for {
		count := box.Machine.Retired
		if count%inputInterval == 0 {
			// The cancellation is returned with the cause rather than dropped.
			// stopContext is the process shutting down and its caller has
			// nothing to report, but a loop that swallows the only error it
			// was handed is the shape that hides a real one later.
			if err := ctx.Err(); err != nil {
				return stopContext, err
			}
			// Taken here, at the same safe point as input, because Runtime
			// admits no owner but this loop.
			if transport.TakeReset() {
				return stopReset, nil
			}
			if err := transport.DrainKeys(func(raw, source uint8) error {
				logger.Debug("handset key queued on CSI", "retired", count, "raw", raw, "source", source)
				return box.CSI.Key(raw, source)
			}); err != nil {
				return stopHalt, err
			}
			// On the same 1024-instruction boundary as input, because the board
			// admits no owner but this loop. Pump is cheap when nothing is due:
			// it compares against the carousel's next due instruction and
			// returns.
			if transmitter != nil {
				if err := transmitter.Pump(count); err != nil {
					return stopHalt, err
				}
			}
		}
		if err := box.Step(); err != nil {
			return stopHalt, err
		}
		count = box.Machine.Retired
		if count%5_000_000 == 0 {
			if logger.Enabled(ctx, slog.LevelDebug) {
				frame, err := box.Compose()
				if err != nil {
					return stopHalt, err
				}
				logger.Debug("guest progress", "retired", count, "pc", box.Machine.Core.PC,
					"csi_pending", box.CSI.Pending(), "frame_hash", fmt.Sprintf("%08X", statehash.HashBytes(frame.Pix)))
			}
		}
		if count%frameInterval == 0 {
			if err := publishFrame(box, transport); err != nil {
				return stopHalt, err
			}
		}
		if !ready && count%stateInterval == 0 {
			evidence, err := readBootEvidence(box)
			if err != nil {
				return stopHalt, err
			}
			phase, reason := coldStatus(evidence)
			if err := transport.PushState(phase, reason); err != nil {
				return stopHalt, err
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
