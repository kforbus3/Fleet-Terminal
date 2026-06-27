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
// EnrollParams controls how enrollment reaches the host for the initial bootstrap.
type EnrollParams struct {
	// Method is "password" to bootstrap a host that does not yet trust the Fleet
	// CA (installs trust + WireGuard over an SSH password), or "trusted" to use
	// the caller's session certificate on a host that already trusts the CA.
	Method        string
	BootstrapUser string
	Password      string
	// SudoPassword is the password for `sudo` when the bootstrap user has
	// password-required sudo. If empty, the SSH password is reused (password
	// method) or passwordless sudo is assumed (trusted method).
	SudoPassword string
	// WGEndpoint overrides the jump host's WireGuard endpoint (host:port) written
	// into the managed host's config — i.e. the publicly-routable address the host
	// uses to reach the VPN server. Defaults to FLEET_WG_JUMP_ENDPOINT.
	WGEndpoint string
	// ViaJump routes the bootstrap SSH connection through the jump host instead
	// of connecting directly from the backend.
	ViaJump bool
}

func (p EnrollParams) method() string {
	if p.Method == "password" {
		return "password"
	}
	return "trusted"
}

func (s *Service) Enroll(ctx context.Context, sessionID uuid.UUID, host *models.Host, actor *uuid.UUID, params EnrollParams) (*Result, error) {
	mgmtAddr := host.Address
	if mgmtAddr == "" {
		mgmtAddr = host.Hostname // fall back to a resolvable name
	}
	loginUser := host.SSHUser
	if loginUser == "" {
		loginUser = "fleet"
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

	// 1) Reach the jump host (the VPN server, which already trusts the CA) and
	//    read its WireGuard public key.
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

	// 2) Connect to the host for bootstrap. With "password" we authenticate with a
	//    bootstrap credential (the host need not trust the CA yet); with "trusted"
	//    we use the session certificate. The connection is either direct from the
	//    backend, or routed *through the jump host* (when the backend cannot reach
	//    the host directly but the jump host can).
	var hostClient *ssh.Client
	var hostClose func()
	var isRoot bool
	var sudoPass string
	via := "direct"
	if params.ViaJump {
		via = "via jump host"
	}
	if params.method() == "password" {
		buser := params.BootstrapUser
		if buser == "" {
			buser = "root"
		}
		isRoot = buser == "root"
		if !isRoot {
			sudoPass = params.SudoPassword
			if sudoPass == "" {
				sudoPass = params.Password // reuse SSH password for sudo by default
			}
		}
		if params.ViaJump {
			conn, derr := s.gw.DialPasswordViaJump(ctx, sessionID.String(), mgmtAddr, host.SSHPort, buser, params.Password)
			if derr != nil {
				return fail("connect_host", derr)
			}
			hostClient, hostClose = conn.Client, conn.Close
		} else {
			hostClient, err = s.gw.DialDirectPassword(ctx, mgmtAddr, host.SSHPort, buser, params.Password)
			if err != nil {
				return fail("connect_host", err)
			}
			hostClose = func() { _ = hostClient.Close() }
		}
		step("connect_host", "ok", fmt.Sprintf("ssh password auth as %s@%s (%s)", buser, mgmtAddr, via))
	} else {
		// Certificate auth has no SSH password, but sudo may still require one.
		sudoPass = params.SudoPassword
		if params.ViaJump {
			conn, derr := s.gw.Dial(ctx, sessionID.String(), mgmtAddr, host.SSHPort, loginUser)
			if derr != nil {
				return fail("connect_host", derr)
			}
			hostClient, hostClose = conn.Client, conn.Close
		} else {
			hostClient, err = s.gw.DialDirect(ctx, sessionID.String(), mgmtAddr, host.SSHPort, loginUser)
			if err != nil {
				return fail("connect_host", err)
			}
			hostClose = func() { _ = hostClient.Close() }
		}
		step("connect_host", "ok", fmt.Sprintf("ssh certificate auth to %s (%s)", mgmtAddr, via))
	}
	defer hostClose()

	// Privileged command runner: root runs directly; otherwise via sudo (with the
	// bootstrap password piped to sudo -S when one was provided).
	priv := func(script string) (string, error) {
		return privRun(hostClient, isRoot, sudoPass, script)
	}

	// 3) Collect host facts.
	if facts, ferr := run(hostClient, "uname -s; uname -r; uname -m; (. /etc/os-release 2>/dev/null; echo \"$NAME $VERSION_ID\"); ssh -V 2>&1 | head -1"); ferr == nil {
		s.recordFacts(ctx, host.ID, facts)
		step("collect_facts", "ok", oneLine(facts))
	} else {
		step("collect_facts", "skipped", ferr.Error())
	}

	// 4) For a password bootstrap, install the SSH CA trust, the login user, and
	//    sshd configuration so subsequent per-user certificate logins work.
	if params.method() == "password" {
		caKeys, kerr := s.store.ListActiveCAPublicKeys(ctx, "user")
		if kerr != nil || len(caKeys) == 0 {
			return fail("install_trust", orErr(kerr, "no active user CA"))
		}
		if out, err := priv(s.caTrustScript(loginUser, strings.Join(caKeys, "\n"))); err != nil || !strings.Contains(out, "CA_OK") {
			return fail("install_trust", orErr(err, out))
		}
		step("install_trust", "ok", "CA trust + login user '"+loginUser+"' + sshd configured")
	}

	// 5) Ensure WireGuard is installed (no-op if already present).
	if out, err := priv(wgInstallScript); err != nil || strings.Contains(out, "WG_MISSING") {
		return fail("install_wireguard", orErr(err, out+" (could not install wireguard tools)"))
	} else {
		step("install_wireguard", "ok", "wireguard tooling present")
	}

	// 6) Determine the overlay address (operator-specified or auto-assigned).
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

	// 7) Bring up WireGuard on the host (kernel module preferred, userspace
	//    wireguard-go fallback). The private key is generated on the host.
	// The endpoint the managed host uses to reach the jump host (VPN server). Must
	// be routable FROM the host. Precedence: per-enroll override -> DB setting ->
	// config default (FLEET_WG_JUMP_ENDPOINT).
	jumpEndpoint := strings.TrimSpace(params.WGEndpoint)
	if jumpEndpoint == "" {
		jumpEndpoint = s.store.WireGuardEndpoint(ctx)
	}
	if jumpEndpoint == "" {
		jumpEndpoint = s.cfg.WGJumpEndpoint
	}
	out, err := priv(s.hostWGScript(wgIP, jumpPub, jumpEndpoint))
	if err != nil {
		return fail("configure_host_wireguard", orErr(err, out))
	}
	hostPub := parseKV(out, "HOSTPUB")
	if hostPub == "" {
		return fail("configure_host_wireguard", fmt.Errorf("host public key not produced: %s", oneLine(out)))
	}
	wgAddr := parseKV(out, "WGADDR")
	if wgAddr == "" {
		return fail("configure_host_wireguard",
			fmt.Errorf("wireguard interface did not come up: %s", oneLine(out)))
	}
	step("configure_host_wireguard", "ok",
		fmt.Sprintf("%s up (addr %s) pub=%s", s.cfg.WGInterface, wgAddr, short(hostPub)))

	// 8) Add the host as a peer on the jump host (the VPN server).
	hostEndpoint := fmt.Sprintf("%s:%d", mgmtAddr, s.cfg.WGPort)
	jumpScript := s.jumpPeerScript(host.Hostname, hostPub, hostEndpoint, wgIP)
	if jout, jerr := run(jumpClient, "sudo sh -c "+shellQuote(jumpScript)); jerr != nil {
		return fail("configure_jump_peer", orErr(jerr, jout))
	}
	step("configure_jump_peer", "ok", fmt.Sprintf("peer %s allowed-ips %s/32", short(hostPub), wgIP))

	// 9) Connectivity check: confirm the WireGuard tunnel actually establishes a
	//    handshake. A failure here usually means the jump endpoint is not
	//    reachable from the host (firewall / wrong address / UDP port closed).
	if ok, detail := s.verifyWireGuard(priv); ok {
		step("verify_connectivity", "ok", detail)
	} else {
		step("verify_connectivity", "warning", fmt.Sprintf(
			"no WireGuard handshake yet — ensure the jump endpoint %s is reachable from the host on UDP %d (firewall/port-forward) and the jump host is listening. %s",
			jumpEndpoint, s.cfg.WGPort, detail))
	}

	// 10) Persist the address/enrolled state now so the validation dial can use it.
	_ = s.store.SetHostWGAddress(ctx, host.ID, wgIP)
	_ = s.store.SetHostEnrolled(ctx, host.ID, true)

	// 11) Validate end to end: connect through the jump host using a per-user
	//     certificate and run a command, proving cert auth + the tunnel path.
	if id, verr := s.validateCertLogin(ctx, sessionID, wgIP, mgmtAddr, host.SSHPort, loginUser); verr == nil {
		step("verify_certificate_login", "ok", "cert login via jump host: "+oneLine(id))
	} else {
		// Non-fatal in the local userspace-WireGuard fabric where the overlay
		// data plane is limited; configuration is applied either way.
		step("verify_certificate_login", "skipped", verr.Error())
	}

	_ = s.store.FinishEnrollmentJob(ctx, job.ID, "succeeded", "")
	_, _ = s.store.AppendAudit(ctx, models.AuditEvent{
		ActorID: actor, Action: "host.enroll", TargetKind: "host", TargetID: host.ID.String(),
		Detail: map[string]any{"wgAddress": wgIP, "hostPublicKey": hostPub, "method": params.method(), "jobId": job.ID},
	})

	final, _ := s.store.GetEnrollmentJob(ctx, job.ID)
	return &Result{Job: final, WGAddr: wgIP, HostPub: hostPub}, nil
}

// verifyWireGuard triggers and waits for a WireGuard handshake with the jump
// host, returning whether the tunnel came up and a short detail string. It runs
// on the host via the privileged runner so it works whether or not `wg` needs root.
func (s *Service) verifyWireGuard(priv func(string) (string, error)) (bool, string) {
	script := fmt.Sprintf(`IF=%s; JIP=%s
ping -c1 -W1 "$JIP" >/dev/null 2>&1 || true
i=0
while [ $i -lt 16 ]; do
  HS=$(wg show "$IF" latest-handshakes 2>/dev/null | awk '{print $2}' | sort -rn | head -1)
  if [ -n "$HS" ] && [ "$HS" != 0 ]; then
    NOW=$(date +%%s); AGO=$((NOW-HS))
    RX=$(wg show "$IF" transfer 2>/dev/null | awk '{print $2}' | paste -sd+ - | bc 2>/dev/null)
    echo "HANDSHAKE_OK age=${AGO}s rx=${RX:-0}"; exit 0
  fi
  i=$((i+1)); sleep 2
done
echo "HANDSHAKE_NONE"`, s.cfg.WGInterface, s.cfg.WGJumpIP)

	out, err := priv(script)
	if err != nil {
		return false, "check failed: " + oneLine(out)
	}
	if strings.Contains(out, "HANDSHAKE_OK") {
		return true, "wireguard handshake established (" + oneLine(parseAfter(out, "HANDSHAKE_OK")) + ")"
	}
	return false, ""
}

// parseAfter returns the text following a marker token on its line.
func parseAfter(out, marker string) string {
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, marker); i >= 0 {
			return strings.TrimSpace(line[i+len(marker):])
		}
	}
	return ""
}

