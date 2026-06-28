// Package scan runs OpenSCAP (oscap) security/compliance scans against managed
// hosts over the backend's SSH gateway, stores the HTML report, and parses a
// pass/fail summary. The backend is the sole SSH client (as everywhere else):
// it dials through the jump host as the privileged `fleet` account and runs
// oscap under sudo.
package scan

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	"github.com/fleet-terminal/backend/internal/config"
	"github.com/fleet-terminal/backend/internal/identity"
	"github.com/fleet-terminal/backend/internal/models"
	"github.com/fleet-terminal/backend/internal/sshgw"
	"github.com/fleet-terminal/backend/internal/store"
)

// scanTimeout bounds a full scan (install + evaluate); oscap runs can be slow.
const scanTimeout = 30 * time.Minute

// profileIDRe guards the profile id we interpolate into the remote shell.
var profileIDRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]*$`)

// Service orchestrates scans. It mirrors the monitor: a system signer maps to
// the privileged host account, dialed through the jump host.
type Service struct {
	store  *store.Store
	cfg    *config.Config
	log    *slog.Logger
	gw     *sshgw.Gateway
	issuer *identity.Issuer

	// installing tracks hosts with an in-flight background scanner install, so
	// repeated "prepare" requests don't kick off duplicate installs.
	installing sync.Map // hostID -> struct{}
}

func New(st *store.Store, cfg *config.Config, log *slog.Logger, gw *sshgw.Gateway, issuer *identity.Issuer) *Service {
	return &Service{store: st, cfg: cfg, log: log, gw: gw, issuer: issuer}
}

// dial opens a privileged connection to the host, trying its candidate
// addresses in turn (WireGuard overlay first, then management address/hostname).
func (s *Service) dial(ctx context.Context, h *models.Host) (*sshgw.Conn, error) {
	signer, err := s.issuer.SystemSigner(ctx, []string{"fleet"}, 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("system signer: %w", err)
	}
	var lastErr error
	for _, addr := range dedupe([]string{h.WGAddress, h.Address, h.Hostname}) {
		conn, derr := s.gw.DialWithSigner(ctx, signer, addr, h.SSHPort, h.SSHUser)
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no reachable address")
	}
	return nil, lastErr
}

// DiscoverProfiles reports whether the host already has oscap + SCAP content and,
// if so, the available profiles for its datastream. It never installs anything
// (kept fast for opening the scan dialog); the scan itself auto-installs.
func (s *Service) DiscoverProfiles(ctx context.Context, h *models.Host) (installed bool, datastream string, profiles []models.ScanProfile, err error) {
	conn, err := s.dial(ctx, h)
	if err != nil {
		return false, "", nil, err
	}
	defer conn.Close()
	out, err := runScript(ctx, conn, discoverScript)
	if err != nil {
		return false, "", nil, err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "STATUS="):
			if v := strings.TrimPrefix(line, "STATUS="); v != "ok" {
				return false, "", nil, nil // missing scanner or content
			}
			installed = true
		case strings.HasPrefix(line, "DATASTREAM="):
			datastream = strings.TrimPrefix(line, "DATASTREAM=")
		case strings.Contains(line, ":") && strings.HasPrefix(line, "xccdf_"):
			id, title, _ := strings.Cut(line, ":")
			profiles = append(profiles, models.ScanProfile{ID: strings.TrimSpace(id), Title: strings.TrimSpace(title)})
		}
	}
	return installed, datastream, profiles, nil
}

// IsInstalling reports whether a background scanner install is in flight for the host.
func (s *Service) IsInstalling(hostID uuid.UUID) bool {
	_, ok := s.installing.Load(hostID)
	return ok
}

// EnsureInstalled installs the scanner + SCAP content on the host in the
// background if not already running, so the profile picker can populate before
// the first scan (a sync request can't wait out a multi-minute package install).
// Idempotent: the install script is a no-op when oscap + content are present.
func (s *Service) EnsureInstalled(h *models.Host) {
	if _, loaded := s.installing.LoadOrStore(h.ID, struct{}{}); loaded {
		return // already installing
	}
	go func() {
		defer s.installing.Delete(h.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		conn, err := s.dial(ctx, h)
		if err != nil {
			s.log.Warn("scan prepare: dial", "host", h.Hostname, "err", err)
			return
		}
		defer conn.Close()
		if _, err := runScript(ctx, conn, installScript); err != nil {
			s.log.Warn("scan prepare: install", "host", h.Hostname, "err", err)
			return
		}
		s.log.Info("scan prepare: scanner ready", "host", h.Hostname)
	}()
}

// Run executes a scan in the background and records its outcome. It is launched
// in its own goroutine by the handler; ctx should be detached (context.Background).
func (s *Service) Run(scanID uuid.UUID, h *models.Host, profile string) {
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	fail := func(msg string) {
		s.log.Warn("host scan failed", "host", h.Hostname, "scan", scanID, "err", msg)
		_ = s.store.FailHostScan(ctx, scanID, msg)
	}

	if !profileIDRe.MatchString(profile) {
		fail("invalid profile id")
		return
	}
	conn, err := s.dial(ctx, h)
	if err != nil {
		fail("connect: " + err.Error())
		return
	}
	defer conn.Close()

	out, err := runScript(ctx, conn, fmt.Sprintf(scanScript, profile))
	if err != nil {
		fail("run oscap: " + err.Error())
		return
	}

	head, report, ok := strings.Cut(out, reportDelimiter)
	meta := parseKV(head)
	if status := meta["STATUS"]; status != "ok" {
		if status == "" {
			status = "scan produced no result"
		}
		// Surface oscap's own output (captured on failure) so the cause is visible.
		if detail := extractBlock(head, "DETAIL_BEGIN", "DETAIL_END"); detail != "" {
			status += " — " + strings.ReplaceAll(detail, "\n", " | ")
		}
		fail("oscap: " + status)
		return
	}
	if !ok || strings.TrimSpace(report) == "" {
		fail("oscap produced no report")
		return
	}

	if err := s.store.StartHostScan(ctx, scanID, meta["PROFILE"], meta["PROFILE_TITLE"], meta["DATASTREAM"]); err != nil {
		s.log.Warn("host scan start record", "scan", scanID, "err", err)
	}

	reportPath := filepath.Join(s.cfg.ScanDir, scanID.String()+".html")
	if err := os.MkdirAll(s.cfg.ScanDir, 0o750); err != nil {
		fail("store report: " + err.Error())
		return
	}
	if err := os.WriteFile(reportPath, []byte(report), 0o640); err != nil {
		fail("write report: " + err.Error())
		return
	}

	pass, failCnt, other := atoi(meta["PASS"]), atoi(meta["FAIL"]), atoi(meta["OTHER"])
	sum := store.ScanSummary{
		PassCount: pass, FailCount: failCnt, OtherCount: other,
		TotalRules: pass + failCnt + other, ReportPath: reportPath,
	}
	if v, perr := strconv.ParseFloat(strings.TrimSpace(meta["SCORE"]), 64); perr == nil {
		sum.Score = &v
	}
	if err := s.store.CompleteHostScan(ctx, scanID, sum); err != nil {
		fail("record results: " + err.Error())
		return
	}
	s.log.Info("host scan completed", "host", h.Hostname, "scan", scanID,
		"profile", meta["PROFILE"], "pass", pass, "fail", failCnt)
}

// --- remote scripts ---

const reportDelimiter = "=====FLEET_REPORT_HTML_BEGIN====="

const discoverScript = `C=/usr/share/xml/scap/ssg/content
command -v oscap >/dev/null 2>&1 || { echo "STATUS=missing"; exit 0; }
ID=$(. /etc/os-release 2>/dev/null; echo "$ID"); VER=$(. /etc/os-release 2>/dev/null; echo "$VERSION_ID" | tr -d .)
DS=""
for c in "$C/ssg-${ID}${VER}-ds.xml" "$C/ssg-${ID}-ds.xml"; do [ -f "$c" ] && DS="$c" && break; done
[ -z "$DS" ] && DS=$(ls "$C"/ssg-*-ds.xml 2>/dev/null | grep -i "$ID" | head -1)
[ -z "$DS" ] && DS=$(ls "$C"/ssg-*-ds.xml 2>/dev/null | head -1)
[ -z "$DS" ] && { echo "STATUS=nocontent"; exit 0; }
echo "STATUS=ok"
echo "DATASTREAM=$DS"
oscap info --profiles "$DS" 2>/dev/null`

// installScript installs the scanner + SCAP content if missing (the install
// portion of scanScript, run standalone by EnsureInstalled). No-op when present.
const installScript = `C=/usr/share/xml/scap/ssg/content
if ! command -v oscap >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then sudo DEBIAN_FRONTEND=noninteractive apt-get update -qq >/dev/null 2>&1; sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq openscap-scanner >/dev/null 2>&1;
  elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y openscap-scanner >/dev/null 2>&1;
  elif command -v yum >/dev/null 2>&1; then sudo yum install -y openscap-scanner >/dev/null 2>&1;
  elif command -v zypper >/dev/null 2>&1; then sudo zypper --non-interactive install openscap-utils >/dev/null 2>&1; fi
