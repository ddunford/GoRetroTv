package cpu_test

import (
	"bytes"
	"testing"

	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

func TestCPUSnapshotRestoresPendingBranchAndISA(t *testing.T) {
	c, _ := machine(t, ri(4, 0, 0, 2), ri(9, 16, 16, 1), 0, 0)
	c.GPR[16] = 41
	c.GPR[24] = 7
	c.COP0[12] = 0x8001
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if !c.HasPendingBranch() {
		t.Fatal("fixture did not arm a branch")
	}
	state, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, _ := machine(t, ri(4, 0, 0, 2), ri(9, 16, 16, 1), 0, 0)
	restored.GPR[16] = 99
	restored.PC = codeBase + 0x40
	if err := restored.Restore(state); err != nil {
		t.Fatal(err)
	}
	again, err := restored.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(state, again) {
		t.Fatal("restored CPU did not reproduce exact state")
	}
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if err := restored.Step(); err != nil {
		t.Fatal(err)
	}
	if c.PC != restored.PC || c.GPR != restored.GPR || c.COP0 != restored.COP0 || restored.HasPendingBranch() {
		t.Fatalf("pending slot restored differently: original PC=%#x restored PC=%#x", c.PC, restored.PC)
	}
}

func TestCPUSnapshotRejectsImpossibleDelayState(t *testing.T) {
	c, _ := machine(t, 0)
	c.GPR[3] = 123
	w := snapcodec.NewWriter("cpu", 1)
	w.Words(make([]uint32, 32))
	w.Uint32(codeBase)
	w.Uint32(0)
	w.Uint32(0)
	w.Bool(false)
	w.Words(make([]uint32, 32))
	w.Uint32(codeBase + 2)
	w.Uint32(codeBase)
	w.Bool(false)
	w.Bool(true)
	w.Bool(true)
	w.Uint32(0)
	w.Bool(false)
	w.Bool(false)
	w.Uint8(0)
	bad, err := w.Blob()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Restore(bad); err == nil {
		t.Fatal("impossible pending branch accepted")
	}
	if c.GPR[3] != 123 || c.PC != codeBase {
		t.Fatal("invalid restore mutated CPU")
	}
}

func TestCPUSnapshotRejectsTruncationWithoutMutation(t *testing.T) {
	c, _ := machine(t, 0)
	c.GPR[3] = 123
	state, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Restore(state[:len(state)-1]); err == nil {
		t.Fatal("truncated CPU snapshot accepted")
	}
	if c.GPR[3] != 123 {
		t.Fatal("failed restore mutated CPU")
	}
}
