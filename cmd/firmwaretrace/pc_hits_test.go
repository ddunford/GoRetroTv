package main

import "testing"

func TestPCHitsSeparateExecutionsBeforeAndAfterKey(t *testing.T) {
	t.Parallel()
	var hits pcHits
	if err := hits.Set("0x800297B0"); err != nil {
		t.Fatal(err)
	}
	if err := hits.Set("0x8006EA04"); err != nil {
		t.Fatal(err)
	}
	if err := hits.Set("0x800297B0"); err == nil {
		t.Fatal("duplicate address accepted")
	}
	if err := hits.Set("0x100000000"); err == nil {
		t.Fatal("address beyond guest width accepted")
	}
	hits.Observe(0x800297B0)
	hits.MarkKey()
	hits.Observe(0x800297B0)
	hits.Observe(0x8006EA04)
	if hits[0].before != 1 || hits[0].total-hits[0].before != 1 || hits[1].before != 0 || hits[1].total-hits[1].before != 1 {
		t.Fatalf("incorrect before/after key split: %+v", hits)
	}
}
