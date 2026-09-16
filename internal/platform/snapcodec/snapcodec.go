// Package snapcodec is the versioned encoding every Device.Snapshot writes into and every
// Device.Restore reads back out of.
//
// It exists because a partial snapshot does not fail. A device that forgets to write one field
// restores into a machine that is plausible but wrong, and the fault then reads as a firmware bug
// somewhere else entirely — spike 003's finding, and the reason Snapshot is in the Device
// interface from the first line rather than retrofitted across eight device models later.
//
// Three properties do the work:
//
// Framing. A blob carries its writer's name and its writer's format version. Restore states which
// name and which versions it understands, so a blob from the wrong device or from a future format
// is refused by name rather than decoded into whatever the fields happen to line up with.
//
// Exhaustion. A Reader tracks how much it consumed. Done reports leftover bytes, so a Restore that
// forgets a field its Snapshot wrote fails loudly instead of quietly restoring a stale value; and
// reading past the end is a sticky error rather than a zero.
//
// Determinism. Fixed-width big-endian throughout, no maps, no padding. The same state encodes to
// the same bytes on every run, which is what lets ARCH-SNAP-1 and the replay gate compare blobs at
// all. Big-endian because the machine being modelled is, so a hex dump of a snapshot reads the
// same way round as a hex dump of its memory.
//
// Errors accumulate rather than being returned per field: a device with forty registers would
// otherwise be forty error checks, and the checks that get skipped are the ones nobody can see are
// missing. Write everything, then check Blob; read everything, then check Done.
package snapcodec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// magic heads every blob. It is here so that a truncated file, a blob from another format or a
// zeroed buffer is rejected at the door rather than parsed into a plausible machine.
var magic = [4]byte{'G', 'R', 'T', 'V'}

// containerVersion is the version of this framing, distinct from any device's own format version.
// It changes only if the header layout below changes.
const containerVersion uint16 = 1

// headerSize is magic + container version + name length + format version + payload length.
const headerSize = 4 + 2 + 2 + 2 + 4

// MaxNameLen bounds a writer's name so a corrupt length cannot ask for an enormous allocation.
const MaxNameLen = 0xFFFF

// Sentinel errors. Restore paths match on these with errors.Is; every one of them is wrapped with
// the detail that says which device and which version, because "unsupported version" on its own
// sends the reader to the wrong file.
var (
	// ErrBadMagic means the bytes are not a snapshot blob at all.
	ErrBadMagic = errors.New("not a snapshot blob")
	// ErrBadContainer means the framing version is not one this build writes or reads.
	ErrBadContainer = errors.New("unsupported container version")
	// ErrTruncated means the blob ended in the middle of something.
	ErrTruncated = errors.New("snapshot blob is truncated")
	// ErrWrongWriter means the blob was written by a different device than the one restoring it.
	ErrWrongWriter = errors.New("snapshot was written by a different device")
	// ErrUnsupportedVersion means the blob's format version is outside the range this decoder
	// understands. Refusing by name is the point: the alternative is decoding a layout that has
	// changed underneath us and calling the result state.
	ErrUnsupportedVersion = errors.New("unsupported snapshot format version")
	// ErrShortRead means a read ran past the end of the payload.
	ErrShortRead = errors.New("read past the end of the snapshot payload")
	// ErrTrailingBytes means the payload was not fully consumed: the writer stored a field the
	// reader never read back.
	ErrTrailingBytes = errors.New("snapshot payload was not fully consumed")
	// ErrNotExpected means Done or a read was called before Expect established what this blob is.
	ErrNotExpected = errors.New("snapshot blob was read before Expect identified it")
)

// Writer builds a snapshot blob for one device.
//
// Errors are sticky: the first failure is kept and every later call is a no-op, so a Snapshot
// implementation writes its fields in a straight line and checks once at the end.
type Writer struct {
	name    string
	version uint16
	payload []byte
	err     error
}

// NewWriter starts a blob for a device called name, in that device's own format version.
//
// The name is the device's stable identity, not its Go type name: renaming a type must not
// invalidate every snapshot on disk. The version is the device's, and it goes up whenever the
// field layout below it changes.
func NewWriter(name string, version uint16) *Writer {
	w := &Writer{name: name, version: version}
	if name == "" {
		w.err = fmt.Errorf("snapcodec: writer name is empty")
	} else if len(name) > MaxNameLen {
		w.err = fmt.Errorf("snapcodec: writer name is %d bytes, limit is %d", len(name), MaxNameLen)
	}
	return w
}

// Bool writes a single byte, 0 or 1.
func (w *Writer) Bool(v bool) {
	if v {
		w.Uint8(1)
		return
	}
	w.Uint8(0)
}

// Uint8 writes one byte.
func (w *Writer) Uint8(v uint8) {
	if w.err != nil {
		return
	}
	w.payload = append(w.payload, v)
}

