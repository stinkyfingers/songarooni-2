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
	"net/http"
	"sync"
	"time"

	"songarooni/internal/songs"
	"songarooni/internal/state"
)

//go:embed bkgd.jpg
var embeddedBkgd []byte

// Slideshow serves the kiosk page over HTTP.
type Slideshow struct {
	addr            string
	parentDir       string // config's SlideDir
	songMap         map[int]songs.Song
	logoEvery       int
	commercialEvery int
	candidEvery     int
	defaultLength   time.Duration
	customIDs       state.CustomIDs
	apiPort         string // the main server's port, for the page's cross-origin poll of /api/state
	camera          CameraProxy
	stateChan       chan string

	// advanceChan carries one signal per POST /advance — the browser's way
	// of telling displayData it actually revealed the current slide, so
	// the real dwell timer can start from when it was genuinely shown
	// rather than from whenever it was pushed. Buffered so a POST never
	// blocks waiting for displayData to be ready to receive it.
	advanceChan chan struct{}

	showMu     sync.Mutex
	ctx        context.Context    // set by Run; Show's cycling goroutines exit when this is done
	cancelShow context.CancelFunc // cancels whichever deck Show previously started cycling, if any
}

// New prepares a Slideshow listening on addr. apiPort is the main server's
// listen port (parsed from config.yaml's addr) — the kiosk browser always
// reaches the main server at the same hostname it reaches this one at,
// just a different port, so only the port number needs to travel to the
// page, not a full URL.
func New(addr, parentDir string, songMap map[int]songs.Song, logoEvery, commercialEvery, candidEvery int, defaultLength time.Duration, apiPort string) *Slideshow {
	return &Slideshow{
		addr:            addr,
		parentDir:       parentDir,
		songMap:         songMap,
		logoEvery:       logoEvery,
		commercialEvery: commercialEvery,
		candidEvery:     candidEvery,
		defaultLength:   defaultLength,
		apiPort:         apiPort,
		stateChan:       make(chan string),
		advanceChan:     make(chan struct{}, 1),
		ctx:             context.Background(), // overwritten by Run; keeps Show safe to call even earlier
	}
}

func (s *Slideshow) SetCustomIDs(ids state.CustomIDs) {
	s.customIDs = ids
}

// CameraProxy streams a live camera feed through to an HTTP response —
// satisfied by *camera.Proxy (see plans/http-video.md). A small interface
// here, rather than an internal/camera import, keeps this package's only
// dependency on that one a setter call made from main.go, same shape as
// SetCustomIDs.
type CameraProxy interface {
	StreamMJPEG(w http.ResponseWriter, r *http.Request)
	Configured() bool
}

// SetCameraProxy wires in the live camera feed that ShowLive and
// /live-camera use. Left unset, ShowLive just fails with "no camera
// configured" and /live-camera 503s — safe to call either even if
// internal/camera was never configured at all.
func (s *Slideshow) SetCameraProxy(p CameraProxy) {
	s.camera = p
}

// handleLiveCamera proxies the configured camera's MJPEG stream straight
// through — the kiosk page's live <img> points here (same origin as the
// kiosk itself, so no cross-origin complication), never at the camera's
// own address directly, so the camera's Basic Auth credentials never need
// to reach the browser.
func (s *Slideshow) handleLiveCamera(w http.ResponseWriter, r *http.Request) {
	if s.camera == nil {
		http.Error(w, "camera not configured", http.StatusServiceUnavailable)
		return
	}
	s.camera.StreamMJPEG(w, r)
}

// Run serves the kiosk page and its assets until ctx is done.
func (s *Slideshow) Run(ctx context.Context) error {
	s.showMu.Lock()
	s.ctx = ctx
	s.showMu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /bkgd.jpg", s.handleBkgd)
	mux.HandleFunc("GET /images/", s.handleImages)
	mux.HandleFunc("GET /live-camera", s.handleLiveCamera)
	mux.HandleFunc("GET /events", s.sseHandler)
	mux.HandleFunc("POST /advance", s.handleAdvance)
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

// handleAdvance is the browser's "I actually revealed the current slide"
// signal (called from app.js's reveal() and its load-error path alike —
// either way, displayData should stop waiting). Non-blocking send: if a
// signal is already pending (a duplicate POST, or one that arrived after
// displayData gave up on it), this is a no-op rather than piling up a
// second one that would wrongly short-circuit a later slide's own wait.
func (s *Slideshow) handleAdvance(w http.ResponseWriter, r *http.Request) {
	select {
	case s.advanceChan <- struct{}{}:
	default:
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Slideshow) sseHandler(w http.ResponseWriter, r *http.Request) {
	// Set required SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Ensure the ResponseWriter supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	for {
		select {
		case msg := <-s.stateChan:
			w.Write([]byte("event: data\ndata: " + msg + "\n\n"))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}

}

func (s *Slideshow) handleImages(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/images/", http.FileServer(http.Dir(s.parentDir))).ServeHTTP(w, r)
}

func (s *Slideshow) handleBkgd(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(embeddedBkgd)
}
