// Package identity issues and holds ephemeral per-session SSH identities. Private
// keys exist ONLY in process memory (this vault) and are zeroized on cleanup —
// never written to disk, database, cookies, or caches.
package identity

import (
	"crypto/ed25519"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

// Credential is a live ephemeral SSH identity bound to a browser session.
type Credential struct {
	SessionID  uuid.UUID
	UserID     uuid.UUID
	Username   string
	Serial     uint64
	Principals []string
	ExpiresAt  time.Time

	privateKey ed25519.PrivateKey // zeroized on Destroy
	cert       *ssh.Certificate
	certSigner ssh.Signer // signer presenting the certificate to hosts
}

// CertSigner returns the ssh.Signer that authenticates with the certificate.
func (c *Credential) CertSigner() ssh.Signer { return c.certSigner }

// Certificate returns the issued certificate.
func (c *Credential) Certificate() *ssh.Certificate { return c.cert }

// Vault stores live credentials keyed by session id.
type Vault struct {
	mu    sync.RWMutex
	creds map[uuid.UUID]*Credential
}

// NewVault constructs an empty Vault.
func NewVault() *Vault { return &Vault{creds: make(map[uuid.UUID]*Credential)} }

func (v *Vault) put(c *Credential) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if existing, ok := v.creds[c.SessionID]; ok {
		existing.zero()
	}
	v.creds[c.SessionID] = c
}

// Get returns the live credential for a session, if present and unexpired.
func (v *Vault) Get(sessionID uuid.UUID) (*Credential, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	c, ok := v.creds[sessionID]
	if !ok {
		return nil, false
	}
	return c, true
}

// Destroy zeroizes and removes a session's credential (logout/idle/cleanup).
func (v *Vault) Destroy(sessionID uuid.UUID) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	c, ok := v.creds[sessionID]
	if !ok {
		return false
	}
	c.zero()
	delete(v.creds, sessionID)
	return true
}

// Sessions returns the session ids that currently hold credentials.
func (v *Vault) Sessions() []uuid.UUID {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]uuid.UUID, 0, len(v.creds))
	for id := range v.creds {
		out = append(out, id)
	}
	return out
}

// zero overwrites private key bytes so they cannot be recovered from memory.
func (c *Credential) zero() {
	for i := range c.privateKey {
		c.privateKey[i] = 0
	}
	c.privateKey = nil
	c.certSigner = nil
}
