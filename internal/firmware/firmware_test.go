package firmware_test

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/firmware"
)

// repoManifest is the manifest this repository actually ships. It is committed, unlike the images,
// so every test run checks it - which is the point: the manifest carried truncated MD5s for months
// and looked like a record the whole time.
const repoManifest = "../../firmware/MANIFEST.md"

// digests computes what a fixture's manifest should say about an image.
//
// These are the standard library's own hashes, which is safe here because they are not what the
// committed manifest is trusted for: its digests were measured independently with sha256sum and
// md5sum, and TestTheRealFirmwareMatchesTheCommittedManifest is what checks the loader against
// them. This helper only keeps the synthetic fixtures self-consistent.
func digests(data []byte) (sha, sum string) {
	s := sha256.Sum256(data)
	m := md5.Sum(data)
	return hex.EncodeToString(s[:]), hex.EncodeToString(m[:])
}

// fixture builds a firmware directory with a manifest and images that agree with it.
type fixture struct {
	dir    string
	images map[string][]byte
	rows   []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{dir: t.TempDir(), images: map[string][]byte{}}
	for i, name := range firmware.Required {
		data := make([]byte, 512+i*16)
		for j := range data {
			data[j] = byte(i*7 + j)
		}
		f.images[name] = data
	}
	return f
}

// write lays the fixture down, computing each image's real digests so the manifest agrees with the
// bytes unless a test deliberately makes it disagree.
func (f *fixture) write(t *testing.T) string {
	t.Helper()
	f.rows = nil
	for _, name := range firmware.Required {
		data := f.images[name]
		sha, sum := digests(data)
		f.rows = append(f.rows, fmt.Sprintf("| `%s` | %d | `%s` | `%s` | a test image |",
			name, len(data), sha, sum))
		if err := os.WriteFile(filepath.Join(f.dir, name), data, 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	f.writeManifest(t, f.rows)
	return f.dir
}

func (f *fixture) writeManifest(t *testing.T, rows []string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("# Test manifest\n\nSome prose.\n\n")
	b.WriteString("| File | Bytes | SHA-256 | MD5 | What it is |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, r := range rows {
		b.WriteString(r + "\n")
	}
	b.WriteString("\nMore prose.\n")
	path := filepath.Join(f.dir, firmware.ManifestFile)
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("writing the manifest: %v", err)
	}
}

// The committed manifest must be complete. It is the whole of TASK-1.7's guarantee: a truncated
// digest verifies nothing while reading exactly like a record that does.
func TestTheRepositoryManifestIsCompleteAndParses(t *testing.T) {
	t.Parallel()
	m, err := firmware.ReadManifest(repoManifest)
	if err != nil {
		t.Fatalf("the committed manifest must parse: %v", err)
	}
	for _, want := range firmware.Required {
		img, ok := m.Image(want)
		if !ok {
			t.Fatalf("the committed manifest says nothing about %s; it describes %v",
				want, m.Files())
		}
		if len(img.SHA256) != 64 || len(img.MD5) != 32 {
			t.Fatalf("%s: SHA-256 %q and MD5 %q - both must be recorded in full",
				want, img.SHA256, img.MD5)
		}
		if img.Bytes <= 0 {
			t.Fatalf("%s: byte count is %d", want, img.Bytes)
		}
	}
	if len(m.Images) != len(firmware.Required) {
		t.Fatalf("the committed manifest describes %v; the machine needs %v",
			m.Files(), firmware.Required)
	}
}

func TestGoodFirmwareLoads(t *testing.T) {
	t.Parallel()
	dir := newFixture(t).write(t)

	set, err := firmware.Load(context.Background(), dir)
	if err != nil {
		t.Fatalf("firmware that matches its manifest must load: %v", err)
	}
	if len(set.U202) == 0 || len(set.U203) == 0 || len(set.ApplicationRAM) == 0 {
		t.Fatalf("Load returned empty images: %d, %d, %d bytes",
			len(set.U202), len(set.U203), len(set.ApplicationRAM))
	}
	if len(set.U202) == len(set.U203) {
		t.Fatal("the fixture's two flash images are the same size, so this cannot tell them apart")
	}
}

// TC-1.4: an image whose checksum does not match the manifest is refused, naming the mismatch.
func TestAnImageWithTheWrongContentsIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	dir := f.write(t)

	// One byte, in the middle, leaving the size correct - the change a corrupted or substituted
	// ROM actually makes, and the one a size check alone would miss.
	path := filepath.Join(dir, firmware.FileU202)
	data, err := os.ReadFile(path) //#nosec G304
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	data[len(data)/2] ^= 0xFF
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("corrupting the fixture: %v", err)
	}

	_, err = firmware.Load(context.Background(), dir)
	if err == nil {
		t.Fatal("an image whose contents do not match the manifest must be refused; running a " +
			"different ROM silently is the failure this prevents")
	}
	msg := err.Error()
	for _, want := range []string{firmware.FileU202, "SHA-256"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("the refusal must name %q, got: %v", want, err)
		}
	}
	if strings.Contains(msg, firmware.FileU203) {
		t.Fatalf("the refusal blames an image that is fine: %v", err)
	}
	t.Logf("caught: %v", err)
}

func TestAnImageOfTheWrongSizeIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	dir := f.write(t)

	path := filepath.Join(dir, firmware.FileU203)
	data, err := os.ReadFile(path) //#nosec G304
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	if err := os.WriteFile(path, append(data, 0), 0o600); err != nil {
		t.Fatalf("growing the fixture: %v", err)
	}

	_, err = firmware.Load(context.Background(), dir)
	if err == nil {
		t.Fatal("an image of the wrong size must be refused")
	}
	// Pinned to the check made BEFORE the file is read. A wrong size is also a wrong digest, so
	// a weaker assertion here passes on the digest mismatch alone and the size check could be
	// deleted without any test noticing - which is exactly what happened when this was written.
	want := fmt.Sprintf("%s: is %d bytes and the manifest records %d",
		firmware.FileU203, len(data)+1, len(data))
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("the refusal must say %q, so it names the size rather than only the digest; "+
			"got: %v", want, err)
	}
	if strings.Contains(err.Error(), "changed size while being read") {
		t.Fatalf("the file did not change while being read, it was the wrong size to begin "+
			"with, and saying otherwise sends the reader looking for a race: %v", err)
	}
	t.Logf("caught: %v", err)
}

func TestEveryBadImageIsReportedTogether(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	dir := f.write(t)
	for _, name := range []string{firmware.FileU202, firmware.FileU203} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path) //#nosec G304
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		data[0] ^= 0xFF
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("corrupting %s: %v", name, err)
		}
	}

	_, err := firmware.Load(context.Background(), dir)
	if err == nil {
		t.Fatal("two wrong images must be refused")
	}
	for _, want := range []string{firmware.FileU202, firmware.FileU203} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("an operator with two wrong images should learn that once: %v", err)
		}
	}
	t.Logf("caught: %v", err)
}

func TestAMissingImageIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	dir := f.write(t)
	if err := os.Remove(filepath.Join(dir, firmware.FileApplicationRAM)); err != nil {
		t.Fatalf("removing the fixture: %v", err)
	}

	_, err := firmware.Load(context.Background(), dir)
	if err == nil {
		t.Fatal("a missing image must be refused")
	}
	if !strings.Contains(err.Error(), firmware.FileApplicationRAM) {
		t.Fatalf("the refusal must name the missing image, got: %v", err)
	}
	t.Logf("caught: %v", err)
}

// A manifest that describes only some of the machine would let it start half-verified, which is
// indistinguishable from a machine that was verified.
func TestAManifestMissingAnImageIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.write(t)
	f.writeManifest(t, f.rows[:1])

	_, err := firmware.Load(context.Background(), f.dir)
	if err == nil {
		t.Fatal("a manifest that does not describe every required image must be refused")
	}
	if !strings.Contains(err.Error(), firmware.FileU203) {
		t.Fatalf("the refusal must name what the manifest is silent about, got: %v", err)
	}
	t.Logf("caught: %v", err)
}

// The regression this whole task exists for. A manifest that records "7541fb4884d7…" reads like a
// record and verifies nothing, so the parser refuses it by name rather than skipping the row.
func TestATruncatedDigestIsRefusedByName(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"truncated MD5": fmt.Sprintf("| `%s` | 2097152 | `%s` | `7541fb4884d7…` | truncated |",
			firmware.FileU202, strings.Repeat("a", 64)),
		"truncated SHA-256": fmt.Sprintf("| `%s` | 2097152 | `32f6b2e84d0c…` | `%s` | truncated |",
			firmware.FileU202, strings.Repeat("0", 32)),
		"digest that is not hex": fmt.Sprintf("| `%s` | 2097152 | `%s` | `%s` | not hex |",
			firmware.FileU202, strings.Repeat("z", 64), strings.Repeat("0", 32)),
		"byte count that is not a number": fmt.Sprintf("| `%s` | two million | `%s` | `%s` | words |",
			firmware.FileU202, strings.Repeat("a", 64), strings.Repeat("0", 32)),
	}
	for name, row := range cases {
		name, row := name, row
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := &fixture{dir: t.TempDir()}
			f.writeManifest(t, []string{row})

			_, err := firmware.ReadManifest(filepath.Join(f.dir, firmware.ManifestFile))
			if err == nil {
				t.Fatalf("a manifest with a %s must be refused", name)
			}
			if !strings.Contains(err.Error(), firmware.FileU202) {
				t.Fatalf("the refusal must name the row it is about, got: %v", err)
			}
			t.Logf("caught: %v", err)
		})
	}
}

