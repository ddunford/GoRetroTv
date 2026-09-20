package board_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/firmware"
)

// BenchmarkStep measures the instruction loop the browser and every gate run
// on: a real board restored to the post-acquisition snapshot, stepped with no
// hooks, no transport and no frame composition.
//
// It exists because this project had no throughput gate at all, and a guest
// running an order of magnitude under its NFR looked exactly like a guest
// running correctly -- the box still boots, still draws, still answers keys,
// just slowly enough that a 45-second test times out and a viewer decides the
// page is broken. A number nobody measures is a number that drifts.
//
// Recorded readings on the development host (12-core, Go 1.27.1), so a later
// run has something to compare against rather than a bare figure:
//
//	2026-09-20  206.0 ns/instruction, 1 alloc/op   with the per-instruction heap
//	                                                allocation in StepWithHooks
//	2026-09-20   94.75 ns/instruction, 0 allocs/op  after it went: 2.17x, and
//	                                                more than the profile's flat
//	                                                attribution predicted,
//	                                                because 51 bytes an
//	                                                instruction also fed the
//	                                                collector
//
// 94.75 ns is ~10.6M instructions/sec, which is still UNDER the 15M/s NFR. The
// remaining time is the CPU core, the bus and the icount clock, with no single
// defect left of the kind this benchmark was written for.
//
// Run it with:
//
//	go test ./internal/board -run XXX -bench BenchmarkStep -benchtime 20000000x
func BenchmarkStep(b *testing.B) {
	dir := filepath.Join("..", "..", "firmware")
	if _, err := os.Stat(filepath.Join(dir, firmware.FileU202)); os.IsNotExist(err) {
		b.Skip("private firmware is not installed")
	}
	snapshot := filepath.Join("..", "..", "snapshots", "post-acquisition.snapshot")
	if _, err := os.Stat(snapshot); os.IsNotExist(err) {
		b.Skip("private post-acquisition snapshot is not installed")
	}
	images, err := firmware.Load(context.Background(), dir)
	if err != nil {
		b.Fatal(err)
	}
	box, err := board.New(images, true)
	if err != nil {
		b.Fatal(err)
	}
	file, err := os.Open(snapshot)
	if err != nil {
		b.Fatal(err)
	}
	if err := box.Restore(file); err != nil {
		b.Fatal(err)
	}
	if err := file.Close(); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := box.Step(); err != nil {
			b.Fatal(err)
		}
	}
}

// TestStepDoesNotAllocate is the part of the benchmark that can fail. A
// benchmark records a number nobody reads on a green run; this asserts the
// property that number came from, so the allocation cannot come back
// unnoticed. It is the whole reason the guest ran at a fifth of its NFR:
// StepWithHooks stored the address of its parameter, so every emulated
// instruction went through the heap.
func TestStepDoesNotAllocate(t *testing.T) {
	dir := filepath.Join("..", "..", "firmware")
	if _, err := os.Stat(filepath.Join(dir, firmware.FileU202)); os.IsNotExist(err) {
		t.Skip("private firmware is not installed")
	}
	images, err := firmware.Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	box, err := board.New(images, true)
	if err != nil {
		t.Fatal(err)
	}
	// A cold board is enough: this is about the step path's own allocations,
	// not about which guest code happens to be running.
	if allocs := testing.AllocsPerRun(2000, func() {
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}); allocs > 0 {
		t.Fatalf("Step allocates %.1f times per emulated instruction; it must not allocate at all", allocs)
	}
}
