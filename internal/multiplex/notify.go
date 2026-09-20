package multiplex

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
	"github.com/ddunford/goretrotv/internal/platform/instrument"
)

// The guide is a SUBSCRIBER, not a reader, and this is the other half of the
// question this package owns.
//
// Everything in request.go reads what the box's HARDWARE will admit. That is
// only the first gate. A section that passes a match unit is parsed and its
// programmes are stored -- and the guide still shows nothing, because the guide
// never polls that store. It registers a notification slot and waits, and
// 0x800C579C fires a slot only when the service, the day key and the table id's
// low two bits all match the section that arrived. Send a pair the guide did
// not subscribe for and the records land, the parser is happy, no error is
// raised anywhere, and the screen is empty.
//
// So the slot is read off the box for the same reason every other id here is:
// it is the box's choice, it moves, and guessing it is right by luck. Two boxes
// measured minutes apart wanted table-id low bits 3 and then 2.
const (
	// guideSlotTables is the base of the guide's two notification tables. Each
	// header is 0x10 bytes -- a u16 count at +0 and a slot pointer at +8 -- and
	// the first header is one stride IN from the base, which is the oracle's
	// measured shape rather than a derived one.
	guideSlotTables uint32 = 0x80165048
	guideSlotHeader uint32 = 0x10
	guideSlotStride uint32 = 0x2c

	// The bounds a header must satisfy to be a header at all. They are what
	// separates "the guide has not subscribed yet", which is an answer, from
	// "this is not the slot table", which is a broken instrument.
	guideSlotMaxCount = 4096
	guideSlotRAMLimit = 0x82000000
)

// GuideSlot is one notification the guide has subscribed for.
type GuideSlot struct {
	// At is the slot's guest address, and Index is its position in the table
	// it came from. Index 0 is reported but never chosen -- see GuideSlots.
	At    uint32
	Table int
	Index int
	// ID is the notification id the slot is waiting for, at +0x04. The
	// carousel's parser sends 0x3EC and 0x3EA, and the collector writes 999
	// back into this field once it has taken the notification -- so an idle
	// slot in a live table reads 0x3E7, not zero.
	ID uint32
	// Ref is +0x08, and WHAT IT MEANS IS NOT ESTABLISHED. It is not one of the
	// fields 0x800C579C compares, and it is not any id the broadcast carries:
	// it counts up across registrations -- 0x0BBA then 0x0BBB on one box,
	// 0x0BBE then 0x0BBF on the next run of the same test -- while this
	// line-up's listings ids are 101 to 501. It reads like a handle the guide
	// allocated for the registration. It is dumped because it is what tells
	// one slot from the next in a log, not because it is understood.
	Ref uint32
	// Service is the service id the slot wants, taken from the field Kind
	// selects: +0x10 normally, +0x12 when Kind is 2.
	Service uint16
	// Kind is +0x1D, which chooses between the two service fields.
	Kind byte
	// DayKey is +0x18: the day the guide is showing, which is its own clock's
	// day and not the day the acquisition subsystem asked for.
	DayKey uint16
	// TableIDLow is +0x1A, compared against the arriving section's tableID & 3.
	TableIDLow byte
	// Wildcard is +0x28 == 1, which the firmware treats as matching whatever
	// arrives regardless of the three fields above.
	Wildcard bool
}

// String renders a slot the way the record writes one.
func (s GuideSlot) String() string {
	return fmt.Sprintf("%s table %d slot %d: id %s, ref %s, service %s, day %d, tableIdLow %d, kind %d, wildcard %v",
		hexfmt.Addr(s.At), s.Table, s.Index, hexfmt.Word(s.ID), hexfmt.Word(s.Ref),
		hexfmt.Half(s.Service), s.DayKey, s.TableIDLow, s.Kind, s.Wildcard)
}

// TableID is the full table id a section must carry to reach this slot, given
// that every listings table id this firmware uses is 0xA0 | low bits.
func (s GuideSlot) TableID() byte { return 0xa0 | (s.TableIDLow & 3) }

// GuideSlots dumps every active notification slot the guide holds.
//
// IT REFUSES TO REPORT AN EMPTY TABLE AS AN EMPTY SUBSCRIPTION. A base address
// that is wrong, a box whose application has not started and a snapshot taken
// before the guide existed all read as "no slots" from a plain walk, and that
// is the shape of finding this project has been wrong about most often: a
// census that cannot find its subject reporting a plausible zero. So a header
// that is not a header at all -- no count, or a slot pointer outside DRAM -- in
// BOTH tables is a harness failure. A readable table holding no active slot is
// a measurement, and returns no slots and no error: the guide registers its
// slot when it is opened, so a box nobody has pressed tv guide on has genuinely
// subscribed to nothing.
func GuideSlots(ram *memory.RAM) ([]GuideSlot, error) {
	if ram == nil {
		return nil, fmt.Errorf("multiplex: no DRAM to read the guide's subscription from")
	}
	var slots []GuideSlot
	tables := 0
	for table := 1; table <= 2; table++ {
		header := guideSlotTables + uint32(table)*guideSlotHeader // #nosec G115 -- a loop bound of 2
		count, at, ok := guideSlotTable(ram, header)
		if !ok {
			continue
		}
		tables++
		for index := 0; index < count; index++ {
			slot, active, err := readGuideSlot(ram, at+uint32(index)*guideSlotStride) // #nosec G115 -- count is bounded above
			if err != nil {
				return nil, err
			}
			if !active {
				continue
			}
			slot.Table, slot.Index = table, index
			slots = append(slots, slot)
		}
	}
	if tables == 0 {
		return nil, &instrument.HarnessError{
			Instrument: "guide notification slots",
			Subject:    "the slot tables at " + hexfmt.Addr(guideSlotTables),
			Detail: "neither table has a readable count and slot pointer, so this is not the guide's " +
				"subscription -- the box has not started its application, or the tables have moved",
			Err: instrument.ErrNothingExamined,
		}
	}
	return slots, nil
}

