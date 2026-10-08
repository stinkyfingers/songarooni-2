// Package click implements the metronome. Tempo is fixed for the lifetime
// of a click (no live tempo changes): one beat-period PCM buffer is
// rendered once and looped into the platform's audio player over a held-
// open stdin pipe, so playback is paced by that player's own buffering
// rather than a Go timer.
//
// New is implemented per-OS (engine_linux.go for aplay/ALSA, engine_darwin.go
// for sox/CoreAudio, engine_other.go as a safety-net no-op elsewhere) but all
// of them share the Clicker interface, NoopClicker, and renderBeat below.
package click

import (
	"fmt"
	"log"
	"math"
)

const sampleRate = 44100

// Clicker starts and stops a metronome at a fixed tempo.
type Clicker interface {
	// Start begins clicking at tempo beats per minute. Calling Start while
	// already running is an error — callers must Stop first.
	Start(tempo int) error
	// Stop ends playback immediately. Safe to call when not running.
	Stop() error
}

// NoopClicker logs clicks instead of playing them. Used when no supported
// audio player is available on PATH.
type NoopClicker struct {
	running bool
}

func (n *NoopClicker) Start(tempo int) error {
	if n.running {
		return fmt.Errorf("click: already running")
	}
	n.running = true
	log.Printf("click: (silent) started at %d BPM", tempo)
	return nil
}

func (n *NoopClicker) Stop() error {
	n.running = false
	return nil
}

// renderBeat renders one beat period (60/tempo seconds) of 16-bit mono PCM
// at sampleRate: a short sine-wave burst with a fast linear fade-out (so it
// doesn't pop), followed by silence padding the buffer out to the full beat
// period.
func renderBeat(tempo int) []byte {
	const (
		toneHz       = 1800.0
		burstSeconds = 0.015
		amplitude    = 0.6 // fraction of full scale, to leave headroom
	)

	sr := float64(sampleRate)
	beatSeconds := 60.0 / float64(tempo)
	totalSamples := int(beatSeconds * sr)
	burstSamples := int(burstSeconds * sr)
	if burstSamples > totalSamples {
		burstSamples = totalSamples
	}

	buf := make([]byte, totalSamples*2) // 2 bytes per sample (S16_LE)
	for i := 0; i < burstSamples; i++ {
		t := float64(i) / sampleRate
		fade := 1.0 - float64(i)/float64(burstSamples) // linear fade-out
		sample := amplitude * fade * math.Sin(2*math.Pi*toneHz*t)
		v := int16(sample * math.MaxInt16)
		buf[2*i] = byte(v)
		buf[2*i+1] = byte(v >> 8)
	}
	// Remaining bytes are already zero (silence).
	return buf
}
