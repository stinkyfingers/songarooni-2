// Package camera proxies a phone-based IP-camera app (e.g. GuardianEye)
// onto the venue display — see plans/http-video.md. The phone serves plain
// HTTP on the LAN (an MJPEG stream plus a single-frame snapshot), Basic
// Auth protected; this package holds the camera's configured address and
// credentials and makes every request to it itself, so the credentials
// never need to reach a browser.
package camera

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

// discoverTimeout bounds each candidate probe during Discover — short
// enough that scanning a whole /24 in parallel finishes in well under a
// second, long enough not to false-negative a phone on a loaded LAN.
const discoverTimeout = 3000 * time.Millisecond

// snapshotTimeout bounds a single Snapshot call (ShowLive's optional
// reachability check, not just Discover) — this sits in a user-facing
// request path, so it needs to fail fast rather than hang.
const snapshotTimeout = 2 * time.Second

// Proxy holds the configured camera's address and credentials. The zero
// value (via New with an empty ip) is "not configured" — Configured()
// reports that, and StreamMJPEG/Snapshot fail cleanly rather than making a
// request to nowhere.
type Proxy struct {
	mu       sync.RWMutex
	ip       string
	port     int
	username string
	password string

	client *http.Client // no blanket Timeout: StreamMJPEG is intentionally long-lived; Snapshot/Discover bound each call via context instead
}

// New builds a Proxy for the camera at ip:port, authenticating with
// username/password. ip may be empty (not yet configured) — Discover or a
// later SetTarget call fills it in.
func New(ip string, port int, username, password string) *Proxy {
	return &Proxy{
		ip:       ip,
		port:     port,
		username: username,
		password: password,
		client:   &http.Client{},
	}
}

// Target returns the currently configured ip/port — never credentials.
func (p *Proxy) Target() (ip string, port int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ip, p.port
}

// Configured reports whether a camera address has ever been set.
func (p *Proxy) Configured() bool {
	ip, _ := p.Target()
	return ip != ""
}

