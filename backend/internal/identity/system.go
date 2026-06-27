package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// systemHolder caches a service certificate used by background workers (the
// monitor) that act without a user session. It is minted from the same CA with
// a short principal set and refreshed before expiry. The private key lives only
// in memory.
type systemHolder struct {
	mu        sync.Mutex
	signer    ssh.Signer
	expiresAt time.Time
}

var systemCache = &systemHolder{}

// SystemSigner returns a certificate-backed signer for the given principals,
// minting (and caching) one as needed. ttl bounds the certificate lifetime.
func (i *Issuer) SystemSigner(ctx context.Context, principals []string, ttl time.Duration) (ssh.Signer, error) {
	systemCache.mu.Lock()
	defer systemCache.mu.Unlock()
	if systemCache.signer != nil && time.Until(systemCache.expiresAt) > 5*time.Minute {
		return systemCache.signer, nil
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, err
	}
	serial, err := i.store.NextCertSerial(ctx)
	if err != nil {
		return nil, err
	}
	keyID := fmt.Sprintf("system/monitor/%d", serial)
	cert, err := i.ca.SignUserCertificate(sshPub, keyID, principals, serial, ttl)
	if err != nil {
		return nil, err
	}
	keySigner, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, err
	}
	certSigner, err := ssh.NewCertSigner(cert, keySigner)
	if err != nil {
		return nil, err
	}
	systemCache.signer = certSigner
	systemCache.expiresAt = time.Unix(int64(cert.ValidBefore), 0)
	return certSigner, nil
}
