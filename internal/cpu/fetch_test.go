package cpu_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
)

func TestInstructionFetchIsDistinctFromDataReadAtSameAddress(t *testing.T) {
	t.Parallel()
	instruction := ri(35, 1, 2, 0) // lw $v0,0($at)
	core, board := machine(t, instruction, 0)
	core.GPR[1] = codeBase
	var seen []bus.ObservedAccess
	board.SetObserver(func(access bus.ObservedAccess) { seen = append(seen, access) })
	if err := core.Step(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || !seen[0].Fetch || seen[1].Fetch ||
		seen[0].Virtual != codeBase || seen[1].Virtual != codeBase || core.GPR[2] != instruction {
		t.Fatalf("fetch and data load were conflated: accesses=%+v value=%#x", seen, core.GPR[2])
	}
}