// GuideSubscription is the one slot a transmitter should address, or false when
// the guide has not subscribed yet.
//
// It picks the first active slot from index 1 upwards, which is what the oracle
// picks and therefore what every measurement in the record was taken against.
// Index 0 is dumped by GuideSlots and skipped here deliberately: the reason it
// is skipped is not established, so the choice stays where it was measured
// rather than being widened on a guess.
func GuideSubscription(ram *memory.RAM) (GuideSlot, bool, error) {
	slots, err := GuideSlots(ram)
	if err != nil {
		return GuideSlot{}, false, err
	}
	for _, slot := range slots {
		if slot.Index >= 1 {
			return slot, true, nil
		}
	}
	return GuideSlot{}, false, nil
}

// guideSlotTable reads one header and says whether it is one.
func guideSlotTable(ram *memory.RAM, header uint32) (count int, at uint32, ok bool) {
	raw, readable := guideRead(ram, header, bus.Half)
	if !readable {
		return 0, 0, false
	}
	pointer, readable := guideRead(ram, header+8, bus.Word)
	if !readable {
		return 0, 0, false
	}
	n := int(raw & 0xffff)
	if n == 0 || n > guideSlotMaxCount {
		return 0, 0, false
	}
	if pointer < memory.DRAMBase || pointer >= guideSlotRAMLimit {
		return 0, 0, false
	}
	return n, pointer, true
}

// readGuideSlot reads one slot, reporting whether it is active.
//
// A slot whose table said it exists but which lies outside DRAM is a harness
// failure rather than an inactive slot: the count and the pointer came from the
// box, so a slot they describe that cannot be read means the shape being walked
// is not the shape that is there.
func readGuideSlot(ram *memory.RAM, at uint32) (GuideSlot, bool, error) {
	field := func(offset uint32, size bus.Size) (uint32, error) {
		value, ok := guideRead(ram, at+offset, size)
		if !ok {
			return 0, &instrument.HarnessError{
				Instrument: "guide notification slots",
				Subject:    "slot at " + hexfmt.Addr(at),
				Detail:     "the table declares a slot that is not in DRAM",
				Err:        instrument.ErrUnknownFinding,
			}
		}
		return value, nil
	}
	// THE ACTIVE FLAG IS A BYTE. Reading it as a word costs nothing visible:
	// a live slot's word is 0x01xxxxxx, which is not 1, so every slot in a
	// fully populated table reads as inactive and the instrument reports a
	// guide that subscribed to nothing. That is what it reported first.
	active, err := field(0x00, bus.Byte)
	if err != nil || active == 0 {
		return GuideSlot{}, false, err
	}
	slot := GuideSlot{At: at}
	for _, read := range []struct {
		offset uint32
		size   bus.Size
		into   func(uint32)
	}{
		{0x04, bus.Word, func(v uint32) { slot.ID = v }},
		{0x08, bus.Word, func(v uint32) { slot.Ref = v }},
		{0x1d, bus.Byte, func(v uint32) { slot.Kind = byte(v) }},       // #nosec G115 -- a byte read
		{0x18, bus.Half, func(v uint32) { slot.DayKey = uint16(v) }},   // #nosec G115 -- a half read
		{0x1a, bus.Byte, func(v uint32) { slot.TableIDLow = byte(v) }}, // #nosec G115 -- a byte read
		{0x28, bus.Word, func(v uint32) { slot.Wildcard = v == 1 }},
	} {
		value, err := field(read.offset, read.size)
		if err != nil {
			return GuideSlot{}, false, err
		}
		read.into(value)
	}
	serviceAt := uint32(0x10)
	if slot.Kind == 2 {
		serviceAt = 0x12
	}
	service, err := field(serviceAt, bus.Half)
	if err != nil {
		return GuideSlot{}, false, err
	}
	slot.Service = uint16(service) // #nosec G115 -- a half read
	return slot, true, nil
}

// guideRead reads a guest DRAM address, or reports that it is not one.
func guideRead(ram *memory.RAM, at uint32, size bus.Size) (uint32, bool) {
	physical, ok := bus.Physical(at)
	if !ok || physical+uint32(size) > ram.Size() { // #nosec G115 -- a bus size is 1, 2 or 4
		return 0, false
	}
	return ram.Read(physical, size), true
}
