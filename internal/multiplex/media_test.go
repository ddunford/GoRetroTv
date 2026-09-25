package multiplex

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMediaTransportAdvancesPerPIDContinuityAndTimestamps(t *testing.T) {
	t.Parallel()
	transport, err := MediaTransport(0x20, 0x64, 1,
		[]EncodedPayload{{PTS90k: 0, Data: make([]byte, 200)}, {PTS90k: 90_000, Data: []byte{1}}},
		[]EncodedPayload{{PTS90k: 45_000, Data: []byte{2}}, {PTS90k: 135_000, Data: []byte{3}}})
	if err != nil {
		t.Fatal(err)
	}
	continuity := map[uint16]byte{}
	starts := map[uint16][][]byte{}
	for at := 0; at < len(transport); at += 188 {
		packet := transport[at : at+188]
		pid := uint16(packet[1]&0x1f)<<8 | uint16(packet[2])
		if pid != videoPID && pid != audioPID {
			continue
		}
		if got := packet[3] & 0x0f; got != continuity[pid] {
			t.Fatalf("PID %#x continuity = %d, want %d", pid, got, continuity[pid])
		}
		continuity[pid] = (continuity[pid] + 1) & 0x0f
		if packet[1]&0x40 == 0 {
			continue
		}
		payloadAt := 4
		if packet[3]&0x20 != 0 {
			payloadAt += 1 + int(packet[4])
		}
		starts[pid] = append(starts[pid], append([]byte(nil), packet[payloadAt:payloadAt+14]...))
	}
	if len(starts[videoPID]) != 2 || len(starts[audioPID]) != 2 {
		t.Fatalf("PES starts = %#v", starts)
	}
	for _, check := range []struct {
		pid       uint16
		index     int
		streamID  byte
		encodedPT []byte
	}{
		{videoPID, 0, 0xe0, []byte{0x21, 0, 1, 0, 1}},
		{videoPID, 1, 0xe0, []byte{0x21, 0, 5, 0xbf, 0x21}},
		{audioPID, 0, 0xc0, []byte{0x21, 0, 3, 0x5f, 0x91}},
		{audioPID, 1, 0xc0, []byte{0x21, 0, 9, 0x1e, 0xb1}},
	} {
		start := starts[check.pid][check.index]
		if start[3] != check.streamID || !bytes.Equal(start[9:14], check.encodedPT) {
			t.Errorf("PID %#x PES %d header = % X", check.pid, check.index, start)
		}
	}
}

func TestMediaTransportIsRecognisedAsMPEG2VideoAndMP2Audio(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required for the media transport acceptance test")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is required for the media transport acceptance test")
	}
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "video.m2v")
	audioPath := filepath.Join(dir, "audio.mp2")
	runFFmpeg(t, ffmpeg, "-f", "lavfi", "-i", "testsrc2=size=352x288:rate=25:duration=1",
		"-an", "-c:v", "mpeg2video", "-profile:v", "main", "-level:v", "main", "-g", "12",
		"-bf", "0", "-flags", "+bitexact", "-f", "mpeg2video", videoPath)
	runFFmpeg(t, ffmpeg, "-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=48000:duration=1",
		"-vn", "-c:a", "mp2", "-b:a", "192k", "-flags", "+bitexact", "-f", "mp2", audioPath)
	video, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	audio, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := MediaTransport(0x20, 0x64, 1,
		[]EncodedPayload{{PTS90k: 90_000, Data: video}},
		[]EncodedPayload{{PTS90k: 90_000, Data: audio}})
	if err != nil {
		t.Fatal(err)
	}
	transportPath := filepath.Join(dir, "service.ts")
	if err := os.WriteFile(transportPath, transport, 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(ffprobe, "-v", "error", "-show_entries", // #nosec G204 -- resolved local executable and test-owned path
		"stream=codec_name,codec_type,id", "-of", "json", transportPath).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe rejected media transport: %v\n%s", err, output)
	}
	var probe struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
			CodecType string `json:"codec_type"`
			ID        string `json:"id"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(output, &probe); err != nil {
		t.Fatalf("decode ffprobe output: %v\n%s", err, output)
	}
	want := map[string][2]string{
		"0x101": {"mpeg2video", "video"},
		"0x102": {"mp2", "audio"},
	}
	for _, stream := range probe.Streams {
		if expected, ok := want[stream.ID]; ok {
			if [2]string{stream.CodecName, stream.CodecType} != expected {
				t.Errorf("stream %s = %s/%s, want %s/%s", stream.ID, stream.CodecName,
					stream.CodecType, expected[0], expected[1])
			}
			delete(want, stream.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("ffprobe did not identify declared streams: missing %#v; output=%s", want, output)
	}
}

func runFFmpeg(t *testing.T, executable string, args ...string) {
	t.Helper()
	args = append([]string{"-v", "error", "-y"}, args...)
	if output, err := exec.Command(executable, args...).CombinedOutput(); err != nil { // #nosec G204 -- resolved local test dependency
		t.Fatalf("ffmpeg fixture generation failed: %v\n%s", err, output)
	}
}
