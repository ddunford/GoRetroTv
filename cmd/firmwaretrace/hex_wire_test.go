package main

import "testing"

func TestWireHexKeepsComparatorAddressFormat(t *testing.T) {
	if got := wireWord(0x800a92d8); got != "800A92D8" {
		t.Fatalf("PC with hex letters changed comparator key: %q", got)
	}
	if got := wireHalf(0x14); got != "0014" {
		t.Fatalf("PID wire width changed: %q", got)
	}
	if got := wireByte(0x4a); got != "4A" {
		t.Fatalf("match-byte wire width changed: %q", got)
	}
}