fi
if ! ls "$C"/ssg-*-ds.xml >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ssg-base ssg-debian ssg-debderived ssg-nondebian ssg-applications >/dev/null 2>&1 || sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq scap-security-guide >/dev/null 2>&1;
  elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y scap-security-guide >/dev/null 2>&1;
  elif command -v yum >/dev/null 2>&1; then sudo yum install -y scap-security-guide >/dev/null 2>&1;
  elif command -v zypper >/dev/null 2>&1; then sudo zypper --non-interactive install scap-security-guide >/dev/null 2>&1; fi
fi
# Debian/Ubuntu lack oscap's global CPE dictionary; provide an empty valid one
# (real platform detection comes from the SSG datastream). Guarded for distros
# that already ship it.
if command -v oscap >/dev/null 2>&1 && [ ! -f /usr/share/openscap/cpe/openscap-cpe-dict.xml ]; then sudo mkdir -p /usr/share/openscap/cpe 2>/dev/null; printf '<?xml version="1.0" encoding="UTF-8"?>\n<cpe-list xmlns="http://cpe.mitre.org/dictionary/2.0"/>\n' | sudo tee /usr/share/openscap/cpe/openscap-cpe-dict.xml >/dev/null 2>&1; fi
echo done`

// scanScript installs oscap + SCAP content if missing, resolves the host's
// datastream, evaluates the given profile (empty -> the standard profile), and
// prints a key/value summary followed by the HTML report after the delimiter.
// %s is the validated profile id. No `set -e`: oscap returns 2 when rules fail.
const scanScript = `C=/usr/share/xml/scap/ssg/content
if ! command -v oscap >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then sudo DEBIAN_FRONTEND=noninteractive apt-get update -qq >/dev/null 2>&1; sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq openscap-scanner >/dev/null 2>&1;
  elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y openscap-scanner >/dev/null 2>&1;
  elif command -v yum >/dev/null 2>&1; then sudo yum install -y openscap-scanner >/dev/null 2>&1;
  elif command -v zypper >/dev/null 2>&1; then sudo zypper --non-interactive install openscap-utils >/dev/null 2>&1; fi