// A parser that quietly finds nothing is the harness failure this project has shipped before: it
// verifies nothing and reports success.
func TestAManifestWithNoTableIsAHarnessFailure(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"no table at all":   "# Firmware\n\nJust prose, no table.\n",
		"a different table": "# Firmware\n\n| Thing | Other |\n|---|---|\n| a | b |\n",
		"a header and no rows": "# Firmware\n\n| File | Bytes | SHA-256 | MD5 | What it is |\n" +
			"|---|---|---|---|---|\n\nNothing follows.\n",
		"columns in the wrong order": "# Firmware\n\n| File | SHA-256 | Bytes | MD5 | What it is |\n" +
			"|---|---|---|---|---|\n| `a.bin` | `x` | 1 | `y` | z |\n",
	}
	for name, body := range cases {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := firmware.ParseManifest("test.md", strings.NewReader(body)); err == nil {
				t.Fatalf("a manifest with %s must be an error, not an empty image list that "+
					"verifies nothing", name)
			} else {
				t.Logf("caught: %v", err)
			}
		})
	}
}

func TestADuplicateRowIsRefused(t *testing.T) {
	t.Parallel()
	row := fmt.Sprintf("| `%s` | 2097152 | `%s` | `%s` | one |",
		firmware.FileU202, strings.Repeat("a", 64), strings.Repeat("0", 32))
	body := "| File | Bytes | SHA-256 | MD5 | What it is |\n|---|---|---|---|---|\n" + row + "\n" + row + "\n"

	if _, err := firmware.ParseManifest("test.md", strings.NewReader(body)); err == nil {
		t.Fatal("two rows for one file must be refused: they could disagree")
	} else {
		t.Logf("caught: %v", err)
	}
}

func TestLoadHonoursACancelledContext(t *testing.T) {
	t.Parallel()
	dir := newFixture(t).write(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := firmware.Load(ctx, dir); err == nil {
		t.Fatal("Load must honour a cancelled context")
	}
}

// The real images, when this machine has them. It skips loudly rather than quietly: a test that
// says nothing when it is skipped is one nobody notices has stopped running.
func TestTheRealFirmwareMatchesTheCommittedManifest(t *testing.T) {
	t.Parallel()
	dir := filepath.Dir(repoManifest)
	for _, name := range firmware.Required {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Skipf("NOT RUN: %s is not on this machine (%v). The firmware is Pace's and is "+
				"gitignored, so this check only runs where the images have been supplied.", name, err)
		}
	}

	set, err := firmware.Load(context.Background(), dir)
	if err != nil {
		t.Fatalf("the firmware on this machine does not match the committed manifest: %v", err)
	}
	if len(set.U202) != 2097152 || len(set.U203) != 2097152 {
		t.Fatalf("flash parts are %d and %d bytes, want 2097152 each",
			len(set.U202), len(set.U203))
	}
	// A file of the right size and digest is still the wrong file if it is not this ROM, so
	// check that it contains the machine. The reset path is a fixed point taken from the record
	// rather than a value invented here: execution starts at 0xBFC00008, and the three words
	// there build 0xBFC00408 in $t0 and jump to it.
	//
	// (The first two words of the image ARE zero, which is why "the reset vector is non-zero"
	// was the wrong check - it failed against correct firmware on the first run.)
	word := func(off int) uint32 {
		return uint32(set.U202[off])<<24 | uint32(set.U202[off+1])<<16 |
			uint32(set.U202[off+2])<<8 | uint32(set.U202[off+3])
	}
	for _, want := range []struct {
		off   int
		value uint32
		asm   string
	}{
		{0x08, 0x3C08BFC0, "lui $t0, 0xBFC0"},
		{0x0C, 0x25080408, "addiu $t0, $t0, 0x0408"},
		{0x10, 0x01000008, "jr $t0"},
	} {
		if got := word(want.off); got != want.value {
			t.Fatalf("U202+%#04x is %#08x, want %#08x (%s) - the image verifies against the "+
				"manifest but does not contain this machine's reset path",
				want.off, got, want.value, want.asm)
		}
	}
}
