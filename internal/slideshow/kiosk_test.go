package slideshow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNextFastExitCount(t *testing.T) {
	origUptime, origMax := minHealthyUptime, maxConsecutiveFastExits
	minHealthyUptime = time.Second
	maxConsecutiveFastExits = 3
	defer func() { minHealthyUptime, maxConsecutiveFastExits = origUptime, origMax }()

	count := 0
	var giveUp bool

	// Three fast exits in a row trips the cap.
	for i := 0; i < 2; i++ {
		count, giveUp = nextFastExitCount(count, 100*time.Millisecond)
		if giveUp {
			t.Fatalf("should not give up after only %d fast exit(s)", i+1)
		}
	}
	count, giveUp = nextFastExitCount(count, 100*time.Millisecond)
	if !giveUp || count != 3 {
		t.Fatalf("expected giveUp after 3 consecutive fast exits, got count=%d giveUp=%v", count, giveUp)
	}

	// A healthy run resets the count, even right before it would've tripped.
	count, giveUp = nextFastExitCount(2, 2*time.Second)
	if giveUp || count != 0 {
		t.Fatalf("expected a healthy run to reset the count to 0, got count=%d giveUp=%v", count, giveUp)
	}
}

// A process that exits immediately, every time, mirrors Chrome's
// single-instance hand-off behavior (the launched process exits almost
// instantly because it handed off to an already-running instance) — this
// must not loop forever piling up launches, and must say why it stopped.
func TestLaunchKioskGivesUpAfterRepeatedFastExits(t *testing.T) {
	origUptime, origMax, origDelay := minHealthyUptime, maxConsecutiveFastExits, retryDelay
	// minHealthyUptime must comfortably exceed the real overhead of
	// actually forking+exiting /bin/sh (tens of milliseconds on a typical
	// machine) or every exit gets misclassified as "healthy" and the
	// counter never advances — exactly the infinite-loop bug this test
	// caught once before.
	minHealthyUptime = 500 * time.Millisecond
	maxConsecutiveFastExits = 3
	retryDelay = time.Millisecond
	defer func() { minHealthyUptime, maxConsecutiveFastExits, retryDelay = origUptime, origMax, origDelay }()

	// Bounded so a miscalibrated threshold (this test caught exactly that
	// once already — see the comment above) fails fast with a clear
	// message instead of hanging until the test binary's own timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := LaunchKiosk(ctx, "http://example.invalid", []string{"/bin/sh", "-c", "exit 0"}, false)
	if err == nil {
		t.Fatal("expected an error after repeated fast exits")
	}
	if ctx.Err() != nil {
		t.Fatalf("LaunchKiosk never gave up within the test's time budget — thresholds likely miscalibrated again; got: %v", err)
	}
	if !strings.Contains(err.Error(), "giving up") {
		t.Fatalf("expected a 'giving up' error explaining why, got: %v", err)
	}
}

// The actual deferred bug this guards against: exec.CommandContext's
// default Cancel behavior is an immediate SIGKILL, which gives Chromium
// zero chance to signal its own child processes to exit before dying —
// they're orphaned. LaunchKiosk's Cancel override must send SIGTERM
// instead, letting a well-behaved process clean up on its own. A SIGKILL
// would bypass this trap entirely (no handler runs), so the trap firing
// and writing to outFile is direct proof SIGTERM was actually what got
// sent, not just that the process eventually died somehow.
func TestLaunchKioskSendsSIGTERMNotSIGKILLOnShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this test's trap-based shell script assumes a POSIX shell")
	}

	outFile := filepath.Join(t.TempDir(), "out.txt")
	// A single long sleep (e.g. "sleep 10") doesn't work here: bash (macOS's
	// /bin/sh) defers running a trap until whatever foreground child it's
	// currently blocked on finishes, so the whole 10s would elapse before
	// the trap ever fires. Looping over short sleeps bounds that deferral
	// window to ~1s worst case instead, comfortably within this test's
	// budget, while still giving cancel() something to interrupt mid-run.
	script := fmt.Sprintf(`trap 'echo got-sigterm >> %s; exit 0' TERM; i=0; while [ $i -lt 10 ]; do sleep 1; i=$((i+1)); done`, outFile)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- LaunchKiosk(ctx, "unused", []string{"/bin/sh", "-c", script}, false)
	}()

	time.Sleep(200 * time.Millisecond) // give the child time to actually start and install its trap
	cancel()

	select {
	case err := <-done:
		if err != ctx.Err() {
			t.Fatalf("expected LaunchKiosk to return ctx.Err(), got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LaunchKiosk did not return promptly after ctx was canceled — the SIGTERM handler may be hanging, or escalation to SIGKILL isn't happening")
	}

	data, err := os.ReadFile(outFile)
	if err != nil || !strings.Contains(string(data), "got-sigterm") {
		t.Fatalf("expected the child to have received SIGTERM and run its trap; outFile contents: %q (read err: %v)", data, err)
	}
}

// --user-data-dir is the actual fix for the single-instance hand-off
// problem (not just the fast-exit cap above, which is only a safety net) —
// confirm it's always present, regardless of noKiosk.
func TestBrowserArgsIncludesDedicatedProfileDir(t *testing.T) {
	for _, noKiosk := range []bool{false, true} {
		args := browserArgs("http://example.invalid", noKiosk)
		found := false
		for _, a := range args {
			if strings.HasPrefix(a, "--user-data-dir=") {
				found = true
			}
		}
		if !found {
			t.Fatalf("noKiosk=%v: expected --user-data-dir in browserArgs, got %v", noKiosk, args)
		}
	}
}

func TestBrowserArgsOmitsKioskFlagWhenNoKiosk(t *testing.T) {
	if !contains(browserArgs("http://example.invalid", false), "--kiosk") {
		t.Fatal("expected --kiosk present when noKiosk=false")
	}
	if contains(browserArgs("http://example.invalid", true), "--kiosk") {
		t.Fatal("expected --kiosk omitted when noKiosk=true")
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestSubstituteURL(t *testing.T) {
	got := substituteURL([]string{"chrome", "--app={{url}}", "--kiosk"}, "http://x/y")
	want := []string{"chrome", "--app=http://x/y", "--kiosk"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
