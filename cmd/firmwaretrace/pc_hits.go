package main

import (
	"fmt"
	"strconv"

	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
)

type pcHit struct {
	address uint32
	total   uint64
	before  uint64
}

type pcHits []pcHit

func (p *pcHits) String() string { return fmt.Sprintf("%d addresses", len(*p)) }

func (p *pcHits) Set(value string) error {
	address, err := strconv.ParseUint(value, 0, 32)
	if err != nil {
		return fmt.Errorf("guest PC %q: %w", value, err)
	}
	for _, probe := range *p {
		if probe.address == uint32(address) {
			return fmt.Errorf("guest PC %s repeated", hexfmt.Addr(uint32(address))) // #nosec G115 -- ParseUint limits address to 32 bits.
		}
	}
	*p = append(*p, pcHit{address: uint32(address)}) // #nosec G115 -- ParseUint limits this to 32 bits.
	return nil
}

func (p pcHits) Observe(pc uint32) {
	for i := range p {
		if p[i].address == pc {
			p[i].total++
		}
	}
}

func (p pcHits) MarkKey() {
	for i := range p {
		p[i].before = p[i].total
	}
}
