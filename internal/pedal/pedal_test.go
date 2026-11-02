package pedal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

type fakeController struct {
	calls []string
	err   error // returned from every call, to test error logging doesn't stop the loop
}

func (f *fakeController) SelectNext() error {
	f.calls = append(f.calls, "SelectNext")
	return f.err
}
func (f *fakeController) SelectPrevious() error {
	f.calls = append(f.calls, "SelectPrevious")
	return f.err
}
func (f *fakeController) Start() error { f.calls = append(f.calls, "Start"); return f.err }
func (f *fakeController) StartDefault() error {
	f.calls = append(f.calls, "StartDefault")
	return f.err
}

// rawEvent builds one raw 24-byte input_event record: a 16-byte timeval
// (ignored by readEvents, so left zeroed), type, code, value.
func rawEvent(typ, code uint16, value int32) []byte {
	buf := make([]byte, eventSize)
	binary.LittleEndian.PutUint16(buf[16:18], typ)
	binary.LittleEndian.PutUint16(buf[18:20], code)
	binary.LittleEndian.PutUint32(buf[20:24], uint32(value))
	return buf
}

func keyPress(code uint16) []byte   { return rawEvent(evKey, code, 1) }
func keyRelease(code uint16) []byte { return rawEvent(evKey, code, 0) }
func keyRepeat(code uint16) []byte  { return rawEvent(evKey, code, 2) }

func TestReadEventsDispatchesEachKeyOnPressOnly(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(keyPress(keySpace))
	buf.Write(keyRelease(keySpace)) // must be ignored
	buf.Write(keyPress(keyEnter))
	buf.Write(keyRepeat(keyEnter)) // must be ignored
	buf.Write(keyPress(keyPageUp))
	buf.Write(keyPress(keyPageDown))

	c := &fakeController{}
	if err := readEvents(&buf, c); err != io.ErrUnexpectedEOF && err != io.EOF {
		t.Fatalf("readEvents: %v", err)
	}

	want := []string{"StartDefault", "Start", "SelectPrevious", "SelectNext"}
	if len(c.calls) != len(want) {
		t.Fatalf("expected calls %v, got %v", want, c.calls)
	}
	for i, call := range want {
		if c.calls[i] != call {
			t.Fatalf("expected calls %v, got %v", want, c.calls)
		}
	}
}

func TestReadEventsIgnoresNonKeyAndUnrelatedCodes(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(rawEvent(0x02 /* EV_REL, mouse movement */, 0, 5))
	buf.Write(keyPress(30)) // KEY_A — not a pedal key
	buf.Write(keyPress(keySpace))

	c := &fakeController{}
	_ = readEvents(&buf, c)

	if len(c.calls) != 1 || c.calls[0] != "StartDefault" {
		t.Fatalf("expected only StartDefault, got %v", c.calls)
	}
}

func TestReadEventsDoesNotStopOnControllerError(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(keyPress(keyEnter))
	buf.Write(keyPress(keySpace))
	buf.Write(keyPress(keyEnter))

	c := &fakeController{err: errors.New("no song selected")}
	_ = readEvents(&buf, c)

	if len(c.calls) != 3 {
		t.Fatalf("expected all 3 keys to still be dispatched despite errors, got %v", c.calls)
	}
}

func TestReadEventsStopsAtEOF(t *testing.T) {
	err := readEvents(bytes.NewReader(nil), &fakeController{})
	if err == nil {
		t.Fatal("expected an error (EOF) from an empty reader")
	}
}
