// Command songarooni runs the band control server: song selection, the
// slide display, and the metronome click, all driven from a phone's
// browser over the local network.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	control_panel "songarooni/internal/control-panel"

	"songarooni/internal/click"
	"songarooni/internal/config"
	"songarooni/internal/pedal"
	"songarooni/internal/slideshow"
	"songarooni/internal/songs"
	"songarooni/internal/state"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "songarooni:", err)
		os.Exit(1)
	}

	logFile, err := os.Create(cfg.LogFile) // truncates on every startup, see initial-plan.md Phase 6
	if err != nil {
		fmt.Fprintln(os.Stderr, "songarooni: opening log file:", err)
		os.Exit(1)
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	catalog, extraFields, err := songs.LoadFile(cfg.SongsCSV, cfg.SlideDir)
	if err != nil {
		log.Fatalf("songs.csv invalid, refusing to start:\n%v", err)
	}
	log.Printf("loaded %d songs from %s (extra fields: %v)", len(catalog), cfg.SongsCSV, extraFields)

	displayedFields := validDisplayedFields(cfg.DisplayedFields, extraFields)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	display := buildDisplay(ctx, cfg, catalog)
	var clicker click.Clicker
	clicker = &click.NoopClicker{} // default to a no-op clicker if no supported audio player is found on PATH
	if !cfg.NoClick {
		clicker = click.New(cfg.AudioDevice, clampVolume(cfg.ClickVolume), cfg.ClickFrequency)
	}
	controller := state.New(catalog, clicker, display)

	go func() {
		if err := control_panel.Run(ctx, controller, extraFields, displayedFields, cfg); err != nil && ctx.Err() == nil {
			log.Fatalf("control panel failed: %v", err)
		}
	}()

	go pedal.Watch(ctx, cfg.PedalDeviceName, controller)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down")
	_ = controller.Stop() // stop makes sure the click dies with the process, not just HTTP
	cancel()
}

// buildDisplay starts the slideshow kiosk server and launches a kiosk
// browser pointed at it. The Controller's Display stays a no-op: the kiosk
// page figures out what to show by polling the control panel's /api/state
// itself (see plans/browser-slideshow-plan.md) rather than the Go side
// pushing to it.
func buildDisplay(ctx context.Context, cfg config.Config, catalog []songs.Song) state.Display {
	songMap := make(map[int]string, len(catalog))
	for _, s := range catalog {
		songMap[s.ID] = s.Slides
	}

	// cfg.Addr is always a bare ":PORT" in this app (see config.yaml's own
	// examples) — no host ever configured — so the control panel's port is
	// just whatever follows that one leading colon.
	apiPort := strings.TrimPrefix(cfg.Addr, ":")

	show := slideshow.New(cfg.BrowserSlideshowAddr, cfg.SlideDir, cfg.DefaultSlideshow, songMap, cfg.LogoFrequency, cfg.SlideIntervalSeconds, apiPort)
	go func() {
		if err := show.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("slideshow: Run exited: %v", err)
		}
	}()

	kioskPort := strings.TrimPrefix(cfg.BrowserSlideshowAddr, ":")
	kioskURL := fmt.Sprintf("http://localhost:%s/", kioskPort)
	go func() {
		if err := slideshow.LaunchKiosk(ctx, kioskURL, cfg.BrowserCommand, cfg.NoKiosk); err != nil && ctx.Err() == nil {
			log.Printf("slideshow: kiosk browser: %v", err)
		}
	}()

	return noopDisplay{}
}

// clampVolume keeps a misconfigured click_volume (negative, or above 1) from
// turning into a crash or silent garbage in renderBeat — a bad volume value
// should just play too loud/quiet, loudly logged, not take anything down.
func clampVolume(v float64) float64 {
	switch {
	case v < 0:
		log.Printf("config: click_volume %v is negative; using 0", v)
		return 0
	case v > 1:
		log.Printf("config: click_volume %v is above 1; using 1", v)
		return 1
	default:
		return v
	}
}

// validDisplayedFields filters configured to only the names that actually
// exist in extraFields, logging a warning for any that don't — a typo in
// config.yaml's displayed_fields shouldn't refuse to start the app.
func validDisplayedFields(configured, extraFields []string) []string {
	known := make(map[string]bool, len(extraFields))
	for _, f := range extraFields {
		known[f] = true
	}
	out := []string{} // never nil: see the matching note in songs.LoadFile
	for _, f := range configured {
		if known[f] {
			out = append(out, f)
		} else {
			log.Printf("config: displayed_fields entry %q is not a songs.csv column; ignoring it", f)
		}
	}
	return out
}

type noopDisplay struct{}

func (noopDisplay) Show(id int) {
	log.Printf("display: (no-op) would show song id %d", id)
}
