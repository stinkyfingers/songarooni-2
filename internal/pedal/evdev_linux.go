//go:build linux

package pedal

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// retryInterval is how often Watch tries to (re-)find the pedal's device:
// on startup if it's not connected yet, and again after any read error
// (including the one caused by a Bluetooth reconnect recreating the device
// node under a new name mid-show). A few seconds of pedal unresponsiveness
// after a reconnect is a trivial cost next to triggerhappy's failure mode,
// which was "never recovers until something notices and restarts it".
const retryInterval = 3 * time.Second

// Watch finds the foot pedal's evdev device (matched case-insensitively
// against nameSubstr) and reads it until ctx is done, calling c's methods
// for each recognized keypress. If the device isn't found, or reading from
// it ever fails, it keeps retrying every retryInterval — this is what makes
// it survive a Bluetooth reconnect mid-show, unlike triggerhappy.
func Watch(ctx context.Context, nameSubstr string, c Controller) {
	for {
		if ctx.Err() != nil {
			return
		}

		f, err := findDevice(nameSubstr)
		if err != nil {
			log.Printf("pedal: %v; retrying in %s", err, retryInterval)
			if !sleepOrDone(ctx, retryInterval) {
				return
			}
			continue
		}

		log.Printf("pedal: reading %s", f.Name())
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				f.Close() // unblocks the Read below
			case <-done:
			}
		}()

		err = readEvents(f, c)
		close(done)
		f.Close()

		if ctx.Err() != nil {
			return
		}
		log.Printf("pedal: lost %s (%v); looking for it again", f.Name(), err)
		if !sleepOrDone(ctx, retryInterval) {
			return
		}
	}
}

// sleepOrDone waits for d, returning false early (without the full delay)
// if ctx is done first.
func sleepOrDone(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// findDevice scans /dev/input/event* for a device whose name (via
// EVIOCGNAME) contains nameSubstr, case-insensitively. Devices that can't be
// opened (e.g. no permission — the process needs to be in the `input`
// group) or queried are skipped rather than treated as fatal, since
// /dev/input holds plenty of unrelated devices (mice, the Pi's own
// keyboard, etc.) that simply aren't the pedal.
func findDevice(nameSubstr string) (*os.File, error) {
	matches, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		return nil, fmt.Errorf("listing /dev/input: %w", err)
	}

	want := strings.ToLower(nameSubstr)
	for _, path := range matches {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		name, err := evdevName(f)
		if err != nil {
			f.Close()
			continue
		}
		if strings.Contains(strings.ToLower(name), want) {
			return f, nil
		}
		f.Close()
	}
	return nil, fmt.Errorf("no input device matching %q found under /dev/input", nameSubstr)
}

// evdevName fetches a device's name via the EVIOCGNAME ioctl.
func evdevName(f *os.File) (string, error) {
	const bufSize = 256
	buf := make([]byte, bufSize)

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), eviocgname(bufSize), uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return "", errno
	}

	if i := indexByte(buf, 0); i >= 0 {
		buf = buf[:i]
	}
	return string(buf), nil
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

// eviocgname computes the EVIOCGNAME(len) ioctl request number — _IOC(_IOC_READ,
// 'E', 0x06, len) per <linux/input.h> — a fixed, stable part of the kernel's
// evdev ABI, not something that changes across kernel versions.
func eviocgname(size int) uintptr {
	const (
		iocRead  = 2
		typeE    = 'E'
		nr       = 0x06
		nrShift  = 0
		typShift = 8
		szShift  = 16
		dirShift = 30
	)
	return (iocRead << dirShift) | (uintptr(typeE) << typShift) | (nr << nrShift) | (uintptr(size) << szShift)
}
