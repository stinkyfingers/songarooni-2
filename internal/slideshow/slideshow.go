// Package slideshow serves a kiosk-mode slideshow page: a plain HTML/JS
// page that polls the main server's /api/state (a different origin — see
// internal/control-panel's CORS allowance on that route) and cycles
// through the current song's slides itself, fetched from this server's own
// /slides and /images/ routes. Deliberately kept on its own port rather
// than merged into the control panel's mux — see
// plans/browser-slideshow-plan.md for the reasoning. This replaced an
// earlier feh-backed display (see that same plan doc for why) — this is
// now the only display backend.
package slideshow

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"

	"songarooni/internal/slides"
)

//go:embed logo.png
var embeddedLogo []byte

//go:embed bkgd.jpg
var embeddedBkgd []byte

// Slideshow serves the kiosk page over HTTP. It holds no "currently
// showing" state of its own — the page it serves polls the main server for
// that itself (see poll() in web/app.js) — so unlike a push-driven display,
// the Controller it runs alongside just uses a no-op Display.
type Slideshow struct {
	addr            string
	parentDir       string // config's SlideDir
	songMap         map[int]string
	logoEvery       int
	commercialEvery int
	intervalMS      int
	apiPort         string // the main server's port, for the page's cross-origin poll of /api/state
}

// New prepares a Slideshow listening on addr. apiPort is the main server's
// listen port (parsed from config.yaml's addr) — the kiosk browser always
// reaches the main server at the same hostname it reaches this one at,
// just a different port, so only the port number needs to travel to the
// page, not a full URL.
func New(addr, parentDir string, songMap map[int]string, logoEvery, commercialEvery, slideIntervalSeconds int, apiPort string) *Slideshow {
	return &Slideshow{
		addr:            addr,
		parentDir:       parentDir,
		songMap:         songMap,
		logoEvery:       logoEvery,
		commercialEvery: commercialEvery,
		intervalMS:      slideIntervalSeconds * 1000,
		apiPort:         apiPort,
	}
}

// Run serves the kiosk page and its assets until ctx is done.
func (s *Slideshow) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config", s.handleConfig)
	mux.HandleFunc("GET /logo.png", s.handleLogo)
	mux.HandleFunc("GET /bkgd.jpg", s.handleBkgd)
	mux.HandleFunc("GET /slides", s.handleSlides)
	mux.HandleFunc("GET /images/", s.handleImages)
	mux.Handle("GET /", http.FileServerFS(webAssets)) // index.html, app.js, styles.css

	server := &http.Server{Addr: s.addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()

	select {
	case <-ctx.Done():
		_ = server.Shutdown(context.Background())
		return ctx.Err()
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}

func (s *Slideshow) handleImages(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/images/", http.FileServer(http.Dir(s.parentDir))).ServeHTTP(w, r)
}

// handleConfig tells the static page the two values it can't know on its
// own: the main server's port (for its cross-origin /api/state poll) and
// how long each slide should hold before crossfading. Kept as a separate
// endpoint rather than templated into index.html so index.html/app.js stay
// fully static, embedded files like internal/control-panel's.
func (s *Slideshow) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		APIPort    string `json:"apiPort"`
		IntervalMs int    `json:"intervalMs"`
	}{s.apiPort, s.intervalMS})
}

func (s *Slideshow) handleLogo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(embeddedLogo)
}

func (s *Slideshow) handleBkgd(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(embeddedBkgd)
}

// handleSlides resolves ?id=<song id> to its ordered slide URLs (JSON
// array of strings), using internal/slides for the actual
// DefaultID/LogoID/missing-directory/shuffle/logo-insertion resolution.
func (s *Slideshow) handleSlides(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, "invalid or missing id", http.StatusBadRequest)
		return
	}

	filenames, err := slides.Resolve(s.parentDir, s.songMap, s.logoEvery, s.commercialEvery, id)
	if err != nil {
		// just show logo
		filenames = []string{""}
	}

	urls := make([]string, len(filenames))
	for i, f := range filenames {
		urls[i] = slideURL(f)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // a song's own deck is shuffled fresh per Resolve call; never cache
	if err := json.NewEncoder(w).Encode(urls); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// slideURL maps one of deck's Files entries to the URL that'll actually
// serve it: "/logo.png" for a logo marker, otherwise under whichever of
// /images/song/ or /images/default/ matches deck.Base — url.URL handles
// percent-encoding any special characters in filenames.
func slideURL(file string) string {
	if file == "" {
		return "/logo.png"
	}
	return filepath.Join("/images", file)
}
