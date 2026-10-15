package click

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
)

// New returns an AplayClicker targeting device at the given volume (0 to 1)
// if the aplay binary is on PATH, or a NoopClicker (with a logged warning)
// otherwise — so the rest of the app runs unchanged on a Pi that hasn't had
// alsa-utils installed yet.
func New(device string, volume float64, frequency float64) Clicker {
	if _, err := exec.LookPath("aplay"); err != nil {
		log.Printf("click: aplay not found on PATH; using a silent no-op clicker (%v)", err)
		return &NoopClicker{}
	}
	return &AplayClicker{device: device, volume: volume, frequency: frequency}
}

// AplayClicker pipes a looped one-beat PCM buffer into an aplay subprocess.
type AplayClicker struct {
	device    string
	volume    float64
	frequency float64

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stopCh chan struct{}
	done   chan struct{} // closed once cmd.Wait() returns
}

func (a *AplayClicker) Start(tempo int) error {
	if a.cmd != nil {
		return fmt.Errorf("click: already running")
	}
	if tempo <= 0 {
		return fmt.Errorf("click: invalid tempo %d", tempo)
	}

	buf := renderBeat(tempo, a.volume, a.frequency)

	// -t raw is required: aplay defaults to expecting a WAV header when no
	// file type is given, and will reject (or misread) headerless PCM
	// without it — silently, since nothing here was capturing its stderr
	// until now.
	cmd := exec.Command("aplay", "-q", "-t", "raw", "-D", a.device, "-f", "S16_LE", "-r", fmt.Sprintf("%d", sampleRate), "-c", "1", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

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
	a.done = make(chan struct{})

	go func() {
		defer close(a.done)
		waitErr := cmd.Wait()
		select {
		case <-a.stopCh:
			// Expected: Stop() killed it.
		default:
			// aplay exited on its own — e.g. a bad device string or a
			// format it rejected. Previously this vanished silently; now
			// it's at least visible in the log.
			log.Printf("click: aplay exited unexpectedly (%v); stderr: %s", waitErr, strings.TrimSpace(stderr.String()))
		}
	}()

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
	killErr := a.cmd.Process.Kill()
	<-a.done // wait for the single cmd.Wait() call so aplay is fully reaped
	a.cmd = nil
	a.stdin = nil
	if killErr != nil {
		return fmt.Errorf("click: stopping aplay: %w", killErr)
	}
	return nil
}
