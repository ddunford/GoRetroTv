package snapcodec_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

// demux stands in for a real device: a few scalars and a variable-length table, which is the shape
// most of this machine's peripherals have.
//
// It carries two format versions on purpose. v1 is what shipped; v2 added the PID filter table.
// Both must restore, because a snapshot taken before v2 existed is still a snapshot.
type demux struct {
	control uint32
	armed   bool
	label   string
	filters []uint32 // added in format v2
}

const (
	demuxName    = "demux"
	demuxV1      = uint16(1)
	demuxV2      = uint16(2)
	demuxCurrent = demuxV2
)

// snapshot writes the device at the given format version. Real devices only ever write the current
// version; this one takes a parameter so the test can produce an old blob without a time machine.
func (d *demux) snapshot(version uint16) ([]byte, error) {
	w := snapcodec.NewWriter(demuxName, version)
	w.Uint32(d.control)
	w.Bool(d.armed)
	w.String(d.label)
	if version >= demuxV2 {
		w.Words(d.filters)
	}
	return w.Blob()
}

// restore reads any format version this build understands.
func (d *demux) restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return err
	}
	if err := r.Expect(demuxName, demuxV1, demuxCurrent); err != nil {
		return err
	}
	d.control = r.Uint32()
	d.armed = r.Bool()
	d.label = r.String()
	if r.Version() >= demuxV2 {
		d.filters = r.Words()
	} else {
		// A v1 blob predates the filter table, so the field takes its reset value rather than
		// whatever the previous occupant of this struct left behind.
		d.filters = nil
	}
	return r.Done()
}

func fullDemux() *demux {
	return &demux{
		control: 0x80081C58,
		armed:   true,
		label:   "transport demux",
		filters: []uint32{0x0011, 0x0BBC, 0xFFFF},
	}
}

func TestRoundTripAtTheCurrentVersion(t *testing.T) {
	t.Parallel()

	want := fullDemux()
	blob, err := want.snapshot(demuxCurrent)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Restore into a device carrying different state, so a field that is never written shows up
	// as the old value rather than coincidentally matching a zero.
	got := &demux{control: 0xDEADBEEF, armed: false, label: "stale", filters: []uint32{1, 2, 3, 4}}
	if err := got.restore(blob); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("restored %+v, want %+v", got, want)
	}
}

func TestRoundTripAtTheOlderVersion(t *testing.T) {
	t.Parallel()

	old := &demux{control: 0x0000BFC0, armed: false, label: "v1 demux"}
	blob, err := old.snapshot(demuxV1)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	got := &demux{control: 0xDEADBEEF, armed: true, label: "stale", filters: []uint32{9, 9}}
	if err := got.restore(blob); err != nil {
		t.Fatalf("restore a v1 blob: %v", err)
	}
	if !reflect.DeepEqual(got, old) {
		t.Errorf("restored %+v, want %+v", got, old)
	}
	if got.filters != nil {
		t.Errorf("filters = %v, want nil: a v1 blob has no filter table and must not leave the previous value in place", got.filters)
	}
}

// TestAFutureVersionIsRefusedByName is the half of TC-1.8 that matters most. A v3 layout read with
// v2's field order does not fail; it produces a machine. The only safe answer is to refuse, saying
// which device and which version.
func TestAFutureVersionIsRefusedByName(t *testing.T) {
	t.Parallel()

	future := fullDemux()
	blob, err := future.snapshot(demuxCurrent + 1)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	got := &demux{}
	err = got.restore(blob)
	if err == nil {
		t.Fatal("a future format version restored silently; it must be refused")
	}
	if !errors.Is(err, snapcodec.ErrUnsupportedVersion) {
		t.Errorf("error is %v, want it to wrap ErrUnsupportedVersion", err)
	}
	for _, want := range []string{demuxName, "v3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q; a refusal that does not say what was refused sends the reader to the wrong file", err, want)
		}
	}
	if !reflect.DeepEqual(got, &demux{}) {
		t.Errorf("a refused restore mutated the device to %+v", got)
	}
}

