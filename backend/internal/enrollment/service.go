// Package enrollment automates onboarding a managed host: it provisions the
// WireGuard tunnel (peer on the jump host + interface on the managed host),
// brings the interface up, collects host facts, and records the result. The
// host's WireGuard private key is generated on the host and never leaves it.
package enrollment

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/fleet-terminal/backend/internal/config"
	"github.com/fleet-terminal/backend/internal/models"
	"github.com/fleet-terminal/backend/internal/sshgw"
	"github.com/fleet-terminal/backend/internal/store"

	"log/slog"

	"github.com/google/uuid"
)

// Service performs host enrollment over SSH.
type Service struct {
	store *store.Store
	cfg   *config.Config
	log   *slog.Logger
	gw    *sshgw.Gateway
}

// New constructs the enrollment Service.
func New(st *store.Store, cfg *config.Config, log *slog.Logger, gw *sshgw.Gateway) *Service {
	return &Service{store: st, cfg: cfg, log: log, gw: gw}
}

// Result summarizes an enrollment run.
type Result struct {
	Job     *models.EnrollmentJob `json:"job"`
	WGAddr  string                `json:"wgAddress"`
	HostPub string                `json:"hostPublicKey"`
}

// Enroll provisions WireGuard + trust for a host using the caller's session
// credentials. It is idempotent: re-running re-applies configuration.
func (s *Service) Enroll(ctx context.Context, sessionID uuid.UUID, host *models.Host, actor *uuid.UUID) (*Result, error) {
	mgmtAddr := host.Address
	if mgmtAddr == "" {
		mgmtAddr = host.Hostname // fall back to a resolvable name
	}
	job, err := s.store.CreateEnrollmentJob(ctx, host.ID, fmt.Sprintf("%s:%d", mgmtAddr, host.SSHPort), "", actor)
	if err != nil {
		return nil, err
	}
	step := func(name, status, detail string) {
		_ = s.store.AppendEnrollmentStep(ctx, job.ID, models.EnrollmentStep{
			Name: name, Status: status, Detail: detail, Timestamp: time.Now(),
		})
	}
	fail := func(name string, err error) (*Result, error) {
		step(name, "failed", err.Error())
		_ = s.store.FinishEnrollmentJob(ctx, job.ID, "failed", err.Error())
		_, _ = s.store.AppendAudit(ctx, models.AuditEvent{
			ActorID: actor, Action: "host.enroll_failed", TargetKind: "host", TargetID: host.ID.String(),
			Detail: map[string]any{"step": name, "error": err.Error()},
		})
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	// 1) Reach the jump host directly and read its WireGuard public key.
	jumpAddr, jumpPort := splitHostPort(s.cfg.JumpHost, 22)
	jumpClient, err := s.gw.DialDirect(ctx, sessionID.String(), jumpAddr, jumpPort, s.cfg.JumpUser)
	if err != nil {
		return fail("connect_jump_host", err)
	}
	defer jumpClient.Close()
	jumpPub, err := run(jumpClient, "sudo cat /etc/wireguard/publickey 2>/dev/null || cat /etc/wireguard/publickey")
	if err != nil || strings.TrimSpace(jumpPub) == "" {
		return fail("read_jump_public_key", orErr(err, "jump host has no WireGuard public key"))
	}
	jumpPub = strings.TrimSpace(jumpPub)
	step("connect_jump_host", "ok", "jump WG pubkey "+short(jumpPub))

	// 2) Reach the managed host directly (it is not on the overlay yet).
	hostClient, err := s.gw.DialDirect(ctx, sessionID.String(), mgmtAddr, host.SSHPort, host.SSHUser)
	if err != nil {
		return fail("connect_host", err)
	}
	defer hostClient.Close()
	step("connect_host", "ok", "ssh certificate auth to "+mgmtAddr)

	// 3) Collect host facts.
	if facts, ferr := run(hostClient, "uname -s; uname -r; uname -m; (. /etc/os-release 2>/dev/null; echo \"$NAME $VERSION_ID\"); ssh -V 2>&1 | head -1"); ferr == nil {
		s.recordFacts(ctx, host.ID, facts)
		step("collect_facts", "ok", oneLine(facts))
	} else {
		step("collect_facts", "skipped", ferr.Error())
	}

	// 4) Determine the overlay address. If the operator specified one, validate
	//    it; otherwise auto-assign the lowest free address from the pool.
	wgIP := strings.TrimSpace(host.WGAddress)
	if wgIP != "" {
		if !isOverlayAddr(wgIP, s.cfg.WGJumpIP) {
			return fail("assign_overlay_address",
				fmt.Errorf("WireGuard address %q is not in the overlay subnet %s", wgIP, s.cfg.WGSubnet))
		}
		if inUse, _ := s.store.WGAddressInUse(ctx, wgIP, host.ID); inUse {
			return fail("assign_overlay_address",
				fmt.Errorf("WireGuard address %s is already assigned to another host", wgIP))
		}
	} else {
		wgIP, err = s.store.NextFreeWGAddress(ctx, s.cfg.WGJumpIP)
		if err != nil {
			return fail("assign_overlay_address", err)
		}
	}
	hostScript := s.hostWGScript(wgIP, jumpPub)
	out, err := run(hostClient, "sudo sh -c "+shellQuote(hostScript))
	if err != nil {
		return fail("configure_host_wireguard", orErr(err, out))
	}
	hostPub := parseKV(out, "HOSTPUB")
	if hostPub == "" {
		return fail("configure_host_wireguard", fmt.Errorf("host public key not produced: %s", oneLine(out)))
	}
	step("configure_host_wireguard", "ok", fmt.Sprintf("wg0=%s pub=%s", wgIP, short(hostPub)))

	// 5) Add the host as a peer on the jump host (the VPN server).
	hostEndpoint := fmt.Sprintf("%s:%d", mgmtAddr, s.cfg.WGPort)
	jumpScript := s.jumpPeerScript(host.Hostname, hostPub, hostEndpoint, wgIP)
	if jout, jerr := run(jumpClient, "sudo sh -c "+shellQuote(jumpScript)); jerr != nil {
		return fail("configure_jump_peer", orErr(jerr, jout))
	}
	step("configure_jump_peer", "ok", fmt.Sprintf("peer %s allowed-ips %s/32", short(hostPub), wgIP))

	// 6) Verify the tunnel established a handshake.
	hs := s.waitHandshake(hostClient)
	if hs {
		step("verify_tunnel", "ok", "wireguard handshake established")
	} else {
		// Non-fatal: handshake may lag, or (in the local userspace fabric on
		// macOS) the data plane is limited. The tunnel is configured either way.
		step("verify_tunnel", "skipped", "no handshake observed yet (configuration applied)")
	}

	// 7) Persist results.
	_ = s.store.SetHostWGAddress(ctx, host.ID, wgIP)
	_ = s.store.SetHostEnrolled(ctx, host.ID, true)
	_ = s.store.FinishEnrollmentJob(ctx, job.ID, "succeeded", "")
	_, _ = s.store.AppendAudit(ctx, models.AuditEvent{
		ActorID: actor, Action: "host.enroll", TargetKind: "host", TargetID: host.ID.String(),
		Detail: map[string]any{"wgAddress": wgIP, "hostPublicKey": hostPub, "jobId": job.ID},
	})

	final, _ := s.store.GetEnrollmentJob(ctx, job.ID)
	return &Result{Job: final, WGAddr: wgIP, HostPub: hostPub}, nil
}

// hostWGScript renders the script that configures WireGuard on the managed host.
// The private key is generated on the host and never transmitted.
func (s *Service) hostWGScript(wgIP, jumpPub string) string {
	iface := s.cfg.WGInterface
	return fmt.Sprintf(`set -e
IF=%s; IP=%s; SUBNET=%s; JPUB='%s'; JEP=%s; PORT=%d
mkdir -p /etc/wireguard; umask 077
[ -f /etc/wireguard/$IF.key ] || wg genkey > /etc/wireguard/$IF.key
PRIV=$(cat /etc/wireguard/$IF.key)
PUB=$(printf '%%s' "$PRIV" | wg pubkey)
if ! ip link show $IF >/dev/null 2>&1; then wireguard-go $IF; sleep 1; fi
printf '%%s' "$PRIV" | wg set $IF private-key /dev/stdin listen-port $PORT
wg set $IF peer "$JPUB" endpoint "$JEP" allowed-ips $SUBNET persistent-keepalive 25
ip addr flush dev $IF 2>/dev/null || true
ip addr add $IP/24 dev $IF
ip link set $IF up
cat > /etc/wireguard/$IF.conf <<EOF
[Interface]
Address = $IP/24
PrivateKey = $PRIV
ListenPort = $PORT
[Peer]
PublicKey = $JPUB
Endpoint = $JEP
AllowedIPs = $SUBNET
PersistentKeepalive = 25
EOF
echo "HOSTPUB=$PUB"`,
		iface, wgIP, s.cfg.WGSubnet, jumpPub, s.cfg.WGJumpEndpoint, s.cfg.WGPort)
}

// jumpPeerScript renders the script that adds the host as a peer on the jump host.
func (s *Service) jumpPeerScript(hostname, hostPub, hostEndpoint, wgIP string) string {
	iface := s.cfg.WGInterface
	return fmt.Sprintf(`set -e
IF=%s
wg set $IF peer '%s' endpoint '%s' allowed-ips %s/32 persistent-keepalive 25
mkdir -p /etc/wireguard/peers
cat > /etc/wireguard/peers/%s.conf <<EOF
[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = %s/32
EOF
echo OK`,
		iface, hostPub, hostEndpoint, wgIP, sanitize(hostname), hostPub, hostEndpoint, wgIP)
}

func (s *Service) waitHandshake(client *ssh.Client) bool {
	// Poke the tunnel so WireGuard initiates a handshake immediately rather than
	// waiting for the persistent-keepalive interval.
	_, _ = run(client, fmt.Sprintf("ping -c1 -W1 %s >/dev/null 2>&1 || true", s.cfg.WGJumpIP))
	// WireGuard initiates the first handshake on its persistent-keepalive timer
	// (~25s), so poll comfortably past that.
	for i := 0; i < 16; i++ {
		out, err := run(client, "wg show "+s.cfg.WGInterface+" latest-handshakes 2>/dev/null | awk '{print $2}' | sort -rn | head -1")
		if err == nil {
			if v := strings.TrimSpace(out); v != "" && v != "0" {
				return true
			}
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

func (s *Service) recordFacts(ctx context.Context, hostID uuid.UUID, facts string) {
	lines := strings.Split(strings.TrimSpace(facts), "\n")
	inv := models.HostInventory{}
	if len(lines) > 0 {
		inv.OSName = strings.TrimSpace(lines[0])
	}
	if len(lines) > 1 {
		inv.KernelVersion = strings.TrimSpace(lines[1])
	}
	if len(lines) > 2 {
		inv.Architecture = strings.TrimSpace(lines[2])
	}
	if len(lines) > 3 && strings.TrimSpace(lines[3]) != "" {
		inv.OSName = strings.TrimSpace(lines[3])
	}
	if len(lines) > 4 {
		inv.SSHVersion = strings.TrimSpace(lines[4])
	}
	_ = s.store.UpsertInventory(ctx, hostID, inv)
}

// --- small helpers ---

func run(client *ssh.Client, cmd string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out, err := sess.CombinedOutput(cmd)
	return string(out), err
}

func splitHostPort(hp string, def int) (string, int) {
	host, port, err := net.SplitHostPort(hp)
	if err != nil {
		return hp, def
	}
	p := def
	fmt.Sscanf(port, "%d", &p)
	return host, p
}

// isOverlayAddr reports whether addr is a usable overlay address in the same /24
// as the jump host (and not the jump host itself).
func isOverlayAddr(addr, jumpIP string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" || addr == jumpIP {
		return false
	}
	return ipPrefix24(addr) == ipPrefix24(jumpIP)
}

func ipPrefix24(ip string) string {
	parts := strings.Split(strings.TrimSpace(ip), ".")
	if len(parts) != 4 {
		return ""
	}
	return strings.Join(parts[:3], ".")
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
}

func parseKV(out, key string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"=")
		}
	}
	return ""
}

func short(k string) string {
	k = strings.TrimSpace(k)
	if len(k) > 12 {
		return k[:12] + "…"
	}
	return k
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

func orErr(err error, msg string) error {
	if err != nil {
		return fmt.Errorf("%v: %s", err, oneLine(msg))
	}
	return fmt.Errorf("%s", oneLine(msg))
}
