// Package monitor performs authenticated SSH health checks (no ICMP) against
// enrolled hosts through the jump host, updates host_status, and pushes live
// updates to dashboards over the WebSocket hub.
package monitor

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/fleet-terminal/backend/internal/config"
	"github.com/fleet-terminal/backend/internal/identity"
	"github.com/fleet-terminal/backend/internal/metrics"
	"github.com/fleet-terminal/backend/internal/models"
	"github.com/fleet-terminal/backend/internal/sshgw"
	"github.com/fleet-terminal/backend/internal/store"
	"github.com/fleet-terminal/backend/internal/ws"
)

// Monitor periodically probes hosts and reports their health.
type Monitor struct {
	store  *store.Store
	cfg    *config.Config
	log    *slog.Logger
	gw     *sshgw.Gateway
	issuer *identity.Issuer
	hub    *ws.Hub

	interval time.Duration
}

// New constructs a Monitor.
func New(st *store.Store, cfg *config.Config, log *slog.Logger, gw *sshgw.Gateway, issuer *identity.Issuer, hub *ws.Hub) *Monitor {
	return &Monitor{store: st, cfg: cfg, log: log, gw: gw, issuer: issuer, hub: hub, interval: 30 * time.Second}
}

// Run drives the monitoring loop until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	// Initial probe shortly after startup, then on the interval.
	t := time.NewTimer(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.sweep(ctx)
			t.Reset(m.interval)
		}
	}
}

// sweep probes every enrolled host once.
func (m *Monitor) sweep(ctx context.Context) {
	hosts, err := m.store.ListHosts(ctx, 1000, 0)
	if err != nil {
		m.log.Warn("monitor list hosts", "err", err)
		return
	}
	signer, err := m.issuer.SystemSigner(ctx, []string{"fleet"}, 24*time.Hour)
	if err != nil {
		m.log.Warn("monitor system signer", "err", err)
		return
	}
	for i := range hosts {
		h := hosts[i]
		if !h.Enrolled {
			continue
		}
		st := m.probe(ctx, signer, &h)
		if err := m.store.UpdateStatus(ctx, h.ID, st); err != nil {
			m.log.Warn("monitor update status", "host", h.Hostname, "err", err)
			continue
		}
		m.hub.Broadcast("host.status", map[string]any{
			"hostId": h.ID, "hostname": h.Hostname, "status": st.Status,
			"latencyMs": st.LatencyMS, "sshOk": st.SSHOK, "wgOk": st.WGOK,
			"uptimeSeconds": st.UptimeSeconds, "checkedAt": time.Now(),
		})
	}
	m.refreshGauges(ctx)
}

// probe runs a lightweight authenticated SSH command through the jump host and
// records latency, uptime, and SSH/WireGuard health.
func (m *Monitor) probe(ctx context.Context, signer ssh.Signer, h *models.Host) models.HostStatus {
	now := time.Now()
	st := models.HostStatus{Status: "unknown", CheckedAt: &now}

	candidates := dedupe([]string{h.WGAddress, h.Address, h.Hostname})
	var conn *sshgw.Conn
	var dialErr error
	for _, addr := range candidates {
		start := time.Now()
		conn, dialErr = m.gw.DialWithSigner(ctx, signer, addr, h.SSHPort, h.SSHUser)
		if dialErr == nil {
			lat := int(time.Since(start).Milliseconds())
			st.LatencyMS = &lat
			// If we reached it via the WireGuard address, the overlay is healthy.
			st.WGOK = addr == h.WGAddress && h.WGAddress != ""
			break
		}
	}
	if dialErr != nil || conn == nil {
		st.Status = "offline"
		st.SSHOK = false
		st.LastError = trunc(errStr(dialErr), 240)
		st.LastFailureAt = &now
		return st
	}
	defer conn.Close()
	st.SSHOK = true
	st.Status = "online"
	st.LastSuccessAt = &now

	if out, err := runCmd(conn, "cat /proc/uptime 2>/dev/null"); err == nil {
		st.UptimeSeconds = parseUptime(out)
	}
	// WireGuard handshake freshness (best-effort; requires sudo on the host).
	if !st.WGOK {
		if out, err := runCmd(conn, "sudo wg show "+m.cfg.WGInterface+" latest-handshakes 2>/dev/null | awk '{print $2}' | sort -rn | head -1"); err == nil {
			if hs := strings.TrimSpace(out); hs != "" && hs != "0" {
				if v, perr := strconv.ParseInt(hs, 10, 64); perr == nil && time.Now().Unix()-v < 180 {
					st.WGOK = true
				}
			}
		}
	}
	return st
}

func runCmd(conn *sshgw.Conn, cmd string) (string, error) {
	sess, err := conn.Client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out, err := sess.CombinedOutput(cmd)
	return string(out), err
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

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (m *Monitor) refreshGauges(ctx context.Context) {
	counts, err := m.store.CountHostsByStatus(ctx)
	if err != nil {
		return
	}
	for status, n := range counts {
		metrics.HostsByStatus.WithLabelValues(status).Set(float64(n))
	}
}

// parseUptime extracts seconds from the contents of /proc/uptime.
func parseUptime(s string) *int64 {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return nil
	}
	f, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil
	}
	v := int64(f)
	return &v
}
