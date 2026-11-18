package live

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func dialSignal(t *testing.T, ts *httptest.Server, role string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/signal?role=" + role
	conn, err := websocket.Dial(wsURL, "", ts.URL)
	if err != nil {
		t.Fatalf("dialing as %s: %v", role, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// relay's whole job is to forward whatever one side sends to the other,
// verbatim, with no understanding of WebRTC semantics — this exercises
// that end to end over a real WebSocket connection in both directions,
// standing in for SDP offer/answer and ICE candidate exchange without
// needing a real browser or camera.
func TestRelayForwardsMessagesBetweenCameraAndViewer(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	mux.Handle("/signal", websocket.Handler(s.handleSignal))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cameraConn := dialSignal(t, ts, "camera")
	viewerConn := dialSignal(t, ts, "viewer")

	// The server registers each role as the first thing its relay
	// goroutine does, right after the handshake that unblocks Dial above
	// — this margin is just for that goroutine to get scheduled.
	time.Sleep(50 * time.Millisecond)
	_ = cameraConn.SetDeadline(time.Now().Add(2 * time.Second))
	_ = viewerConn.SetDeadline(time.Now().Add(2 * time.Second))

	const offer = `{"type":"offer","sdp":"fake-sdp"}`
	if err := websocket.Message.Send(cameraConn, offer); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := websocket.Message.Receive(viewerConn, &got); err != nil {
		t.Fatalf("viewer never received the camera's message: %v", err)
	}
	if got != offer {
		t.Fatalf("expected %q relayed verbatim, got %q", offer, got)
	}

	const answer = `{"type":"answer","sdp":"fake-answer"}`
	if err := websocket.Message.Send(viewerConn, answer); err != nil {
		t.Fatal(err)
	}
	if err := websocket.Message.Receive(cameraConn, &got); err != nil {
		t.Fatalf("camera never received the viewer's message: %v", err)
	}
	if got != answer {
		t.Fatalf("expected %q relayed verbatim, got %q", answer, got)
	}
}

// A message sent before the other side has connected at all must not
// block or panic the relay loop — it's just dropped (nobody to relay to
// yet), same as a viewer connecting before any camera has.
func TestRelayDropsMessagesWithNoOtherSideConnected(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	mux.Handle("/signal", websocket.Handler(s.handleSignal))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	viewerConn := dialSignal(t, ts, "viewer")
	time.Sleep(50 * time.Millisecond)

	// Nothing to assert on receipt (nobody relayed it anywhere) — this is
	// really just confirming the viewer connection stays alive and the
	// server doesn't crash with no camera ever having connected.
	_ = viewerConn.SetDeadline(time.Now().Add(200 * time.Millisecond))
	var discard string
	if err := websocket.Message.Receive(viewerConn, &discard); err == nil {
		t.Fatal("expected no message to arrive with no camera ever connected")
	}
}

func TestActiveSessionReflectsCameraConnection(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	mux.Handle("/signal", websocket.Handler(s.handleSignal))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	if _, ok := s.ActiveSession(); ok {
		t.Fatal("expected no active session before any camera connects")
	}

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/signal?role=camera"
	conn, err := websocket.Dial(wsURL, "", ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	id, ok := waitForActiveSession(s, true)
	if !ok || id == "" {
		t.Fatal("expected a non-empty active session once a camera connects")
	}

	conn.Close()

	if _, ok := waitForActiveSession(s, false); ok {
		t.Fatal("expected no active session after the camera disconnects")
	}
}

// Two cameras connecting means "last one wins" for v1 (see
// plans/web-rtc.md) — the second camera's session replaces the first's.
func TestActiveSessionIsReplacedByASecondCamera(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	mux.Handle("/signal", websocket.Handler(s.handleSignal))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/signal?role=camera"
	first, err := websocket.Dial(wsURL, "", ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	firstID, _ := waitForActiveSession(s, true)

	second, err := websocket.Dial(wsURL, "", ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	var secondID string
	for i := 0; i < 200; i++ {
		id, ok := s.ActiveSession()
		if ok && id != firstID {
			secondID = id
			break
		}
		time.Sleep(time.Millisecond)
	}
	if secondID == "" {
		t.Fatal("expected the second camera's session to replace the first's")
	}
}

func waitForActiveSession(s *Server, want bool) (string, bool) {
	var id string
	var ok bool
	for i := 0; i < 200; i++ {
		id, ok = s.ActiveSession()
		if ok == want {
			return id, ok
		}
		time.Sleep(time.Millisecond)
	}
	return id, ok
}

// Confirmed on a real device: ListenAndServeTLS's default dual-stack
// listener let a client try an IPv6 link-local address that turned out
// to be unreachable from where it actually was, producing a confusing
// connect/retry loop instead of a clean failure. Run must listen on
// IPv4 only so any client that tries IPv6 first gets refused instantly
// and falls back, rather than whatever that was.
func TestRunListensIPv4OnlyNotIPv6(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := generateSelfSignedCert(certFile, keyFile); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	// Find a free port by briefly listening on it, then hand that same
	// address to Run — the usual pick-a-free-port pattern; the brief
	// TOCTOU window between closing probe and Run's own Listen is fine
	// in practice for a test like this.
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := probe.Addr().String()
	probe.Close()

	s := &Server{addr: addr, certFile: certFile, keyFile: keyFile}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() { runErr <- s.Run(ctx) }()

	var conn net.Conn
	for i := 0; i < 200; i++ {
		conn, err = net.DialTimeout("tcp4", addr, 50*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("expected to reach the server over IPv4, got: %v", err)
	}
	conn.Close()

	_, port, _ := net.SplitHostPort(addr)
	ipv6Addr := net.JoinHostPort("::1", port)
	if conn, err := net.DialTimeout("tcp6", ipv6Addr, 200*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("expected the IPv6 loopback dial to be refused, but it connected")
	}

	cancel()
	select {
	case <-runErr:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after ctx was canceled")
	}
}
