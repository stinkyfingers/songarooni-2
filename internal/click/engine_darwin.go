package click

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
)

// New returns a SoxClicker at the given volume (0 to 1) if the sox binary is
// on PATH, or a NoopClicker (with a logged warning) otherwise. This is the
// macOS dev-convenience path only — the real deployment target is
// Linux/aplay (see engine_linux.go).
func New(device string, volume float64, frequency float64) Clicker {
	if _, err := exec.LookPath("sox"); err != nil {
		log.Printf("click: sox not found on PATH (try `brew install sox` to hear the click during development); using a silent no-op clicker (%v)", err)
		return &NoopClicker{}
	}
	return &SoxClicker{volume: volume, frequency: frequency}
}

// SoxClicker pipes a looped one-beat PCM buffer into a sox subprocess,
// playing to the system's default output. Unlike AplayClicker, it has no
// device argument: SoX on macOS plays through CoreAudio, which doesn't have
// the same device-by-name story ALSA does, and this path only exists for
// local dev convenience rather than the real venue deployment.
type SoxClicker struct {
	volume    float64
	frequency float64

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stopCh chan struct{}
	done   chan struct{} // closed once cmd.Wait() returns
}

func (s *SoxClicker) Start(tempo int) error {
	if s.cmd != nil {
		return fmt.Errorf("click: already running")
	}
	if tempo <= 0 {
		return fmt.Errorf("click: invalid tempo %d", tempo)
	}

	buf := renderBeat(tempo, s.volume, s.frequency)

	cmd := exec.Command("sox",
		"-t", "raw", "-r", fmt.Sprintf("%d", sampleRate), "-e", "signed", "-b", "16", "-c", "1", "--endian", "little",
		"-",  // read from stdin
		"-d", // play to the default output device
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("click: creating stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("click: starting sox: %w", err)
	}

	s.cmd = cmd
	s.stdin = stdin
	s.stopCh = make(chan struct{})
	s.done = make(chan struct{})

	go func() {
		defer close(s.done)
		waitErr := cmd.Wait()
		select {
		case <-s.stopCh:
			// Expected: Stop() killed it.
		default:
			log.Printf("click: sox exited unexpectedly (%v); stderr: %s", waitErr, strings.TrimSpace(stderr.String()))
		}
	}()

	go func() {
		for {
			select {
			case <-s.stopCh:
				return
			default:
				if _, err := stdin.Write(buf); err != nil {
					return // pipe closed by Stop, or sox exited
				}
			}
		}
	}()

	return nil
}

func (s *SoxClicker) Stop() error {
	if s.cmd == nil {
		return nil
	}
	close(s.stopCh)
	s.stdin.Close()
	killErr := s.cmd.Process.Kill()
	<-s.done // wait for the single cmd.Wait() call so sox is fully reaped
	s.cmd = nil
	s.stdin = nil
	if killErr != nil {
		return fmt.Errorf("click: stopping sox: %w", killErr)
	}
	return nil
}