fi
if ! ls "$C"/ssg-*-ds.xml >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ssg-base ssg-debian ssg-debderived ssg-nondebian ssg-applications >/dev/null 2>&1 || sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq scap-security-guide >/dev/null 2>&1;
  elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y scap-security-guide >/dev/null 2>&1;
  elif command -v yum >/dev/null 2>&1; then sudo yum install -y scap-security-guide >/dev/null 2>&1;
  elif command -v zypper >/dev/null 2>&1; then sudo zypper --non-interactive install scap-security-guide >/dev/null 2>&1; fi
fi
command -v oscap >/dev/null 2>&1 || { echo "STATUS=scanner_unavailable"; exit 0; }
# Debian/Ubuntu don't ship oscap's global CPE dictionary; without it oscap can't
# confirm the platform (every rule -> notapplicable) and aborts (SIGABRT) at the
# end. An empty valid dict is enough — real platform detection comes from the SSG
# datastream. Guarded so distros that ship it (RHEL etc.) are untouched.
if [ ! -f /usr/share/openscap/cpe/openscap-cpe-dict.xml ]; then sudo mkdir -p /usr/share/openscap/cpe 2>/dev/null; printf '<?xml version="1.0" encoding="UTF-8"?>\n<cpe-list xmlns="http://cpe.mitre.org/dictionary/2.0"/>\n' | sudo tee /usr/share/openscap/cpe/openscap-cpe-dict.xml >/dev/null 2>&1; fi
ID=$(. /etc/os-release 2>/dev/null; echo "$ID"); VER=$(. /etc/os-release 2>/dev/null; echo "$VERSION_ID" | tr -d .)
DS=""
for c in "$C/ssg-${ID}${VER}-ds.xml" "$C/ssg-${ID}-ds.xml"; do [ -f "$c" ] && DS="$c" && break; done
[ -z "$DS" ] && DS=$(ls "$C"/ssg-*-ds.xml 2>/dev/null | grep -i "$ID" | head -1)
[ -z "$DS" ] && DS=$(ls "$C"/ssg-*-ds.xml 2>/dev/null | head -1)
[ -z "$DS" ] && { echo "STATUS=no_content"; exit 0; }
PROFILE='%s'
if [ -z "$PROFILE" ]; then
  PROFILE=$(oscap info --profiles "$DS" 2>/dev/null | grep -iE 'profile_standard' | head -1 | cut -d: -f1)
  [ -z "$PROFILE" ] && PROFILE=$(oscap info --profiles "$DS" 2>/dev/null | head -1 | cut -d: -f1)
