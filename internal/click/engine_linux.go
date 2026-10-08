package click

import (
	"fmt"
	"io"
	"log"
	"os/exec"
)

// New returns an AplayClicker targeting device if the aplay binary is on
// PATH, or a NoopClicker (with a logged warning) otherwise — so the rest of
// the app runs unchanged on a Pi that hasn't had alsa-utils installed yet.
func New(device string) Clicker {
	if _, err := exec.LookPath("aplay"); err != nil {
		log.Printf("click: aplay not found on PATH; using a silent no-op clicker (%v)", err)
		return &NoopClicker{}
	}
	return &AplayClicker{device: device}
}

// AplayClicker pipes a looped one-beat PCM buffer into an aplay subprocess.
type AplayClicker struct {
	device string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stopCh chan struct{}
}

func (a *AplayClicker) Start(tempo int) error {
	if a.cmd != nil {
		return fmt.Errorf("click: already running")
	}
	if tempo <= 0 {
		return fmt.Errorf("click: invalid tempo %d", tempo)
	}

	buf := renderBeat(tempo)

	cmd := exec.Command("aplay", "-q", "-D", a.device, "-f", "S16_LE", "-r", fmt.Sprintf("%d", sampleRate), "-c", "1", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("click: creating stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("click: starting aplay: %w", err)
	}

	a.cmd = cmd
	a.stdin = stdin
	a.stopCh = make(chan struct{})

	go func() {
		for {
			select {
			case <-a.stopCh:
				return
			default:
				if _, err := stdin.Write(buf); err != nil {
					return // pipe closed by Stop, or aplay exited
				}
			}
		}
	}()

	return nil
}

func (a *AplayClicker) Stop() error {
	if a.cmd == nil {
		return nil
	}
	close(a.stopCh)
	a.stdin.Close()
	err := a.cmd.Process.Kill()
	a.cmd.Wait()
	a.cmd = nil
	a.stdin = nil
	if err != nil {
		return fmt.Errorf("click: stopping aplay: %w", err)
	}
	return nil
}
