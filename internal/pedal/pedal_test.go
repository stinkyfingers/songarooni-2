package pedal

import (
	"errors"
	"strings"
	"testing"
)

type fakeController struct {
	calls []string
	err   error // returned from every call, to test error logging doesn't stop the loop
}

func (f *fakeController) SelectNext() error { f.calls = append(f.calls, "SelectNext"); return f.err }
func (f *fakeController) SelectPrevious() error {
	f.calls = append(f.calls, "SelectPrevious")
	return f.err
}
func (f *fakeController) Start() error { f.calls = append(f.calls, "Start"); return f.err }
func (f *fakeController) StartDefault() error {
	f.calls = append(f.calls, "StartDefault")
	return f.err
}

func TestRunDispatchesEachKey(t *testing.T) {
	// Space, Enter (CR), Page Up (ESC [ 5 ~), Page Down (ESC [ 6 ~), and a
	// line-feed Enter variant, all in one stream — mirrors how the pedal's
	// keys actually arrive, byte for byte.
	input := " \r\x1b[5~\x1b[6~\n"
	c := &fakeController{}

	if err := Run(strings.NewReader(input), c); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{"StartDefault", "Start", "SelectPrevious", "SelectNext", "Start"}
	if len(c.calls) != len(want) {
		t.Fatalf("expected calls %v, got %v", want, c.calls)
	}
	for i, call := range want {
		if c.calls[i] != call {
			t.Fatalf("expected calls %v, got %v", want, c.calls)
		}
	}
}

func TestRunIgnoresUnrelatedEscapeSequences(t *testing.T) {
	// Arrow keys (ESC [ A) and Home (ESC [ H) aren't pedal keys — must be
	// drained without dispatching anything and without getting the parser
	// stuck, so the next real key still works.
	input := "\x1b[A\x1b[H \n"
	c := &fakeController{}

	if err := Run(strings.NewReader(input), c); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{"StartDefault", "Start"}
	if len(c.calls) != len(want) {
		t.Fatalf("expected only %v (arrow/home ignored), got %v", want, c.calls)
	}
}

func TestRunDoesNotStopOnControllerError(t *testing.T) {
	// A rejected call (e.g. Start() with nothing selected) must not stop the
	// pedal from handling the next keypress.
	c := &fakeController{err: errors.New("no song selected")}
	input := "\r \r"

	if err := Run(strings.NewReader(input), c); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(c.calls) != 3 {
		t.Fatalf("expected all 3 keys to still be dispatched despite errors, got %v", c.calls)
	}
}

func TestRunStopsCleanlyAtEOF(t *testing.T) {
	if err := Run(strings.NewReader(""), &fakeController{}); err != nil {
		t.Fatalf("expected EOF to be treated as a clean exit, got %v", err)
	}
}
