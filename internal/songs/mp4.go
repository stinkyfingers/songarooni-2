package songs

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// MP4Duration reads path's duration straight from its MP4 container
// metadata (the moov/mvhd box) — no decoding, no external dependency like
// ffprobe, just the handful of bytes ISO/IEC 14496-12 stores it in.
func MP4Duration(path string) (time.Duration, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	moov, err := findTopLevelBox(f, "moov")
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	mvhd, err := findChildBox(moov, "mvhd")
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	return parseMvhd(mvhd)
}

// findTopLevelBox scans r's top-level ISO base media boxes (ftyp, moov,
// mdat, ...) for the first one named want, returning its payload. Every
// other box is skipped via Seek rather than read, so a multi-gigabyte mdat
// box never has to pass through memory just to get past it.
func findTopLevelBox(r io.ReadSeeker, want string) ([]byte, error) {
	for {
		header := make([]byte, 8)
		if _, err := io.ReadFull(r, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, fmt.Errorf("box %q not found", want)
			}
			return nil, err
		}
		size := int64(binary.BigEndian.Uint32(header[0:4]))
		name := string(header[4:8])
		headerLen := int64(8)

		if size == 1 { // 64-bit size follows the 8-byte header
			ext := make([]byte, 8)
			if _, err := io.ReadFull(r, ext); err != nil {
				return nil, err
			}
			size = int64(binary.BigEndian.Uint64(ext))
			headerLen = 16
		}

		if size == 0 { // box extends to end of file — only ever the last one
			if name != want {
				return nil, fmt.Errorf("box %q not found", want)
			}
			return io.ReadAll(r)
		}

		payloadLen := size - headerLen
		if payloadLen < 0 {
			return nil, fmt.Errorf("malformed box %q: size %d smaller than its own header", name, size)
		}

		if name == want {
			data := make([]byte, payloadLen)
			_, err := io.ReadFull(r, data)
			return data, err
		}

		if _, err := r.Seek(payloadLen, io.SeekCurrent); err != nil {
			return nil, err
		}
	}
}

// findChildBox is findTopLevelBox's counterpart for a box list already held
// in memory (a parent box's own payload, e.g. moov's), which is always
// small enough that slicing it directly is simpler than re-running a
// Seek-based scan over it.
func findChildBox(data []byte, want string) ([]byte, error) {
	for len(data) > 0 {
		if len(data) < 8 {
			return nil, fmt.Errorf("truncated box header looking for %q", want)
		}
		size := int64(binary.BigEndian.Uint32(data[0:4]))
		name := string(data[4:8])
		headerLen := int64(8)

		if size == 1 {
			if len(data) < 16 {
				return nil, fmt.Errorf("truncated 64-bit box header looking for %q", want)
			}
			size = int64(binary.BigEndian.Uint64(data[8:16]))
			headerLen = 16
		}
		if size < headerLen || size > int64(len(data)) {
			return nil, fmt.Errorf("malformed box %q: size %d", name, size)
		}

		if name == want {
			return data[headerLen:size], nil
		}
		data = data[size:]
	}
	return nil, fmt.Errorf("box %q not found", want)
}

// parseMvhd reads the timescale/duration pair out of an mvhd box's payload
// (ISO/IEC 14496-12 §8.2.2) and returns it as a time.Duration.
func parseMvhd(data []byte) (time.Duration, error) {
	if len(data) < 1 {
		return 0, fmt.Errorf("mvhd box is empty")
	}

	var timescale, duration uint64
	switch version := data[0]; version {
	case 0:
		if len(data) < 20 {
			return 0, fmt.Errorf("mvhd (version 0) box too short")
		}
		timescale = uint64(binary.BigEndian.Uint32(data[12:16]))
		duration = uint64(binary.BigEndian.Uint32(data[16:20]))
	case 1:
		if len(data) < 32 {
			return 0, fmt.Errorf("mvhd (version 1) box too short")
		}
		timescale = uint64(binary.BigEndian.Uint32(data[20:24]))
		duration = binary.BigEndian.Uint64(data[24:32])
	default:
		return 0, fmt.Errorf("unsupported mvhd version %d", version)
	}
	if timescale == 0 {
		return 0, fmt.Errorf("mvhd has a zero timescale")
	}

	seconds := float64(duration) / float64(timescale)
	return time.Duration(seconds * float64(time.Second)), nil
}
