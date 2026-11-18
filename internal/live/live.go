// Package live implements a minimal WebRTC signaling relay: a camera
// (an iOS Safari page using getUserMedia) and a viewer (eventually the
// kiosk slideshow page) each connect over a WebSocket, and the server
// blindly relays whatever JSON message one sends to the other — SDP
// offers/answers, ICE candidates. It never touches the actual video
// itself; that travels directly between the two browsers once WebRTC
// negotiation completes, not through this process. See
// plans/web-rtc.md for the full design and why it's shaped this way.
package live

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// Server relays signaling messages between one connected camera and one
// connected viewer, and tracks whether a camera is currently connected.
type Server struct {
	addr     string
	certFile string
	keyFile  string

	mu      sync.Mutex
	camera  *websocket.Conn
	viewer  *websocket.Conn
	session string // non-empty while a camera is connected; identifies the current live session
}

// New prepares a Server listening on addr, generating (or reusing) this
// Pi's self-signed TLS cert — HTTPS is required, not optional: iOS
// Safari's getUserMedia refuses to run outside a secure context.
func New(addr string) (*Server, error) {
	certFile, keyFile, err := certPaths()
	if err != nil {
		return nil, err
	}
	return &Server{addr: addr, certFile: certFile, keyFile: keyFile}, nil
}

// Run serves the signaling WebSocket and the Phase 1 test pages over
// HTTPS until ctx is done.
//
// Listens on "tcp4" specifically, not the bare "tcp" ListenAndServeTLS
// would use (which is dual-stack: both IPv4 and the host's IPv6
// link-local address). Confirmed on a real device: mDNS can hand a phone
// an IPv6 AAAA record for songarooni.local alongside the IPv4 one, and
// if that IPv6 address isn't actually reachable from wherever the phone
// is attached, Happy-Eyeballs-style IPv6-first racing produces a
// confusing hang/retry-loop instead of a clean failure. Refusing IPv6
// here entirely means any client that tries it first gets a fast,
// unambiguous connection-refused and falls back to IPv4 immediately.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("GET /signal", websocket.Handler(s.handleSignal))
	mux.HandleFunc("GET /camera", s.handleCameraPage)
	mux.HandleFunc("GET /viewer", s.handleViewerPage)

	listener, err := net.Listen("tcp4", s.addr)
	if err != nil {
		return err
	}

	server := &http.Server{Addr: s.addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() { errCh <- server.ServeTLS(listener, s.certFile, s.keyFile) }()

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

// ActiveSession reports the current camera session's id, and whether
// there is one. This is the interface Phase 2's Slideshow.ShowLive uses
// to check "is there anything to show" before cutting the kiosk display
// over to it.
func (s *Server) ActiveSession() (id string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session, s.session != ""
}

// handleSignal dispatches a new /signal WebSocket connection to the
// camera or viewer relay loop based on its ?role= query param.
func (s *Server) handleSignal(ws *websocket.Conn) {
	switch role := ws.Request().URL.Query().Get("role"); role {
	case "camera":
		s.handleCamera(ws)
	case "viewer":
		s.handleViewer(ws)
	default:
		log.Printf("live: /signal: unrecognized role %q", role)
	}
}

func (s *Server) handleCamera(ws *websocket.Conn) {
	sessionID := newSessionID()

	s.mu.Lock()
	// Scoped to one active camera at a time for v1 (see plans/web-rtc.md)
	// — a second camera connecting simply replaces the first here;
	// whatever goroutine was relaying the old one just keeps running
	// harmlessly until its own connection errors out or closes.
	s.camera = ws
	s.session = sessionID
	s.mu.Unlock()
	log.Printf("live: camera connected (session %s)", sessionID)

	defer func() {
		s.mu.Lock()
		if s.camera == ws {
			s.camera = nil
			s.session = ""
		}
		s.mu.Unlock()
		log.Printf("live: camera disconnected (session %s)", sessionID)
	}()

	s.relay(ws, func() *websocket.Conn {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.viewer
	})
}

func (s *Server) handleViewer(ws *websocket.Conn) {
	s.mu.Lock()
	s.viewer = ws
	s.mu.Unlock()
	log.Println("live: viewer connected")

	defer func() {
		s.mu.Lock()
		if s.viewer == ws {
			s.viewer = nil
		}
		s.mu.Unlock()
		log.Println("live: viewer disconnected")
	}()

	s.relay(ws, func() *websocket.Conn {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.camera
	})
}

// relay reads messages from ws until it errors or closes, forwarding
// each one verbatim to whatever other() currently returns. No part of
// this parses or understands the WebRTC signaling content (SDP, ICE
// candidates) — it's a dumb relay, by design; that's the whole point of
// keeping WebRTC itself out of Go entirely.
func (s *Server) relay(ws *websocket.Conn, other func() *websocket.Conn) {
	for {
		var msg string
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			return
		}
		dst := other()
		if dst == nil {
			continue // nobody on the other side yet to relay to
		}
		if err := websocket.Message.Send(dst, msg); err != nil {
			log.Printf("live: relay: %v", err)
		}
	}
}

func newSessionID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
