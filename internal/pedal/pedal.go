// Package pedal reads the M-Vave Chocolate foot pedal's keystrokes directly
// from the controlling terminal, translating them into Controller calls —
// the same ones the phone's buttons make. This bypasses triggerhappy
// entirely: the kernel's TTY layer delivers keystrokes to whichever process
// has the terminal in the foreground regardless of which specific
// /dev/input/eventN produced them, so unlike triggerhappy's raw evdev reads,
// it's immune to the device renumbering a Bluetooth reconnect causes
// mid-show (see plans/pedal-connection-plan.md for the full history).
package pedal

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"os"

	"golang.org/x/term"
)

// Controller is the subset of state.Controller's methods the pedal drives.
// A narrow interface rather than *state.Controller directly, so the parsing
// logic below can be tested without a real Controller (clicker, display,
// catalog and all).
type Controller interface {
	SelectNext() error
	SelectPrevious() error
	Start() error
	StartDefault() error
}

// Keys the pedal is wired to send (see plans/pedal-connection-plan.md Part
// 3): Page Up/Down move the selection, Enter starts whatever's selected,
// Space selects-and-starts Default. Stop is deliberately not bound here —
// it's UI-only, from the phone.
const (
	keySpace = ' '
	keyCR    = '\r'
	keyLF    = '\n'
	keyEsc   = 0x1b
)

// ListenStdin puts stdin into raw mode and runs the pedal listener against
// it until ctx is done or stdin closes. If stdin isn't a real terminal (e.g.
// a future headless setup with stdin redirected), it logs that and returns
// immediately — the phone's HTTP control keeps working either way.
func ListenStdin(ctx context.Context, c Controller) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		log.Println("pedal: stdin is not a terminal; foot pedal input disabled (the phone still works)")
		return
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		log.Printf("pedal: failed to set stdin to raw mode; foot pedal input disabled: %v", err)
		return
	}
	defer func() {
		if err := term.Restore(fd, oldState); err != nil {
			log.Printf("pedal: failed to restore terminal state: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		os.Stdin.Close() // unblocks the Read loop below on shutdown
	}()

	if err := Run(os.Stdin, c); err != nil {
		log.Printf("pedal: stdin listener exited: %v", err)
	}
}

// Run reads r until it returns an error (including io.EOF, which Run treats
// as a clean exit, not a failure) and calls the matching Controller method
// for each recognized key. Errors from the Controller itself are logged,
// not returned — a rejected call (e.g. Start() with nothing selected)
// shouldn't stop the pedal from working on the next keypress.
func Run(r io.Reader, c Controller) error {
	br := bufio.NewReader(r)
	for {
		b, err := br.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		switch b {
		case keySpace:
			logErr("StartDefault", c.StartDefault())
		case keyCR, keyLF:
			logErr("Start", c.Start())
		case keyEsc:
			handleEscape(br, c)
		}
	}
}

// handleEscape reads the rest of a CSI escape sequence (ESC [ params
// final-byte) and dispatches Page Up (ESC [ 5 ~) / Page Down (ESC [ 6 ~);
// anything else (arrow keys, Home/End, etc.) is drained and ignored.
// Malformed or unexpected bytes just stop processing this sequence rather
// than blocking — a mis-synced read here must never hang the pedal listener
// waiting for bytes that aren't coming.
func handleEscape(br *bufio.Reader, c Controller) {
	b, err := br.ReadByte()
	if err != nil || b != '[' {
		return
	}

	var params []byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			return
		}
		switch {
		case b >= 0x30 && b <= 0x3f: // parameter byte, e.g. the "5" in ESC [ 5 ~
			params = append(params, b)
		case b >= 0x40 && b <= 0x7e: // final byte — sequence complete
			if b == '~' && len(params) == 1 {
				switch params[0] {
				case '5':
					logErr("SelectPrevious", c.SelectPrevious())
				case '6':
					logErr("SelectNext", c.SelectNext())
				}
			}
			return
		default: // intermediate byte or garbage — not a sequence we care about
			return
		}
	}
}

func logErr(action string, err error) {
	if err != nil {
		log.Printf("pedal: %s: %v", action, err)
	}
}
