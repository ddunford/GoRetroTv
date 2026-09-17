package main

import "testing"

func TestSectionInputsAcceptTimedPIDAndRejectMalformedSections(t *testing.T) {
	var inputs sectionInputs
	if err := inputs.Set("469000000:0x14:707005c67e120000"); err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 || inputs[0].at != 469000000 || inputs[0].pid != 20 || len(inputs[0].bytes) != 8 {
		t.Fatalf("wrong scheduled TDT: %+v", inputs)
	}
	for _, spec := range []string{
		"469000000:0x2000:707005c67e120000", // invalid PID
		"0:20:707005c67e120000",             // no meaningful delivery time
		"469000000:20:707006c67e120000",     // length field is not the payload
		"469000000:20:707005c67e1200zz",     // non-hex byte
	} {
		if err := inputs.Set(spec); err == nil {
			t.Errorf("accepted malformed section %q", spec)
		}
	}
	if len(inputs) != 1 {
		t.Fatalf("invalid sections changed schedule: %+v", inputs)
	}
}

func TestSectionsAtOneInstructionKeepCLIOrder(t *testing.T) {
	var inputs sectionInputs
	for _, spec := range []string{
		"477000000:20:707005c67e120004", // clock first
		"473000000:16:707005c67e120002", // earlier count sorts first
		"477000000:17:707005c67e120000", // same count stays after clock
	} {
		if err := inputs.Set(spec); err != nil {
			t.Fatal(err)
		}
	}
	inputs.sortByTime()
	for i, want := range []struct {
		at  uint64
		pid uint16
	}{{473000000, 16}, {477000000, 20}, {477000000, 17}} {
		if inputs[i].at != want.at || inputs[i].pid != want.pid {
			t.Fatalf("section %d got at=%d pid=%d, want at=%d pid=%d", i,
				inputs[i].at, inputs[i].pid, want.at, want.pid)
		}
	}
}
