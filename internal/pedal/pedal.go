// Package pedal reads the M-Vave Chocolate foot pedal's keystrokes directly
// from its Linux evdev device (/dev/input/eventN), translating them into
// Controller calls — the same ones the phone's buttons make.
//
// This reads at the kernel input-subsystem level rather than from a
// terminal or triggerhappy, deliberately: a terminal only receives
// keystrokes while it holds OS-level input focus, which feh's own fullscreen
// slideshow window takes away the moment it's running (the same reason
// unbind-feh-keys.sh is needed at all) — so a stdin-based approach, tried
// first, never actually saw pedal input once feh was up. triggerhappy reads
// at this same evdev level and doesn't have that focus problem, but it only
// ever opens the device files it saw at its own startup and never notices a
// new one appearing after a Bluetooth reconnect mid-show. Watch (see
// evdev_linux.go) fixes that by re-finding and reopening the device
// whenever reading from it fails. See plans/pedal-connection-plan.md for
// the full history.
package pedal

import (
	"encoding/binary"
	"io"
	"log"
)

// Controller is the subset of state.Controller's methods the pedal drives.
// A narrow interface rather than *state.Controller directly, so the
// parsing logic below can be tested without a real Controller (clicker,
// display, catalog and all).
type Controller interface {
	SelectNext() error
	SelectPrevious() error
	Start() error
	StartDefault() error
}

// Linux evdev's input_event layout on 64-bit kernels (the only target this
// app cross-compiles for — see Makefile's build-pi): a 16-byte timeval
// followed by type (uint16), code (uint16), and value (int32); 24 bytes
// total, fixed by the kernel ABI (see <linux/input.h>).
const eventSize = 24

const evKey = 0x01 // EV_KEY, from <linux/input-event-codes.h>

// Key codes (see <linux/input-event-codes.h>) for the four keys the pedal
// is wired to send (plans/pedal-connection-plan.md Part 3): Page Up/Down
// move the selection, Enter starts whatever's selected, Space
// selects-and-starts Default. Stop is deliberately not bound here — it's
// UI-only, from the phone.
const (
	keyEnter    = 28
	keySpace    = 57
	keyPageUp   = 104
	keyPageDown = 109
)

// readEvents reads raw input_event records from r until it returns an
// error (including io.EOF) and calls the matching Controller method for
// each EV_KEY press (value == 1; releases and autorepeat are ignored).
// Errors from the Controller itself are logged, not returned — a rejected
// call (e.g. Start() with nothing selected) shouldn't stop the pedal from
// working on the next keypress.
func readEvents(r io.Reader, c Controller) error {
	buf := make([]byte, eventSize)
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return err
		}

		typ := binary.LittleEndian.Uint16(buf[16:18])
		if typ != evKey {
			continue
		}
		value := int32(binary.LittleEndian.Uint32(buf[20:24]))
		if value != 1 {
			continue
		}
		code := binary.LittleEndian.Uint16(buf[18:20])

		switch code {
		case keySpace:
			logErr("StartDefault", c.StartDefault())
		case keyEnter:
			logErr("Start", c.Start())
		case keyPageUp:
			logErr("SelectPrevious", c.SelectPrevious())
		case keyPageDown:
			logErr("SelectNext", c.SelectNext())
		}
	}
}

func logErr(action string, err error) {
	if err != nil {
		log.Printf("pedal: %s: %v", action, err)
	}
}
