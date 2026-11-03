package slideshow

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// minHealthyUptime is how long a launched browser process has to stay up
// before an exit counts as "it ran, then crashed" rather than "it never
// really started." Below this, after maxConsecutiveFastExits in a row,
// LaunchKiosk stops retrying instead of looping forever — see its doc.
// retryDelay is the pause between restart attempts. All three are vars,
// not consts, so a test can override them instead of needing real seconds
// (or a correctly-guessed-in-advance process-spawn overhead) to exercise
// the cap.
var (
	minHealthyUptime        = 5 * time.Second
	maxConsecutiveFastExits = 3
	retryDelay              = time.Second
)

// LaunchKiosk resolves the kiosk browser command (override if non-empty —
// config.yaml's browser_command, with "{{url}}" substituted — otherwise
// auto-detected per platform) and runs it against url, restarting it if it
// exits unexpectedly — the same supervision pattern internal/slideshow
// uses for feh — until ctx is done.
//
// One real failure mode this guards against: Chrome/Chromium's
// single-instance behavior. If an instance using the same profile
// directory is already running, a new launch just hands off to it (opening
// another tab there) and exits almost immediately — which looks identical
// to "it crashed" from here. Without a cap, that turns into an unbounded
// pileup of tabs in the same window, restarted once a second forever. Two
// defenses: a dedicated profile directory (see browserArgs) so this
// process's launches never hand off to some other already-running Chrome
// instance in the first place, and — in case anything else causes a fast
// exit loop regardless — giving up after a few consecutive fast exits
// rather than retrying forever.
func LaunchKiosk(ctx context.Context, url string, override []string, noKiosk bool) error {
	cmd, err := resolveBrowserCommand(override, url, noKiosk)
	if err != nil {
		return err
	}
	log.Printf("slideshow: launching kiosk browser: %v", cmd)

	fastExits := 0
	for {
		start := time.Now()
		execCmd := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
		runErr := execCmd.Run()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		uptime := time.Since(start)

		var giveUp bool
		fastExits, giveUp = nextFastExitCount(fastExits, uptime)
		if giveUp {
			return fmt.Errorf("kiosk browser exited within %s, %d times in a row (last error: %v) — likely handed off to an already-running browser instance rather than actually crashing; giving up rather than piling up more windows/tabs. Close any existing browser windows and restart songarooni, or check browser_command in config.yaml",
				minHealthyUptime, fastExits, runErr)
		}

		log.Printf("slideshow: kiosk browser exited after %s (%v); restarting", uptime.Round(time.Millisecond), runErr)
		time.Sleep(retryDelay)
	}
}

// nextFastExitCount updates the consecutive-fast-exit count given how long
// the last launch stayed up: a healthy (>= minHealthyUptime) run resets it
// to 0; a fast exit increments it, and giveUp reports whether that's now
// hit maxConsecutiveFastExits. A pure function so the counting logic is
// testable without actually spawning processes.
func nextFastExitCount(count int, uptime time.Duration) (newCount int, giveUp bool) {
	if uptime >= minHealthyUptime {
		return 0, false
	}
	newCount = count + 1
	return newCount, newCount >= maxConsecutiveFastExits
}

func resolveBrowserCommand(override []string, url string, noKiosk bool) ([]string, error) {
	if len(override) > 0 {
		return substituteURL(override, url), nil
	}
	if runtime.GOOS == "darwin" {
		return darwinBrowserCommand(url, noKiosk)
	}
	return linuxBrowserCommand(url, noKiosk)
}

func substituteURL(args []string, url string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.ReplaceAll(a, "{{url}}", url)
	}
	return out
}

// browserArgs is the common kiosk flag set, for whichever binary gets
// launched.
//   - --user-data-dir points at a profile directory dedicated to this kiosk
//     (not the user's own default profile) specifically so a relaunch never
//     hands off to some other already-running Chrome/Chromium instance —
//     see LaunchKiosk's doc for why that matters.
//   - --password-store=basic skips the OS secret-storage service
//     (GNOME Keyring/KWallet/libsecret) entirely. Without it, a fresh
//     profile's first run can block on a keyring-unlock prompt that
//     --kiosk mode never properly presents or lets anyone dismiss,
//     leaving the page stuck on a blank screen forever — exactly the
//     symptom this was added to fix (see
//     plans/browser-slideshow-plan.md's Pi debugging notes). Harmless on
//     platforms without this concept (e.g. macOS).
func browserArgs(url string, noKiosk bool) []string {
	args := []string{
		"--app=" + url,
		"--noerrdialogs",
		"--disable-infobars",
		"--password-store=basic",
		"--user-data-dir=" + kioskProfileDir(),
	}
	if !noKiosk {
		args = append(args, "--kiosk")
	}
	return args
}

func kioskProfileDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "songarooni", "kiosk-browser-profile")
}

// linuxBrowserCommand tries the Chromium-family binary names actually seen
// across Raspberry Pi OS versions — "chromium-browser" on Buster/Bullseye,
// just "chromium" on current Bookworm — rather than requiring
// browser_command to be set correctly up front.
func linuxBrowserCommand(url string, noKiosk bool) ([]string, error) {
	candidates := []string{"chromium-browser", "chromium", "google-chrome", "google-chrome-stable"}
	for _, bin := range candidates {
		if path, err := exec.LookPath(bin); err == nil {
			return append([]string{path}, browserArgs(url, noKiosk)...), nil
		}
	}
	return nil, fmt.Errorf("no kiosk-capable browser found on PATH (tried %v); set browser_command in config.yaml", candidates)
}

// darwinBrowserCommand is for Mac dev-testing: Chrome isn't normally a bare
// PATH binary on macOS, so check the standard app bundle path directly,
// falling back to `open -a` if that's not where it's installed.
func darwinBrowserCommand(url string, noKiosk bool) ([]string, error) {
	const bundlePath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	if _, err := os.Stat(bundlePath); err == nil {
		return append([]string{bundlePath}, browserArgs(url, noKiosk)...), nil
	}
	if _, err := exec.LookPath("open"); err == nil {
		return append([]string{"open", "-a", "Google Chrome", "--args"}, browserArgs(url, noKiosk)...), nil
	}
	return nil, fmt.Errorf("no Chrome found (checked %s and `open -a`); set browser_command in config.yaml", bundlePath)
}
