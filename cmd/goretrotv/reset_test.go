package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/firmware"
	"github.com/ddunford/goretrotv/internal/web"
)

const privateSnapshot = "post-acquisition.snapshot"

// privateBox loads the real firmware and the verified post-acquisition
// snapshot, or skips: neither is redistributable, so a machine without them
// must report honestly rather than silently test nothing.
func privateBox(t *testing.T) (*firmware.Set, string) {
	t.Helper()
	dir := filepath.Join("..", "..", "firmware")
	if _, err := os.Stat(filepath.Join(dir, firmware.FileU202)); os.IsNotExist(err) {
		t.Skip("private firmware is not installed")
	}
	snapshot := filepath.Join("..", "..", "snapshots", privateSnapshot)
	if _, err := os.Stat(snapshot); os.IsNotExist(err) {
		t.Skip("private post-acquisition snapshot is not installed")
	}
	images, err := firmware.Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return images, snapshot
}

// waitForState reads state messages until one satisfies match, so every
// assertion is about what the browser is actually told rather than about
// internal state. Matching on the REASON matters: the transport replays the
// current state to each new socket, so a bare phase match happily accepts the
// state that was already there before the reset was ever asked for.
func waitForState(t *testing.T, ctx context.Context, conn *websocket.Conn,
	what string, match func(phase, reason string) bool) string {
	t.Helper()
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %s: %v", what, err)
		}
		if typ != websocket.MessageText {
			continue
		}
		var message struct {
			Type   string `json:"type"`
			Phase  string `json:"phase"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(data, &message); err != nil {
			t.Fatal(err)
		}
		if message.Type == "state" && match(message.Phase, message.Reason) {
			return message.Reason
		}
	}
}

// The halted box is the case the reset control exists for, and before this
// change there was no way back from it: haltMachine pushed its state and the
// instruction goroutine returned for good. The halt here is a real one -- an
// illegal instruction written at the guest's own program counter, the same
// fault a wrong device model produces -- not a simulated flag.
func TestResetRestartsAHaltedBoxAndSaysWhatItDid(t *testing.T) {
	images, snapshot := privateBox(t)
	box, ready, err := startBox(images, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("the private snapshot did not restore to the ready state")
	}
	// KSEG0 at the restored program counter maps straight to physical DRAM.
	pc := box.Machine.Core.PC
	box.RAM.Write(pc&0x1fffffff, bus.Word, 0xfc000000)

	transport := web.NewTransport()
	if err := transport.PushState("ready", "The box is ready. Press tv guide on the handset."); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(transport)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	go runMachine(ctx, box, ready, images, snapshot, "", transport, logger, nil)

	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(4 << 20)

	haltReason := waitForState(t, ctx, conn, "the halt",
		func(phase, _ string) bool { return phase == "halted" })
	if haltReason == "" {
		t.Fatal("the halted box gave the browser no reason")
	}

	// The reset must be accepted in the halted phase, which is where the
	// handset's own gate refuses everything.
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"reset","version":4}`)); err != nil {
		t.Fatal(err)
	}
	reason := waitForState(t, ctx, conn, "the rebuilt box", func(phase, reason string) bool {
		return phase == "ready" && strings.Contains(reason, "reset")
	})
	if !strings.Contains(reason, "restored to its startup state") {
		t.Fatalf("the reset did not say which of the two paths it took: %q", reason)
	}
}

// A reset on a WORKING box must still rebuild it, and the rebuilt machine has
// to run: a restore that landed a frozen board would look identical on the
// page for as long as anyone watched the status line.
//
// Every observation here goes through the transport, never through the board.
// Runtime says only its instruction-loop owner may touch it, and an earlier
// version of this test read box.Machine.Retired from the test goroutine to see
// whether the machine was moving -- which the race detector caught, and which
// was the test breaking the very contract the product is built on.
func TestResetRebuildsARunningBoxAndItKeepsRetiring(t *testing.T) {
	images, snapshot := privateBox(t)
	box, ready, err := startBox(images, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	transport := web.NewTransport()
	if err := transport.PushState("ready", "The box is ready. Press tv guide on the handset."); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(transport)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	go runMachine(ctx, box, ready, images, snapshot, "", transport, logger, nil)

	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(4 << 20)

	// A second viewer, watching the same box. One machine serves everyone --
	// per-viewer boxes are a rejected decision -- so a reset is not a private
	// act and the other page must be told it happened.
	watcher, watcherResponse, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if watcherResponse.Body != nil {
		defer watcherResponse.Body.Close()
	}
	defer watcher.Close(websocket.StatusNormalClosure, "")
	watcher.SetReadLimit(4 << 20)

	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"reset","version":4}`)); err != nil {
		t.Fatal(err)
	}
	waitForState(t, ctx, conn, "the rebuilt box", func(phase, reason string) bool {
		return phase == "ready" && strings.Contains(reason, "reset")
	})
	waitForState(t, ctx, watcher, "the other viewer being told", func(phase, reason string) bool {
		return phase == "ready" && strings.Contains(reason, "reset")
	})

	// An idle box publishes NOTHING: PushFrame drops an image identical to
	// the last one, and a ready machine pushes no state. So "frames arrive"
	// is not a liveness signal here -- the first version of this test waited
	// for frames that correctly never came. Pressing sky is the signal,
	// because it makes the guest draw, and it proves the rebuilt board runs,
	// takes input and reaches the compositor: the whole path, the way a
	// viewer would find out.
	if err := conn.Write(ctx, websocket.MessageText,
		[]byte(`{"type":"key","version":4,"raw":125,"source":0}`)); err != nil {
		t.Fatal(err)
	}
	for frames := 0; frames < 1; {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("the rebuilt box drew nothing when sky was pressed: %v", err)
		}
		if typ != websocket.MessageText {
			continue
		}
		var message struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &message); err != nil {
			t.Fatal(err)
		}
		if message.Type == "frame" {
			frames++
		}
	}
}

func TestStartBoxRefusesASnapshotThatIsNotTheVerifiedState(t *testing.T) {
	images, _ := privateBox(t)
	path := filepath.Join(t.TempDir(), privateSnapshot)
	if err := os.WriteFile(path, []byte("not a machine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := startBox(images, path); err == nil {
		t.Fatal("a corrupt snapshot produced a box")
	}
}

var _ = http.StatusOK