fi
[ -z "$PROFILE" ] && { echo "STATUS=no_profile"; exit 0; }
PT=$(oscap info --profiles "$DS" 2>/dev/null | grep -F "$PROFILE:" | head -1 | cut -d: -f2-)
R=/tmp/fleet-scan-$$
sudo oscap xccdf eval --profile "$PROFILE" --results "$R-results.xml" --report "$R-report.html" "$DS" >"$R-out.log" 2>&1
RC=$?
if [ $RC -ne 0 ] && [ $RC -ne 2 ]; then
  echo "STATUS=eval_failed_rc_$RC"
  echo "DATASTREAM=$DS"
  echo "PROFILE=$PROFILE"
  echo "DETAIL_BEGIN"
  tail -c 1500 "$R-out.log" 2>/dev/null | tr -d '\r'
  echo ""
  echo "DETAIL_END"
  sudo rm -f "$R-results.xml" "$R-report.html" "$R-out.log" 2>/dev/null
  exit 0
fi
sudo rm -f "$R-out.log" 2>/dev/null
sudo chmod 0644 "$R-results.xml" "$R-report.html" 2>/dev/null
PASS=$(grep -c '<result>pass</result>' "$R-results.xml" 2>/dev/null || echo 0)
FAIL=$(grep -c '<result>fail</result>' "$R-results.xml" 2>/dev/null || echo 0)
ERR=$(grep -c '<result>error</result>' "$R-results.xml" 2>/dev/null || echo 0)
OTH=$(grep -cE '<result>(notapplicable|notchecked|notselected|informational|unknown|fixed)</result>' "$R-results.xml" 2>/dev/null || echo 0)
SCORE=$(grep -oE '<score[^>]*>[0-9.]+</score>' "$R-results.xml" | head -1 | grep -oE '[0-9.]+' | tail -1)
echo "STATUS=ok"
echo "DATASTREAM=$DS"
echo "PROFILE=$PROFILE"
echo "PROFILE_TITLE=$PT"
echo "SCORE=$SCORE"
echo "PASS=$PASS"
echo "FAIL=$FAIL"
echo "OTHER=$((ERR+OTH))"
echo "` + reportDelimiter + `"
cat "$R-report.html"
sudo rm -f "$R-results.xml" "$R-report.html" 2>/dev/null`

// --- helpers ---

// runScript runs a /bin/sh script over the connection, respecting ctx by killing
// the session if it is cancelled (e.g. scan timeout).
func runScript(ctx context.Context, conn *sshgw.Conn, script string) (string, error) {
	sess, err := conn.Client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, rerr := sess.CombinedOutput(script)
		done <- result{out, rerr}
	}()
	select {
	case <-ctx.Done():
		_ = sess.Signal(ssh.SIGKILL)
		_ = sess.Close()
		return "", ctx.Err()
	case r := <-done:
		return string(r.out), r.err
	}
}

// extractBlock returns the text between the begin and end markers, trimmed.
func extractBlock(s, begin, end string) string {
	i := strings.Index(s, begin)
	if i < 0 {
		return ""
	}
	rest := s[i+len(begin):]
	if j := strings.Index(rest, end); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

func parseKV(s string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if k, v, ok := strings.Cut(line, "="); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
