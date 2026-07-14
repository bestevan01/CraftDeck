package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"
)

// selfSignedCache holds the one self-signed certificate this process ever
// generates -- FR-33's fallback for whenever the operator has WAN exposure
// on but no owned main domain (and thus nothing for Let's Encrypt to issue
// against). Generated fresh in memory on first use and kept only for this
// process's lifetime: it's just there so traffic isn't served completely
// unencrypted, not something operators are expected to trust/pin, so there's
// no reason to persist it to disk across restarts.
var (
	selfSignedOnce sync.Once
	selfSignedCert *tls.Certificate
	selfSignedErr  error
)

// GetSelfSigned returns the process-lifetime self-signed certificate,
// generating it on first call.
func GetSelfSigned() (*tls.Certificate, error) {
	selfSignedOnce.Do(func() {
		selfSignedCert, selfSignedErr = generateSelfSigned()
	})
	return selfSignedCert, selfSignedErr
}

func generateSelfSigned() (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "CraftDeck (self-signed)"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,
		// IP SANs cover "connect by public/local IP with no domain" --
		// the exact scenario this fallback exists for. DNSNames is left
		// empty (browsers only warn about an untrusted cert either way,
		// not a hostname mismatch, since operators reach this over
		// whatever address happens to route -- there's no fixed hostname
		// to pin it to).
		IPAddresses: []net.IP{net.IPv4zero, net.IPv6zero},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}
	cert := &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	return cert, nil
}
