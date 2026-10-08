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
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"songarooni/internal/click"
	"songarooni/internal/config"
	"songarooni/internal/httpapi"
	"songarooni/internal/slideshow"
	"songarooni/internal/songs"
	"songarooni/internal/state"

	webassets "songarooni"
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
	clicker := click.New(cfg.AudioDevice)
	controller := state.New(catalog, clicker, display)

	mux := http.NewServeMux()
	mux.Handle("/api/", httpapi.Handler(controller, extraFields, displayedFields))
	mux.Handle("/", noCache(http.FileServerFS(webassets.FS)))

	server := &http.Server{Addr: cfg.Addr, Handler: mux}

	go func() {
		log.Printf("listening on %s", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down")
	_ = controller.Stop() // stop makes sure the click dies with the process, not just HTTP
	cancel()
	_ = server.Shutdown(context.Background())
}

// buildDisplay runs the real feh-backed Slideshow if feh is on PATH, or a
// logging no-op otherwise — e.g. on a Mac used for development, per
// initial-plan.md Phase 1's "keep hardware-specific operations behind small
// interfaces" guidance.
func buildDisplay(ctx context.Context, cfg config.Config, catalog []songs.Song) state.Display {
	if _, err := exec.LookPath("feh"); err != nil {
		log.Printf("display: feh not found on PATH; using a no-op display (%v)", err)
		return noopDisplay{}
	}

	songMap := make(map[int]string, len(catalog))
	for _, s := range catalog {
		songMap[s.ID] = s.Slides
	}

	show, err := slideshow.New(cfg.SlideDir, cfg.DefaultSlideshow, songMap, cfg.LogoFrequency, cfg.SlideIntervalSeconds)
	if err != nil {
		// A slides-only problem (e.g. a bad slide_dir) must not take the
		// click and the rest of the app down with it — fall back to the
		// same no-op used when feh itself is missing.
		log.Printf("display: %v; using a no-op display (click still works)", err)
		return noopDisplay{}
	}
	go func() {
		if err := show.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("display: Run exited: %v", err)
		}
	}()
	show.Start()
	return show
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

// noCache stops the browser from caching the control page's static assets.
// This is a small, locally-served app under active development — serving a
// stale cached app.js against a server whose API shape has since changed
// (as happened across this session's field renames) is a worse failure mode
// than re-fetching a few KB on every load.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

type noopDisplay struct{}

func (noopDisplay) Show(id int) {
	log.Printf("display: (no-op) would show song id %d", id)
}
