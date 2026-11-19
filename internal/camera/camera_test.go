package camera

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// jpegMagic is enough for looksLikeImage to recognize a JPEG — it only
// checks the leading magic bytes, not a fully valid file.
var jpegMagic = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0}

func splitHostPort(t *testing.T, ts *httptest.Server) (string, int) {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func TestConfiguredReflectsTarget(t *testing.T) {
	p := New("", 8080, "", "")
	if p.Configured() {
		t.Fatal("expected Configured()=false with no IP set")
	}
	if err := p.SetTarget("192.168.1.42", 8080); err != nil {
		t.Fatalf("SetTarget: %v", err)
	}
	if !p.Configured() {
		t.Fatal("expected Configured()=true after SetTarget")
	}
	ip, port := p.Target()
	if ip != "192.168.1.42" || port != 8080 {
		t.Fatalf("expected 192.168.1.42:8080, got %s:%d", ip, port)
	}
}

func TestSetTargetRejectsNonPositivePortWithNonEmptyIP(t *testing.T) {
	p := New("", 0, "", "")
	if err := p.SetTarget("192.168.1.42", 0); err == nil {
		t.Fatal("expected an error for a non-positive port with a non-empty IP")
	}
}

func TestStreamMJPEGProxiesUpstreamBodyAndContentType(t *testing.T) {
	const wantUser, wantPass = "alice", "secret123"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mjpeg" {
			t.Errorf("expected /mjpeg, got %s", r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != wantUser || pass != wantPass {
			t.Errorf("expected Basic Auth %s:%s, got ok=%v %s:%s", wantUser, wantPass, ok, user, pass)
		}
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake mjpeg bytes"))
	}))
	defer ts.Close()

	host, port := splitHostPort(t, ts)
	p := New(host, port, wantUser, wantPass)

	req := httptest.NewRequest("GET", "/live-camera", nil)
	w := httptest.NewRecorder()
	p.StreamMJPEG(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "multipart/x-mixed-replace; boundary=frame" {
		t.Fatalf("expected the upstream Content-Type to pass through, got %q", ct)
	}
	if w.Body.String() != "fake mjpeg bytes" {
		t.Fatalf("expected the upstream body to pass through, got %q", w.Body.String())
	}
}

func TestStreamMJPEGFailsWithNoCameraConfigured(t *testing.T) {
	p := New("", 8080, "", "")
	req := httptest.NewRequest("GET", "/live-camera", nil)
	w := httptest.NewRecorder()
	p.StreamMJPEG(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestStreamMJPEGRelaysUpstreamFailureAsBadGateway(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	host, port := splitHostPort(t, ts)
	p := New(host, port, "wrong", "creds")

	req := httptest.NewRequest("GET", "/live-camera", nil)
	w := httptest.NewRecorder()
	p.StreamMJPEG(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when the upstream camera rejects the request, got %d", w.Code)
	}
}

func TestSnapshotReturnsBodyOnSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/snapshot.jpg" {
			t.Errorf("expected /snapshot.jpg, got %s", r.URL.Path)
		}
		_, _ = w.Write(jpegMagic)
	}))
	defer ts.Close()

	host, port := splitHostPort(t, ts)
	p := New(host, port, "alice", "secret123")

	body, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if string(body) != string(jpegMagic) {
		t.Fatalf("expected the snapshot body back, got %v", body)
	}
}

func TestSnapshotFailsWithNoCameraConfigured(t *testing.T) {
	p := New("", 8080, "", "")
	if _, err := p.Snapshot(context.Background()); err == nil {
		t.Fatal("expected an error with no camera configured")
	}
}

// scanPrefix is Discover's core logic, exercised directly against a
// loopback-bound test server — Discover itself can't be pointed at an
// arbitrary subnet in a test (it only scans this machine's own real,
// non-loopback IPv4 prefixes), but scanPrefix takes the prefix as a plain
// argument.
func TestScanPrefixFindsTheRightHost(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jpegMagic)
	}))
	defer ts.Close()

	_, port := splitHostPort(t, ts)
	p := New("", port, "alice", "secret123")

	// httptest binds 127.0.0.1, which is host 1 under the "127.0.0" prefix.
	found := p.scanPrefix(context.Background(), "127.0.0", port, "alice", "secret123")
	if len(found) != 1 || found[0] != "127.0.0.1" {
		t.Fatalf("expected [127.0.0.1], got %v", found)
	}
}

func TestScanPrefixIgnoresWrongCredentials(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "alice" || pass != "secret123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(jpegMagic)
	}))
	defer ts.Close()

	_, port := splitHostPort(t, ts)
	p := New("", port, "", "")

	found := p.scanPrefix(context.Background(), "127.0.0", port, "wrong", "creds")
	if len(found) != 0 {
		t.Fatalf("expected no hosts found with wrong credentials, got %v", found)
	}
}

func TestScanPrefixIgnoresNonImageResponses(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not a jpeg"))
	}))
	defer ts.Close()

	_, port := splitHostPort(t, ts)
	p := New("", port, "", "")

	found := p.scanPrefix(context.Background(), "127.0.0", port, "", "")
	if len(found) != 0 {
		t.Fatalf("expected no hosts found for a non-image response, got %v", found)
	}
}

func TestLooksLikeImage(t *testing.T) {
	if !looksLikeImage(jpegMagic) {
		t.Fatal("expected jpegMagic to look like an image")
	}
	if looksLikeImage([]byte("not a jpeg")) {
		t.Fatal("expected plain text not to look like an image")
	}
	if looksLikeImage(nil) {
		t.Fatal("expected an empty body not to look like an image")
	}
}

// A nil []string marshals to JSON `null`, not `[]` — which broke the
// control panel's `const { found } = await api(...); found.length` the
// moment Discover genuinely found nothing. Confirmed via a real scan of
// this machine's own LAN (expected to find nothing in a test environment)
// rather than asserting on scanPrefix/append's behavior directly, since
// that's exactly the code path that regressed.
func TestDiscoverNeverReturnsANilSliceOnSuccess(t *testing.T) {
	p := New("", 8080, "nobody", "nothing")
	found, err := p.Discover(context.Background())
	if err != nil {
		t.Skipf("no local IPv4 network to scan in this environment: %v", err)
	}
	if found == nil {
		t.Fatal("expected a non-nil (possibly empty) slice so JSON marshals to [] rather than null")
	}
}

func TestLocalIPv4PrefixesExcludesLoopback(t *testing.T) {
	prefixes, err := localIPv4Prefixes()
	if err != nil {
		t.Fatalf("localIPv4Prefixes: %v", err)
	}
	for _, p := range prefixes {
		if strings.HasPrefix(p, "127.") {
			t.Fatalf("expected loopback to be excluded, got %v", prefixes)
		}
	}
}