// Uint16 writes two big-endian bytes.
func (w *Writer) Uint16(v uint16) {
	if w.err != nil {
		return
	}
	w.payload = binary.BigEndian.AppendUint16(w.payload, v)
}

// Uint32 writes four big-endian bytes.
func (w *Writer) Uint32(v uint32) {
	if w.err != nil {
		return
	}
	w.payload = binary.BigEndian.AppendUint32(w.payload, v)
}

// Uint64 writes eight big-endian bytes.
func (w *Writer) Uint64(v uint64) {
	if w.err != nil {
		return
	}
	w.payload = binary.BigEndian.AppendUint64(w.payload, v)
}

// Bytes writes a length-prefixed byte slice. A nil slice and an empty slice both encode as length
// zero and both read back as an empty slice.
func (w *Writer) Bytes(b []byte) {
	if w.err != nil {
		return
	}
	n, err := u32len(len(b))
	if err != nil {
		w.err = fmt.Errorf("snapcodec: %s: byte slice: %w", w.name, err)
		return
	}
	w.Uint32(n)
	w.payload = append(w.payload, b...)
}

// Words writes a length-prefixed slice of 32-bit words: a register file, a cache line array.
func (w *Writer) Words(words []uint32) {
	if w.err != nil {
		return
	}
	n, err := u32len(len(words))
	if err != nil {
		w.err = fmt.Errorf("snapcodec: %s: word slice: %w", w.name, err)
		return
	}
	w.Uint32(n)
	for _, v := range words {
		w.Uint32(v)
	}
}

// String writes a length-prefixed UTF-8 string.
func (w *Writer) String(s string) { w.Bytes([]byte(s)) }

// Err reports the first error the writer hit, if any.
func (w *Writer) Err() error { return w.err }

// Blob finalises the snapshot: header, then payload.
func (w *Writer) Blob() ([]byte, error) {
	if w.err != nil {
		return nil, w.err
	}
	payloadLen, err := u32len(len(w.payload))
	if err != nil {
		return nil, fmt.Errorf("snapcodec: %s: payload: %w", w.name, err)
	}
	nameLen, err := u16len(len(w.name))
	if err != nil {
		return nil, fmt.Errorf("snapcodec: %s: name: %w", w.name, err)
	}

	out := make([]byte, 0, headerSize+len(w.name)+len(w.payload))
	out = append(out, magic[:]...)
	out = binary.BigEndian.AppendUint16(out, containerVersion)
	out = binary.BigEndian.AppendUint16(out, nameLen)
	out = append(out, w.name...)
	out = binary.BigEndian.AppendUint16(out, w.version)
	out = binary.BigEndian.AppendUint32(out, payloadLen)
	out = append(out, w.payload...)
	return out, nil
}

// Reader reads a snapshot blob back.
//
// Like Writer, its errors are sticky and its reads return zero once one has occurred — which is
// exactly why a Restore must end in Done rather than trusting the values it got.
type Reader struct {
	name     string
	version  uint16
	payload  []byte
	pos      int
	expected bool
	err      error
}

// Open validates the framing and reports what the blob claims to be.
//
// It does not yet say whether that is what the caller wanted: Expect does that.
func Open(blob []byte) (*Reader, error) {
	if len(blob) < headerSize {
		return nil, fmt.Errorf("snapcodec: %w: %d bytes, header alone needs %d", ErrTruncated, len(blob), headerSize)
	}
	if string(blob[0:4]) != string(magic[:]) {
		return nil, fmt.Errorf("snapcodec: %w: header is %q", ErrBadMagic, blob[0:4])
	}

	pos := 4
	container := binary.BigEndian.Uint16(blob[pos:])
	pos += 2
	if container != containerVersion {
		return nil, fmt.Errorf("snapcodec: %w: blob is container v%d, this build reads v%d",
			ErrBadContainer, container, containerVersion)
	}

	nameLen := int(binary.BigEndian.Uint16(blob[pos:]))
	pos += 2
	if pos+nameLen+2+4 > len(blob) {
		return nil, fmt.Errorf("snapcodec: %w: header claims a %d-byte name, %d bytes remain",
			ErrTruncated, nameLen, len(blob)-pos)
	}
	name := string(blob[pos : pos+nameLen])
	pos += nameLen

	version := binary.BigEndian.Uint16(blob[pos:])
	pos += 2
	payloadLen := int(binary.BigEndian.Uint32(blob[pos:]))
	pos += 4
	if pos+payloadLen != len(blob) {
		return nil, fmt.Errorf("snapcodec: %w: %s v%d declares a %d-byte payload, %d bytes present",
			ErrTruncated, name, version, payloadLen, len(blob)-pos)
	}

	return &Reader{name: name, version: version, payload: blob[pos : pos+payloadLen]}, nil
}

// Name reports which device wrote the blob.
func (r *Reader) Name() string { return r.name }

// Version reports the writer's format version.
func (r *Reader) Version() uint16 { return r.version }

