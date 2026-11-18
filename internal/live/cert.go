package live

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// certPaths returns where this Pi's self-signed TLS cert/key live,
// generating and caching them on first use — same spirit as
// slideshow's kioskProfileDir(): a restart must reuse the same files,
// not regenerate them, since every phone that's ever going to broadcast
// has to manually trust this specific cert once in Safari. A fresh cert
// on every boot would mean re-trusting it every time too.
func certPaths() (certFile, keyFile string, err error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "songarooni", "live-tls")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}

	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			return certFile, keyFile, nil // already generated on a previous run
		}
	}
	if err := generateSelfSignedCert(certFile, keyFile); err != nil {
		return "", "", fmt.Errorf("generating self-signed cert: %w", err)
	}
	return certFile, keyFile, nil
}

// generateSelfSignedCert writes a new RSA key + self-signed certificate
// to certFile/keyFile, entirely with the standard library — no openssl
// dependency, matching how this project already avoids external tool
// dependencies elsewhere (see the MP4 duration parser).
//
// Covers "songarooni.local" and "localhost" (how this app's own network
// setup expects to be reached — see README's "Network setup") plus
// whatever non-loopback LAN IPs this machine has right now. The IPs
// matter in practice, not just in theory: a cert with no IP SAN at all
// hits a *stricter* failure mode than plain self-signed when a phone
// connects by raw IP instead of hostname — confirmed on iOS, where the
// usual "visit this website anyway" bypass didn't fully take, unlike the
// clean single warning a correctly-SAN'd cert produces. If the Pi's IP
// ever changes, the cached cert won't have the new one until it's
// regenerated (delete the cached cert/key to force that) — hostname
// access is unaffected either way, since DNSNames doesn't depend on it.
func generateSelfSignedCert(certFile, keyFile string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "songarooni.local"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0), // a kiosk appliance's cert, not rotated
		// A plain self-signed server (leaf) certificate, not a CA — it's
		// self-*signed* (issuer == subject, mechanically, via the
		// CreateCertificate call below) without claiming CA status.
		// IsCA/KeyUsageCertSign were set on an earlier version of this;
		// that's a non-standard profile for a cert presented directly
		// for TLS, and plausibly gets stricter/less-forgiving validation
		// on some platforms even after a user accepts the self-signed
		// warning. Every standard self-signed-cert guide generates a
		// leaf cert, not a CA cert, for exactly this reason.
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              []string{"songarooni.local", "localhost"},
		IPAddresses:           localNonLoopbackIPs(),
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return err
	}

	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return err
	}

	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	return pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

// localNonLoopbackIPs returns every non-loopback, non-link-local IP
// currently assigned to this machine, for the cert's IPAddresses SAN
// list. Link-local addresses (IPv4 169.254.0.0/16, IPv6 fe80::/10) are
// excluded deliberately — they're scoped per-interface/per-link, not
// globally meaningful the way a real LAN address is, and including them
// doesn't help a client connect (the server also only listens on IPv4 —
// see Run — specifically because an advertised IPv6 link-local address
// turned out to cause real client-side connectivity trouble).
// Best-effort: if it can't enumerate interfaces at all, it just returns
// nil rather than failing cert generation over it — the DNSNames-based
// hostname access still works regardless.
func localNonLoopbackIPs() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var ips []net.IP
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() || ipNet.IP.IsLinkLocalUnicast() {
			continue
		}
		ips = append(ips, ipNet.IP)
	}
	return ips
}