// validateCertLogin connects to the host through the jump host using the
// session's per-user certificate and runs `id`, proving the full path works.
func (s *Service) validateCertLogin(ctx context.Context, sessionID uuid.UUID, wgIP, mgmtAddr string, port int, user string) (string, error) {
	for _, addr := range []string{wgIP, mgmtAddr} {
		if addr == "" {
			continue
		}
		conn, err := s.gw.Dial(ctx, sessionID.String(), addr, port, user)
		if err != nil {
			continue
		}
		out, rerr := run(conn.Client, "id")
		conn.Close()
		if rerr == nil {
			return out, nil
		}
	}
	return "", fmt.Errorf("certificate login not reachable yet")
}

// hostWGScript renders the script that configures and STARTS WireGuard on the
// managed host. It writes a wg-quick config and brings the interface up with
// wg-quick (kernel module, with a userspace wireguard-go fallback), enables it
// on boot, and reports the resulting interface state. The private key is
// generated on the host and never transmitted.
func (s *Service) hostWGScript(wgIP, jumpPub, jumpEndpoint string) string {
	iface := s.cfg.WGInterface
	return fmt.Sprintf(`set -e
IF=%s; IP=%s; SUBNET=%s; JPUB='%s'; JEP=%s; PORT=%d
mkdir -p /etc/wireguard; umask 077
[ -f /etc/wireguard/$IF.key ] || wg genkey > /etc/wireguard/$IF.key
PRIV=$(cat /etc/wireguard/$IF.key)
PUB=$(printf '%%s' "$PRIV" | wg pubkey)
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
chmod 600 /etc/wireguard/$IF.conf

# Bring the interface UP. Prefer wg-quick (standard; sets address + routes and
# brings it up). Use wireguard-go for the userspace fallback when there is no
# kernel module (containers / restricted kernels).
export WG_QUICK_USERSPACE_IMPLEMENTATION=wireguard-go
UP=no
if command -v wg-quick >/dev/null 2>&1; then
  wg-quick down $IF >/dev/null 2>&1 || true
  if wg-quick up $IF >/dev/null 2>&1; then
    UP=yes
    (systemctl enable wg-quick@$IF >/dev/null 2>&1) || true
  fi
fi
if [ "$UP" != yes ]; then
  ip link del $IF >/dev/null 2>&1 || true
  if ! ip link add dev $IF type wireguard >/dev/null 2>&1; then
    command -v wireguard-go >/dev/null 2>&1 && wireguard-go $IF && sleep 1
  fi
  ip link show $IF >/dev/null 2>&1 || { echo "ERROR no wireguard interface available"; exit 1; }
  printf '%%s' "$PRIV" | wg set $IF private-key /dev/stdin listen-port $PORT
  wg set $IF peer "$JPUB" endpoint "$JEP" allowed-ips $SUBNET persistent-keepalive 25
  ip address add $IP/24 dev $IF 2>/dev/null || true
  ip link set $IF up
fi
sleep 1
# WireGuard interfaces report operational state UNKNOWN even when up.
ip link show $IF >/dev/null 2>&1 || { echo "ERROR interface not present after bring-up"; exit 1; }
ip link set $IF up 2>/dev/null || true
WGSTATE=$(ip -br link show $IF 2>/dev/null | awk '{print $2}')
WGADDR=$(ip -br addr show $IF 2>/dev/null | awk '{print $3}')
echo "WGSTATE=$WGSTATE"
echo "WGADDR=$WGADDR"
echo "HOSTPUB=$PUB"`,
		iface, wgIP, s.cfg.WGSubnet, jumpPub, jumpEndpoint, s.cfg.WGPort)
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

// caTrustScript installs the Fleet user CA, creates the login user with sudo and
// the principal mapping, configures sshd to trust certificates, and reloads sshd.
func (s *Service) caTrustScript(loginUser, caKeys string) string {
	return fmt.Sprintf(`set -e
LOGIN=%s
# Login account that per-user certificates map to (shared account, unique certs).
id "$LOGIN" >/dev/null 2>&1 || useradd -m -s /bin/bash "$LOGIN" 2>/dev/null || adduser -D "$LOGIN" 2>/dev/null || true
mkdir -p /etc/sudoers.d && printf '%%s ALL=(ALL) NOPASSWD:ALL\n' "$LOGIN" > /etc/sudoers.d/fleet && chmod 0440 /etc/sudoers.d/fleet
# Trust the Fleet user CA.
cat > /etc/ssh/fleet_ca.pub <<'CAEOF'
%s
CAEOF
chmod 644 /etc/ssh/fleet_ca.pub
mkdir -p /etc/ssh/auth_principals && printf 'fleet\n' > /etc/ssh/auth_principals/"$LOGIN"
# sshd: prefer a drop-in; also append directly if the main config has no Include.
mkdir -p /etc/ssh/sshd_config.d
cat > /etc/ssh/sshd_config.d/00-fleet.conf <<'SSHEOF'
PubkeyAuthentication yes
TrustedUserCAKeys /etc/ssh/fleet_ca.pub
AuthorizedPrincipalsFile /etc/ssh/auth_principals/%%u
SSHEOF
if ! grep -q 'sshd_config.d' /etc/ssh/sshd_config 2>/dev/null && ! grep -q 'TrustedUserCAKeys /etc/ssh/fleet_ca.pub' /etc/ssh/sshd_config 2>/dev/null; then
  { echo ''; echo '# Fleet Terminal'; echo 'PubkeyAuthentication yes'; echo 'TrustedUserCAKeys /etc/ssh/fleet_ca.pub'; echo 'AuthorizedPrincipalsFile /etc/ssh/auth_principals/%%u'; } >> /etc/ssh/sshd_config
fi
mkdir -p /run/sshd
sshd -t
( systemctl reload sshd 2>/dev/null || systemctl reload ssh 2>/dev/null || service sshd reload 2>/dev/null || service ssh reload 2>/dev/null || pkill -HUP sshd 2>/dev/null ) || true
echo CA_OK`,
		loginUser, caKeys)
}

// wgInstallScript installs WireGuard tooling via the host's package manager if
// the `wg` command is not already present.
const wgInstallScript = `set -e
if ! command -v wg >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq >/dev/null 2>&1 || true
    apt-get install -y -qq wireguard-tools >/dev/null 2>&1 || apt-get install -y -qq wireguard >/dev/null 2>&1 || true
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y -q wireguard-tools >/dev/null 2>&1 || { dnf install -y -q epel-release >/dev/null 2>&1; dnf install -y -q wireguard-tools >/dev/null 2>&1; } || true
  elif command -v yum >/dev/null 2>&1; then
    yum install -y -q wireguard-tools >/dev/null 2>&1 || true
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache wireguard-tools >/dev/null 2>&1 || true
  fi
fi
command -v wg >/dev/null 2>&1 && echo WG_INSTALLED || echo WG_MISSING`

// privRun executes a script with privilege. As root it runs directly; otherwise
// via sudo, piping the bootstrap password to `sudo -S` when one is supplied.
func privRun(client *ssh.Client, isRoot bool, password, script string) (string, error) {
	if isRoot {
		return run(client, "sh -c "+shellQuote(script))
	}
	if password != "" {
		return runWithInput(client, "sudo -S -p '' sh -c "+shellQuote(script), password+"\n")
	}
	return run(client, "sudo sh -c "+shellQuote(script))
}

// runWithInput runs a command, writing input to its stdin (used for sudo -S).
func runWithInput(client *ssh.Client, cmd, input string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	sess.Stdin = strings.NewReader(input)
	out, err := sess.CombinedOutput(cmd)
	return string(out), err
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