// Expect asserts that the blob came from name and carries a format version between min and max
// inclusive, and must be called before any field is read.
//
// This is the refusal TC-1.8 asks for. A blob from another device, or from a format version whose
// layout this build has never seen, is named in the error rather than decoded: reading a v3 layout
// with v2's field order does not fail, it produces a machine.
func (r *Reader) Expect(name string, minVersion, maxVersion uint16) error {
	if r.err != nil {
		return r.err
	}
	if r.name != name {
		r.err = fmt.Errorf("snapcodec: %w: blob is %q, restoring into %q", ErrWrongWriter, r.name, name)
		return r.err
	}
	if r.version < minVersion || r.version > maxVersion {
		r.err = fmt.Errorf("snapcodec: %w: %s blob is v%d, this build reads v%d..v%d",
			ErrUnsupportedVersion, r.name, r.version, minVersion, maxVersion)
		return r.err
	}
	r.expected = true
	return nil
}

// Bool reads one byte as a boolean. Any value other than 0 or 1 is an error, not a truthiness
// judgement: it means the read is misaligned with what was written.
func (r *Reader) Bool() bool {
	v := r.Uint8()
	switch v {
	case 0:
		return false
	case 1:
		return true
	default:
		if r.err == nil {
			r.err = fmt.Errorf("snapcodec: %s: boolean byte is %d, want 0 or 1 (the read is misaligned)", r.name, v)
		}
		return false
	}
}

// Uint8 reads one byte.
func (r *Reader) Uint8() uint8 {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

// Uint16 reads two big-endian bytes.
func (r *Reader) Uint16() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint16(b)
}

// Uint32 reads four big-endian bytes.
func (r *Reader) Uint32() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

// Uint64 reads eight big-endian bytes.
func (r *Reader) Uint64() uint64 {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}

// Bytes reads a length-prefixed byte slice, copied so the caller cannot alias the blob.
func (r *Reader) Bytes() []byte {
	n := r.Uint32()
	if r.err != nil {
		return nil
	}
	b := r.take(int(n))
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// Words reads a length-prefixed slice of 32-bit words.
func (r *Reader) Words() []uint32 {
	n := r.Uint32()
	if r.err != nil {
		return nil
	}
	// Check the bytes are there before sizing the slice, so a corrupt length cannot ask for an
	// allocation the blob could never have contained.
	if r.remaining() < int(n)*4 {
		r.fail(int(n) * 4)
		return nil
	}
	out := make([]uint32, n)
	for i := range out {
		out[i] = r.Uint32()
	}
	return out
}

// String reads a length-prefixed UTF-8 string.
func (r *Reader) String() string { return string(r.Bytes()) }

// Err reports the first error the reader hit, if any.
func (r *Reader) Err() error { return r.err }

// Done ends a Restore: it reports any read error, and any bytes the writer stored that the reader
// never read back.
//
// The trailing-bytes check is the completeness instrument. A device whose Snapshot grew a field
// its Restore did not is the failure that produces a plausible machine, and this is what turns it
// into an error at the point of restore instead of a wrong answer several million instructions
// later.
func (r *Reader) Done() error {
	if r.err != nil {
		return r.err
	}
	if !r.expected {
		return fmt.Errorf("snapcodec: %w: %s v%d", ErrNotExpected, r.name, r.version)
	}
	if left := r.remaining(); left != 0 {
		return fmt.Errorf("snapcodec: %w: %s v%d left %d of %d payload bytes unread",
			ErrTrailingBytes, r.name, r.version, left, len(r.payload))
	}
	return nil
}

func (r *Reader) remaining() int { return len(r.payload) - r.pos }

func (r *Reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if !r.expected {
		r.err = fmt.Errorf("snapcodec: %w: %s v%d", ErrNotExpected, r.name, r.version)
		return nil
	}
	if r.remaining() < n {
		r.fail(n)
		return nil
	}
	b := r.payload[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *Reader) fail(want int) {
	if r.err == nil {
		r.err = fmt.Errorf("snapcodec: %w: %s v%d wanted %d bytes at offset %d, %d remain",
			ErrShortRead, r.name, r.version, want, r.pos, r.remaining())
	}
}

// u32len and u16len narrow a Go length to the width of its wire field.
//
// They exist so the bound check and the conversion cannot drift apart: a conversion written
// without one truncates silently, which on a length prefix means a reader that walks off into the
// middle of the next field and decodes whatever it finds.
func u32len(n int) (uint32, error) {
	if n < 0 || int64(n) > math.MaxUint32 {
		return 0, fmt.Errorf("length %d does not fit a 32-bit field", n)
	}
	return uint32(n), nil //nolint:gosec // bounded on the line above
}

func u16len(n int) (uint16, error) {
	if n < 0 || int64(n) > math.MaxUint16 {
		return 0, fmt.Errorf("length %d does not fit a 16-bit field", n)
	}
	return uint16(n), nil //nolint:gosec // bounded on the line above
}
