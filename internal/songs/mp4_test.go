package songs

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// box builds one ISO base media box: a 4-byte size, a 4-byte type, then
// payload — the exact format MP4Duration parses.
func box(name string, payload []byte) []byte {
	buf := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(buf[0:4], uint32(8+len(payload)))
	copy(buf[4:8], name)
	copy(buf[8:], payload)
	return buf
}

// mvhdV0 builds a version-0 mvhd payload with just the fields
// MP4Duration reads (creation/modification time are left zeroed; nothing
// after duration matters for parsing).
func mvhdV0(timescale, duration uint32) []byte {
	payload := make([]byte, 20)
	binary.BigEndian.PutUint32(payload[12:16], timescale)
	binary.BigEndian.PutUint32(payload[16:20], duration)
	return payload
}

// mvhdV1 is mvhdV0's 64-bit-duration counterpart.
func mvhdV1(timescale uint32, duration uint64) []byte {
	payload := make([]byte, 32)
	payload[0] = 1 // version
	binary.BigEndian.PutUint32(payload[20:24], timescale)
	binary.BigEndian.PutUint64(payload[24:32], duration)
	return payload
}

func writeMP4(t *testing.T, boxes ...[]byte) string {
	t.Helper()
	var data []byte
	for _, b := range boxes {
		data = append(data, b...)
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMP4DurationVersion0(t *testing.T) {
	ftyp := box("ftyp", []byte("isom"))
	moov := box("moov", box("mvhd", mvhdV0(1000, 12345)))
	path := writeMP4(t, ftyp, moov)

	got, err := MP4Duration(path)
	if err != nil {
		t.Fatalf("MP4Duration: %v", err)
	}
	want := 12345 * time.Second / 1000
	if got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestMP4DurationVersion1(t *testing.T) {
	moov := box("moov", box("mvhd", mvhdV1(48000, 480000)))
	path := writeMP4(t, box("ftyp", []byte("isom")), moov)

	got, err := MP4Duration(path)
	if err != nil {
		t.Fatalf("MP4Duration: %v", err)
	}
	if want := 10 * time.Second; got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

// A real moov box has other children (trak, udta, ...) before or after
// mvhd — this confirms findChildBox actually scans rather than assuming
// mvhd is the only or first child.
func TestMP4DurationSkipsOtherMoovChildren(t *testing.T) {
	moov := box("moov", append(box("udta", []byte("ignored")), box("mvhd", mvhdV0(1000, 2000))...))
	path := writeMP4(t, box("ftyp", []byte("isom")), moov)

	got, err := MP4Duration(path)
	if err != nil {
		t.Fatalf("MP4Duration: %v", err)
	}
	if want := 2 * time.Second; got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

// A large box after moov (standing in for mdat, which can be gigabytes)
// must never be read into memory — only skipped via Seek.
func TestMP4DurationSkipsTrailingBoxWithoutReadingIt(t *testing.T) {
	moov := box("moov", box("mvhd", mvhdV0(1000, 1000)))
	hugePayload := make([]byte, 1<<20) // stands in for a large mdat; never actually read
	path := writeMP4(t, box("ftyp", []byte("isom")), moov, box("mdat", hugePayload))

	got, err := MP4Duration(path)
	if err != nil {
		t.Fatalf("MP4Duration: %v", err)
	}
	if want := time.Second; got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestMP4DurationErrorsWhenMoovMissing(t *testing.T) {
	path := writeMP4(t, box("ftyp", []byte("isom")))
	if _, err := MP4Duration(path); err == nil {
		t.Fatal("expected an error when the file has no moov box")
	}
}

func TestMP4DurationErrorsWhenMvhdMissing(t *testing.T) {
	path := writeMP4(t, box("moov", box("udta", []byte("nope"))))
	if _, err := MP4Duration(path); err == nil {
		t.Fatal("expected an error when moov has no mvhd box")
	}
}

func TestMP4DurationErrorsOnZeroTimescale(t *testing.T) {
	moov := box("moov", box("mvhd", mvhdV0(0, 1000)))
	path := writeMP4(t, moov)
	if _, err := MP4Duration(path); err == nil {
		t.Fatal("expected an error for a zero timescale")
	}
}

func TestMP4DurationErrorsOnUnsupportedVersion(t *testing.T) {
	payload := mvhdV0(1000, 1000)
	payload[0] = 2 // no such mvhd version
	moov := box("moov", box("mvhd", payload))
	path := writeMP4(t, moov)
	if _, err := MP4Duration(path); err == nil {
		t.Fatal("expected an error for an unsupported mvhd version")
	}
}

func TestMP4DurationErrorsOnMissingFile(t *testing.T) {
	if _, err := MP4Duration(filepath.Join(t.TempDir(), "nope.mp4")); err == nil {
		t.Fatal("expected an error for a nonexistent file")
	}
}
