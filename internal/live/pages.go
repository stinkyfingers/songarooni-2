package live

import (
	_ "embed"
	"net/http"
)

// camera.html and viewer.html are Phase 1's standalone test pages — not
// part of the real control panel or kiosk display, just enough to prove
// the signaling relay actually works end to end (one device as the
// camera, another as the viewer) before anything in internal/slideshow
// or internal/control-panel changes. See plans/web-rtc.md.

//go:embed camera.html
var cameraPage []byte

//go:embed viewer.html
var viewerPage []byte

func (s *Server) handleCameraPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(cameraPage)
}

func (s *Server) handleViewerPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(viewerPage)
}