func TestABlobFromAnotherDeviceIsRefusedByName(t *testing.T) {
	t.Parallel()

	w := snapcodec.NewWriter("blitter", 1)
	w.Uint32(0x1234)
	blob, err := w.Blob()
	if err != nil {
		t.Fatalf("blob: %v", err)
	}

	err = (&demux{}).restore(blob)
	if !errors.Is(err, snapcodec.ErrWrongWriter) {
		t.Fatalf("error is %v, want it to wrap ErrWrongWriter", err)
	}
	for _, want := range []string{"blitter", demuxName} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// TestASnapshotThatForgetsAFieldFails is the completeness instrument in the write direction: the
// device stores less than it restores, so the read runs off the end.
func TestASnapshotThatForgetsAFieldFails(t *testing.T) {
	t.Parallel()

	// Deliberately incomplete: the label and the filter table are missing.
	w := snapcodec.NewWriter(demuxName, demuxCurrent)
	w.Uint32(0x80081C58)
	w.Bool(true)
	blob, err := w.Blob()
	if err != nil {
		t.Fatalf("blob: %v", err)
	}

	err = (&demux{}).restore(blob)
	if err == nil {
		t.Fatal("a snapshot missing two fields restored cleanly; the completeness check is not working")
	}
	if !errors.Is(err, snapcodec.ErrShortRead) {
		t.Errorf("error is %v, want it to wrap ErrShortRead", err)
	}
}

// TestARestoreThatForgetsAFieldFails is the same instrument in the read direction: the device
// stores more than it reads back, so a field silently keeps its stale value. Done catches it.
func TestARestoreThatForgetsAFieldFails(t *testing.T) {
	t.Parallel()

	blob, err := fullDemux().snapshot(demuxCurrent)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	r, err := snapcodec.Open(blob)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := r.Expect(demuxName, demuxV1, demuxCurrent); err != nil {
		t.Fatalf("expect: %v", err)
	}
	// A Restore that was never updated when the filter table was added.
	_ = r.Uint32()
	_ = r.Bool()
	_ = r.String()

	err = r.Done()
	if err == nil {
		t.Fatal("a restore that skipped a field reported success; this is the plausible-machine failure")
	}
	if !errors.Is(err, snapcodec.ErrTrailingBytes) {
		t.Errorf("error is %v, want it to wrap ErrTrailingBytes", err)
	}
}

func TestEncodingIsDeterministic(t *testing.T) {
	t.Parallel()

	first, err := fullDemux().snapshot(demuxCurrent)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	second, err := fullDemux().snapshot(demuxCurrent)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Error("two snapshots of identical state produced different bytes; byte-identical replay depends on this")
	}
}

func TestEveryPrimitiveRoundTrips(t *testing.T) {
	t.Parallel()

	w := snapcodec.NewWriter("primitives", 1)
	w.Uint8(0xAB)
	w.Uint16(0xABCD)
	w.Uint32(0xABCDEF01)
	w.Uint64(0xABCDEF0123456789)
	w.Bool(true)
	w.Bool(false)
	w.Bytes([]byte{1, 2, 3})
	w.Bytes(nil)
	w.Words([]uint32{0x11111111, 0x22222222})
	w.String("sky digital")
	blob, err := w.Blob()
	if err != nil {
		t.Fatalf("blob: %v", err)
	}

	r, err := snapcodec.Open(blob)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := r.Expect("primitives", 1, 1); err != nil {
		t.Fatalf("expect: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Uint8", r.Uint8(), uint8(0xAB)},
		{"Uint16", r.Uint16(), uint16(0xABCD)},
		{"Uint32", r.Uint32(), uint32(0xABCDEF01)},
		{"Uint64", r.Uint64(), uint64(0xABCDEF0123456789)},
		{"Bool true", r.Bool(), true},
		{"Bool false", r.Bool(), false},
		{"Bytes", r.Bytes(), []byte{1, 2, 3}},
		{"Bytes nil", r.Bytes(), []byte{}},
		{"Words", r.Words(), []uint32{0x11111111, 0x22222222}},
		{"String", r.String(), "sky digital"},
	}
	for _, c := range checks {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if err := r.Done(); err != nil {
		t.Errorf("Done: %v", err)
	}
}

func TestOpenRefusesMalformedBlobs(t *testing.T) {
	t.Parallel()

	good, err := fullDemux().snapshot(demuxCurrent)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	tests := []struct {
		name string
		blob []byte
		want error
	}{
		{"empty", nil, snapcodec.ErrTruncated},
		{"too short for a header", good[:8], snapcodec.ErrTruncated},
		{"wrong magic", append([]byte("XXXX"), good[4:]...), snapcodec.ErrBadMagic},
		{"truncated payload", good[:len(good)-4], snapcodec.ErrTruncated},
		{"trailing rubbish", append(append([]byte{}, good...), 0, 0), snapcodec.ErrTruncated},
		{"all zeroes", make([]byte, 64), snapcodec.ErrBadMagic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, err := snapcodec.Open(tt.blob)
			if err == nil {
				t.Fatalf("Open accepted a %s blob as %s v%d", tt.name, r.Name(), r.Version())
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("error is %v, want it to wrap %v", err, tt.want)
			}
		})
	}
}

// TestReadingBeforeExpectIsRefused stops the one way a caller could still mis-read a blob: reading
// its fields without ever asserting what it is.
func TestReadingBeforeExpectIsRefused(t *testing.T) {
	t.Parallel()

	blob, err := fullDemux().snapshot(demuxCurrent)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	r, err := snapcodec.Open(blob)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if got := r.Uint32(); got != 0 {
		t.Errorf("read before Expect returned %#x, want 0", got)
	}
	if !errors.Is(r.Err(), snapcodec.ErrNotExpected) {
		t.Errorf("error is %v, want it to wrap ErrNotExpected", r.Err())
	}
}

func TestWriterRefusesAnEmptyName(t *testing.T) {
	t.Parallel()

	w := snapcodec.NewWriter("", 1)
	w.Uint32(1)
	if _, err := w.Blob(); err == nil {
		t.Error("an unnamed writer produced a blob; a blob that cannot say what wrote it cannot be refused by name either")
	}
}
