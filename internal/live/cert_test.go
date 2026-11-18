package live

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

// generateSelfSignedCert is tested directly (it takes explicit paths)
// rather than certPaths(), which resolves os.UserCacheDir() — on Darwin
// that's always $HOME/Library/Caches with no env var to redirect it in a
// test, so exercising it here would write into the real user's cache
// directory. kioskProfileDir() (internal/slideshow), the existing
// precedent for this exact pattern, has no dedicated test either.
func TestGenerateSelfSignedCertProducesAValidPair(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	if err := generateSelfSignedCert(certFile, keyFile); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	// The actual thing ListenAndServeTLS needs: the key must actually
	// match the cert, and both must parse as a well-formed pair.
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		t.Fatalf("cert/key don't form a valid pair: %v", err)
	}

	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("expected a CERTIFICATE PEM block, got %+v", block)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parsing certificate: %v", err)
	}

	wantDNS := map[string]bool{"songarooni.local": true, "localhost": true}
	if len(cert.DNSNames) != len(wantDNS) {
		t.Fatalf("expected DNSNames %v, got %v", wantDNS, cert.DNSNames)
	}
	for _, name := range cert.DNSNames {
		if !wantDNS[name] {
			t.Fatalf("unexpected DNS name %q in cert, got %v", name, cert.DNSNames)
		}
	}

	// Confirmed necessary in practice, not just in theory: a cert with no
	// IP SAN hits a stricter, not-reliably-bypassable failure mode on iOS
	// when a phone connects by raw IP instead of hostname. This machine's
	// actual non-loopback, non-link-local IPs must be present — calling
	// the real function under test here (same package), not a duplicate,
	// so this can't silently drift from what generateSelfSignedCert
	// actually does.
	wantIPs := localNonLoopbackIPs()
	if len(wantIPs) == 0 {
		t.Skip("no non-loopback IPs on this machine to compare against")
	}
	for _, want := range wantIPs {
		found := false
		for _, got := range cert.IPAddresses {
			if got.Equal(want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected IP %v in cert.IPAddresses, got %v", want, cert.IPAddresses)
		}
	}
}

func TestGenerateSelfSignedCertKeyFilePermissions(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	if err := generateSelfSignedCert(certFile, keyFile); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	info, err := os.Stat(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0077 != 0 {
		t.Fatalf("expected the private key to be unreadable by group/other, got mode %v", perm)
	}
}
