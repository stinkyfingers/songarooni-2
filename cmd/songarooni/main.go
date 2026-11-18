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
	"time"

	control_panel "songarooni/internal/control-panel"

	"songarooni/internal/click"
	"songarooni/internal/config"
	"songarooni/internal/live"
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

	catalog, err := songs.LoadJSON(cfg.SongsFile, cfg.SlideDir, cfg.IncludeEmptySongs, time.Duration(cfg.SlideIntervalSeconds)*time.Second)
	if err != nil {
		log.Fatalf("songs.json invalid, refusing to start:\n%v", err)
	}
	log.Printf("loaded %d songs from %s", len(catalog), cfg.SongsFile)

	cleanCandidDir(cfg.SlideDir, catalog)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	display := buildDisplay(ctx, cfg, catalog)
	buildLive(ctx, cfg)
	var clicker click.Clicker
	clicker = &click.NoopClicker{} // default to a no-op clicker if no supported audio player is found on PATH
	if !cfg.NoClick {
		clicker = click.New(cfg.AudioDevice, cfg.ClickVolume, cfg.ClickFrequency)
	}
	controller := state.New(catalog, clicker, display)

	go func() {
		if err := control_panel.Run(ctx, controller, cfg); err != nil && ctx.Err() == nil {
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
// browser pointed at it.
func buildDisplay(ctx context.Context, cfg config.Config, catalog []songs.Song) state.Display {
	songMap := make(map[int]songs.Song, len(catalog))
	for _, s := range catalog {
		songMap[s.ID] = s
	}

	// cfg.Addr is always a bare ":PORT" in this app (see config.yaml's own
	// examples) — no host ever configured — so the control panel's port is
	// just whatever follows that one leading colon.
	apiPort := strings.TrimPrefix(cfg.Addr, ":")

	show := slideshow.New(cfg.BrowserSlideshowAddr, cfg.SlideDir, songMap, cfg.LogoFrequency, cfg.CommercialFrequency, cfg.CandidFrequency, time.Duration(cfg.SlideIntervalSeconds)*time.Second, apiPort)
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

	return show
}

// buildLive starts the WebRTC signaling server (Phase 1 of
// plans/web-rtc.md) — standalone for now, not wired into the real
// display yet. A failure here is non-fatal: Phase 1 is independent of
// the slideshow/click/pedal path, so a broken cert or a busy port
// shouldn't take down the rest of the app.
func buildLive(ctx context.Context, cfg config.Config) {
	liveServer, err := live.New(cfg.LiveAddr)
	if err != nil {
		log.Printf("live: %v", err)
		return
	}
	go func() {
		if err := liveServer.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("live: Run exited: %v", err)
		}
	}()
}

// cleanCandidDir removes any existing files in the candid slide directory.
func cleanCandidDir(slideDir string, catalog []songs.Song) {
	var dir string
	for _, c := range catalog {
		if c.Category == "candid" {
			dir = c.Slides
			break
		}
	}
	candidDir := fmt.Sprintf("%s/%s", slideDir, dir)
	if err := os.RemoveAll(candidDir); err != nil {
		log.Printf("cleaning candid dir %s: %v", candidDir, err)
	}
	if err := os.MkdirAll(candidDir, 0755); err != nil {
		log.Printf("creating candid dir %s: %v", candidDir, err)
	}
}