// SetTarget updates the camera's address at runtime (the control panel's
// "Save"/"Find Camera" flow) — in-memory only, not persisted to
// config.yaml (plans/http-video.md's open questions: no need to persist).
func (p *Proxy) SetTarget(ip string, port int) error {
	if ip != "" && port <= 0 {
		return fmt.Errorf("port must be positive")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ip = ip
	p.port = port
	return nil
}

func (p *Proxy) credentials() (ip string, port int, username, password string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ip, p.port, p.username, p.password
}

// StreamMJPEG proxies the camera's MJPEG stream through to w, authenticating
// with the configured credentials so they never need to reach the browser.
// The outgoing request is bound to r.Context(), so when the kiosk page
// tears down its <img> element (client disconnect), the upstream request
// to the phone is canceled too, rather than leaking an open stream to the
// camera indefinitely.
func (p *Proxy) StreamMJPEG(w http.ResponseWriter, r *http.Request) {
	ip, port, username, password := p.credentials()
	if ip == "" {
		http.Error(w, "no camera configured", http.StatusServiceUnavailable)
		return
	}

	url := fmt.Sprintf("http://%s:%d/mjpeg", ip, port)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.SetBasicAuth(username, password)

	resp, err := p.client.Do(req)
	if err != nil {
		http.Error(w, "camera unreachable: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "camera returned "+resp.Status, http.StatusBadGateway)
		return
	}

	// The multipart/x-mixed-replace boundary is what makes a plain <img>
	// tag render this as live video — it must pass through unchanged.
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(flushWriter{w}, resp.Body)
}

// flushWriter flushes after every Write — without it, frames can sit
// buffered instead of reaching the browser as the camera sends them,
// turning "live" video into a stuttering one.
type flushWriter struct {
	w http.ResponseWriter
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if f, ok := fw.w.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

// Snapshot fetches a single still frame from the configured camera — a
// cheap reachability check compared to opening a whole MJPEG stream just
// to test the connection.
func (p *Proxy) Snapshot(ctx context.Context) ([]byte, error) {
	ip, port, username, password := p.credentials()
	if ip == "" {
		return nil, fmt.Errorf("no camera configured")
	}
	ctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()
	body, status, err := p.probeSnapshot(ctx, ip, port, username, password)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("camera returned status %d", status)
	}
	return body, nil
}

// probeSnapshot is the raw "ask this exact host" primitive both Snapshot
// and Discover build on — Discover needs to vary the IP across candidates
// while keeping the configured port/credentials fixed, which Snapshot's
// own signature (always the configured target) can't express.
func (p *Proxy) probeSnapshot(ctx context.Context, ip string, port int, username, password string) ([]byte, int, error) {
	url := fmt.Sprintf("http://%s:%d/snapshot.jpg", ip, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(username, password)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // a snapshot is a few hundred KB at most; cap defensively
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// Discover scans the Pi's own /24 for a host answering /snapshot.jpg with
// the configured credentials — "a means to determine the camera IP" from
// plans/http-video.md, rather than requiring the IP already be known. Not
// a general IP-camera discovery protocol (no ONVIF) — just a probe for
// this one app's known URL shape, which is all that's needed since
// there's exactly one kind of camera this app talks to.
func (p *Proxy) Discover(ctx context.Context) ([]string, error) {
	_, port, username, password := p.credentials()
	if port <= 0 {
		return nil, fmt.Errorf("camera port not configured")
	}

	prefixes, err := localIPv4Prefixes()
	if err != nil {
		return nil, err
	}
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("no local IPv4 network found to scan")
	}

	// Non-nil even with zero matches: encoding/json marshals a nil slice
	// as `null`, not `[]`, which breaks the control panel's
	// `const { found } = await api(...); found.length` on an empty result.
	all := []string{}
	for _, prefix := range prefixes {
		all = append(all, p.scanPrefix(ctx, prefix, port, username, password)...)
	}
	sort.Strings(all)
	return all, nil
}

// scanPrefix probes every host (1-254) under prefix (an "a.b.c" /24) in
// parallel, returning whichever answered /snapshot.jpg with a 200 and a
// plausible JPEG body. Split out from Discover so it can be exercised
// directly against a loopback-bound test server — Discover itself filters
// loopback/link-local prefixes out via localIPv4Prefixes, which a test
// can't redirect, but scanPrefix takes the prefix as a plain argument.
func (p *Proxy) scanPrefix(ctx context.Context, prefix string, port int, username, password string) []string {
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		found []string
	)
	for host := 1; host <= 254; host++ {
		ip := fmt.Sprintf("%s.%d", prefix, host)
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			reqCtx, cancel := context.WithTimeout(ctx, discoverTimeout)
			defer cancel()
			body, status, err := p.probeSnapshot(reqCtx, ip, port, username, password)
			if err != nil || status != http.StatusOK || !looksLikeImage(body) {
				return
			}
			mu.Lock()
			found = append(found, ip)
			mu.Unlock()
		}(ip)
	}
	wg.Wait()
	return found
}

// looksLikeImage is a cheap plausibility check for Discover's "200 OK, but
// is it actually a camera" question — a JPEG always starts with this
// 2-byte magic number.
func looksLikeImage(body []byte) bool {
	return len(body) >= 2 && body[0] == 0xFF && body[1] == 0xD8
}

// localIPv4Prefixes returns each non-loopback, non-link-local IPv4
// address currently assigned to this machine, as an "a.b.c" /24 prefix
// (the last octet stripped) — Discover's scan range. Best-effort: returns
// an empty slice rather than erroring if interfaces can't be enumerated,
// same spirit as the WebRTC plan's now-removed localNonLoopbackIPs().
func localIPv4Prefixes() ([]string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var prefixes []string
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip4 := ipNet.IP.To4()
		if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
			continue
		}
		prefix := fmt.Sprintf("%d.%d.%d", ip4[0], ip4[1], ip4[2])
		if !seen[prefix] {
			seen[prefix] = true
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes, nil
}
