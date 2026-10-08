package click

import (
	"fmt"
	"io"
	"log"
	"os/exec"
)

// New returns a SoxClicker if the sox binary is on PATH, or a NoopClicker
// (with a logged warning) otherwise. This is the macOS dev-convenience path
// only — the real deployment target is Linux/aplay (see engine_linux.go).
func New(device string) Clicker {
	if _, err := exec.LookPath("sox"); err != nil {
		log.Printf("click: sox not found on PATH (try `brew install sox` to hear the click during development); using a silent no-op clicker (%v)", err)
		return &NoopClicker{}
	}
	return &SoxClicker{}
}

// SoxClicker pipes a looped one-beat PCM buffer into a sox subprocess,
// playing to the system's default output. Unlike AplayClicker, it has no
// device argument: SoX on macOS plays through CoreAudio, which doesn't have
// the same device-by-name story ALSA does, and this path only exists for
// local dev convenience rather than the real venue deployment.
type SoxClicker struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stopCh chan struct{}
}

func (s *SoxClicker) Start(tempo int) error {
	if s.cmd != nil {
		return fmt.Errorf("click: already running")
	}
	if tempo <= 0 {
		return fmt.Errorf("click: invalid tempo %d", tempo)
	}

	buf := renderBeat(tempo)

	cmd := exec.Command("sox",
		"-t", "raw", "-r", fmt.Sprintf("%d", sampleRate), "-e", "signed", "-b", "16", "-c", "1", "--endian", "little",
		"-",  // read from stdin
		"-d", // play to the default output device
	)
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
	err := s.cmd.Process.Kill()
	s.cmd.Wait()
	s.cmd = nil
	s.stdin = nil
	if err != nil {
		return fmt.Errorf("click: stopping sox: %w", err)
	}
	return nil
}
