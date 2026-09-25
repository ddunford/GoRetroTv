package main

import "testing"

func TestDecoderTransportBatchIsPacketAligned(t *testing.T) {
	if decoderTransportBatch%188 != 0 {
		t.Fatalf("decoder transport batch = %d, want whole 188-byte packets", decoderTransportBatch)
	}
}
