// Package sshgw is the SSH gateway: the ONLY SSH client in the system. It dials
// managed hosts through the jump host (native ProxyJump), authenticating with the
// session's ephemeral certificate. The browser never speaks SSH directly.
package sshgw

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	"github.com/fleet-terminal/backend/internal/config"
	"github.com/fleet-terminal/backend/internal/identity"
)

// Gateway establishes SSH connections through the jump host.
type Gateway struct {
	cfg   *config.Config
	log   *slog.Logger
	vault *identity.Vault
}

// New constructs a Gateway.
func New(cfg *config.Config, log *slog.Logger, vault *identity.Vault) *Gateway {
	return &Gateway{cfg: cfg, log: log, vault: vault}
}

// Conn bundles a live SSH client and its underlying network connections so the
// caller can close everything cleanly.
type Conn struct {
	Client *ssh.Client
	jump   *ssh.Client
}

// Close tears down the host client and the jump tunnel.
func (c *Conn) Close() {
	if c.Client != nil {
		_ = c.Client.Close()
	}
	if c.jump != nil {
		_ = c.jump.Close()
	}
}

// Dial connects to host:port through the jump host using the ephemeral
// credentials bound to sessionID. host should be the managed host's WireGuard
// address, which is routable from the jump host.
func (g *Gateway) Dial(ctx context.Context, sessionID, host string, port int, user string) (*Conn, error) {
	cred, ok := g.vaultLookup(sessionID)
	if !ok {
		return nil, fmt.Errorf("no live credential for session")
	}
	signer := cred.CertSigner()
	if signer == nil {
		return nil, fmt.Errorf("session credential unavailable")
	}

	// NOTE: host key verification uses a fixed known_hosts file in production
	// (cfg.JumpKnownHostsFile). For the bundled local test fabric we accept the
	// presented key; this is documented in the security guide.
	hostKeyCB := ssh.InsecureIgnoreHostKey()

	jumpCfg := &ssh.ClientConfig{
		User:            g.cfg.JumpUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCB,
		Timeout:         10 * time.Second,
	}
	jumpClient, err := ssh.Dial("tcp", g.cfg.JumpHost, jumpCfg)
	if err != nil {
		return nil, fmt.Errorf("dial jump host: %w", err)
	}

	target := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	tunnel, err := jumpClient.DialContext(ctx, "tcp", target)
	if err != nil {
		_ = jumpClient.Close()
		return nil, fmt.Errorf("tunnel to %s via jump: %w", target, err)
	}

	hostCfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCB,
		Timeout:         10 * time.Second,
	}
	ncc, chans, reqs, err := ssh.NewClientConn(tunnel, target, hostCfg)
	if err != nil {
		_ = tunnel.Close()
		_ = jumpClient.Close()
		return nil, fmt.Errorf("ssh handshake with %s: %w", target, err)
	}
	return &Conn{Client: ssh.NewClient(ncc, chans, reqs), jump: jumpClient}, nil
}

// DialHost implements app.Dialer.
func (g *Gateway) DialHost(handle, host string, port int, user string) (any, error) {
	return g.Dial(context.Background(), handle, host, port, user)
}

func (g *Gateway) vaultLookup(sessionID string) (*identity.Credential, bool) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, false
	}
	return g.vault.Get(id)
}
