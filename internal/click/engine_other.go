//go:build !linux && !darwin

package click

import "log"

// New always returns a NoopClicker on platforms with no supported audio
// backend wired up (anything besides Linux/aplay or macOS/sox) — this is a
// safety net so the package still compiles everywhere, not an expected
// deployment target.
func New(device string) Clicker {
	log.Printf("click: no audio backend for this OS; using a silent no-op clicker")
	return &NoopClicker{}
}
