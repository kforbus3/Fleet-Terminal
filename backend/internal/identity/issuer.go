package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	"github.com/fleet-terminal/backend/internal/ca"
	"github.com/fleet-terminal/backend/internal/config"
	"github.com/fleet-terminal/backend/internal/metrics"
	"github.com/fleet-terminal/backend/internal/models"
	"github.com/fleet-terminal/backend/internal/store"
)

// Issuer mints ephemeral identities, persists certificate metadata, and keeps
// the live private keys in the Vault.
type Issuer struct {
	store *store.Store
	ca    *ca.CA
	cfg   *config.Config
	log   *slog.Logger
	vault *Vault
}

// NewIssuer constructs an Issuer.
func NewIssuer(st *store.Store, c *ca.CA, cfg *config.Config, log *slog.Logger, vault *Vault) *Issuer {
	return &Issuer{store: st, ca: c, cfg: cfg, log: log, vault: vault}
}

// Vault exposes the live credential store.
func (i *Issuer) Vault() *Vault { return i.vault }

// Issue generates a new Ed25519 keypair in memory, signs a user certificate, and
// records its metadata. The private key is retained only in the Vault.
func (i *Issuer) Issue(ctx context.Context, sessionID, userID uuid.UUID, username string, principals []string) (*Credential, error) {
	if len(principals) == 0 {
		principals = []string{username}
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
	auditID := uuid.New()
	keyID := fmt.Sprintf("%s/%s/%d", username, sessionID.String()[:8], serial)

	cert, err := i.ca.SignUserCertificate(sshPub, keyID, principals, serial, i.cfg.UserCertTTL)
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

	caID, _ := uuid.Parse(i.ca.ActiveID())
	expiresAt := time.Unix(int64(cert.ValidBefore), 0)
	if _, err := i.store.InsertCertificate(ctx, store.InsertCertificateParams{
		Serial: serial, Kind: "user", CAKeyID: caID, UserID: &userID, SessionID: &sessionID,
		KeyID: keyID, Principals: principals,
		PublicKey: string(ssh.MarshalAuthorizedKey(sshPub)), AuditID: auditID, ExpiresAt: expiresAt,
	}); err != nil {
		return nil, err
	}

	cred := &Credential{
		SessionID: sessionID, UserID: userID, Username: username, Serial: serial,
		Principals: principals, ExpiresAt: expiresAt,
		privateKey: priv, cert: cert, certSigner: certSigner,
	}
	i.vault.put(cred)
	metrics.CertificatesIssued.WithLabelValues("user").Inc()

	_, _ = i.store.AppendAudit(ctx, models.AuditEvent{
		ActorID: &userID, ActorName: username, Action: "certificate.issue",
		TargetKind: "certificate", TargetID: fmt.Sprintf("%d", serial),
		Detail: map[string]any{"principals": principals, "expiresAt": expiresAt, "auditId": auditID},
	})
	return cred, nil
}

// IssueForSession implements app.CAIssuer.
func (i *Issuer) IssueForSession(sessionID, userID, username string, principals []string) (string, error) {
	sid, err := uuid.Parse(sessionID)
	if err != nil {
		return "", err
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		return "", err
	}
	if _, err := i.Issue(context.Background(), sid, uid, username, principals); err != nil {
		return "", err
	}
	return sessionID, nil
}

// DestroySession zeroizes a session's live key and revokes its certificates.
func (i *Issuer) DestroySession(ctx context.Context, sessionID uuid.UUID) {
	i.vault.Destroy(sessionID)
	if n, err := i.store.RevokeSessionCertificates(ctx, sessionID, "session_ended"); err != nil {
		i.log.Warn("revoke session certs", "err", err)
	} else if n > 0 {
		i.log.Info("revoked session certificates", "session", sessionID, "count", n)
	}
}

// RenewExpiring re-issues certificates that will expire within the renewal window
// for sessions still holding live credentials, without disrupting active use.
func (i *Issuer) RenewExpiring(ctx context.Context) {
	cutoff := time.Now().Add(i.cfg.CertRenewBefore)
	expiring, err := i.store.ExpiringCertificates(ctx, cutoff)
	if err != nil {
		i.log.Warn("scan expiring certs", "err", err)
		return
	}
	for _, c := range expiring {
		if c.SessionID == nil {
			continue
		}
		if _, live := i.vault.Get(*c.SessionID); !live {
			continue // session gone; nothing to renew
		}
		if c.UserID == nil {
			continue
		}
		username := ""
		if u, err := i.store.GetUserByID(ctx, *c.UserID); err == nil {
			username = u.Username
		}
		if _, err := i.Issue(ctx, *c.SessionID, *c.UserID, username, c.Principals); err != nil {
			i.log.Warn("renew cert", "session", c.SessionID, "err", err)
			continue
		}
		// Revoke the superseded certificate.
		_ = i.store.RevokeCertificate(ctx, c.Serial, "renewed")
		i.log.Info("renewed ephemeral certificate", "session", c.SessionID, "oldSerial", c.Serial)
	}
}
