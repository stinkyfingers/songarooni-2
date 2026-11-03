package slideshow

import (
	"context"
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
