// Package sftp provides audited file transfer (list/download/upload) to managed
// hosts, brokered through the SSH gateway. The browser never speaks SFTP — the
// backend opens the SFTP subsystem over the session's gateway connection.
package sftp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	pkgsftp "github.com/pkg/sftp"

	"github.com/fleet-terminal/backend/internal/app"
	"github.com/fleet-terminal/backend/internal/auth"
	"github.com/fleet-terminal/backend/internal/models"
	"github.com/fleet-terminal/backend/internal/sshgw"
	"github.com/fleet-terminal/backend/internal/store"
)

// Mount attaches SFTP routes (require auth + File.Transfer + host access).
func Mount(r chi.Router, d *app.Deps, gw *sshgw.Gateway) {
	h := &handler{d: d, gw: gw}
	r.Group(func(pr chi.Router) {
		pr.Use(d.Auth.RequireAuth)
		pr.With(d.Auth.RequirePermission("File.Transfer")).Get("/hosts/{id}/sftp/list", h.list)
		pr.With(d.Auth.RequirePermission("File.Transfer")).Get("/hosts/{id}/sftp/download", h.download)
		pr.With(d.Auth.RequirePermission("File.Transfer")).Post("/hosts/{id}/sftp/upload", h.upload)
		pr.With(d.Auth.RequirePermission("File.Transfer")).Get("/hosts/{id}/sftp/transfers", h.transfers)
	})
}

type handler struct {
	d  *app.Deps
	gw *sshgw.Gateway
}

// connect opens an SFTP client to the host through the gateway, after access checks.
func (h *handler) connect(w http.ResponseWriter, r *http.Request) (*pkgsftp.Client, *sshgw.Conn, *auth.Principal, *models.Host, bool) {
	p := auth.MustPrincipal(r)
	hostID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid host id")
		return nil, nil, nil, nil, false
	}
	if !p.IsSuperAdmin {
		ok, aerr := h.d.Store.UserCanAccessHost(r.Context(), p.UserID, hostID)
		if aerr != nil || !ok {
			writeError(w, http.StatusForbidden, "not authorized for host")
			return nil, nil, nil, nil, false
		}
	}
	host, err := h.d.Store.GetHost(r.Context(), hostID)
	if err != nil {
		writeError(w, http.StatusNotFound, "host not found")
		return nil, nil, nil, nil, false
	}
	conn, derr := h.dial(r, p, host)
	if derr != nil {
		writeError(w, http.StatusBadGateway, "connection failed: "+derr.Error())
		return nil, nil, nil, nil, false
	}
	client, serr := pkgsftp.NewClient(conn.Client)
	if serr != nil {
		conn.Close()
		writeError(w, http.StatusBadGateway, "sftp subsystem failed: "+serr.Error())
		return nil, nil, nil, nil, false
	}
	return client, conn, p, host, true
}

func (h *handler) dial(r *http.Request, p *auth.Principal, host *models.Host) (*sshgw.Conn, error) {
	var lastErr error
	for _, addr := range dedupe([]string{host.WGAddress, host.Address, host.Hostname}) {
		conn, err := h.gw.Dial(r.Context(), p.SessionID.String(), addr, host.SSHPort, host.SSHUser)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no reachable address")
	}
	return nil, lastErr
}

type entry struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	IsDir   bool      `json:"isDir"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"modTime"`
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	client, conn, _, _, ok := h.connect(w, r)
	if !ok {
		return
	}
	defer conn.Close()
	defer client.Close()

	dir := r.URL.Query().Get("path")
	if dir == "" {
		dir = "."
	}
	infos, err := client.ReadDir(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read dir: "+err.Error())
		return
	}
	entries := make([]entry, 0, len(infos))
	for _, fi := range infos {
		entries = append(entries, entry{
			Name: fi.Name(), Size: fi.Size(), IsDir: fi.IsDir(),
			Mode: fi.Mode().String(), ModTime: fi.ModTime(),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
	abs, _ := client.RealPath(dir)
	writeJSON(w, http.StatusOK, map[string]any{"path": abs, "entries": entries})
}

func (h *handler) download(w http.ResponseWriter, r *http.Request) {
	client, conn, p, host, ok := h.connect(w, r)
	if !ok {
		return
	}
	defer conn.Close()
	defer client.Close()

	remote := r.URL.Query().Get("path")
	if remote == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}
	f, err := client.Open(remote)
	if err != nil {
		writeError(w, http.StatusNotFound, "open: "+err.Error())
		return
	}
	defer f.Close()
	rec, _ := h.d.Store.RecordSFTPTransfer(r.Context(), store.SFTPTransferInput{
		UserID: &p.UserID, HostID: &host.ID, Direction: "download", RemotePath: remote, Status: "started",
	})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(remote)))
	n, _ := io.Copy(w, f)
	h.finishTransfer(r, rec, n)
	h.audit(r, p, "sftp.download", host.ID, remote, n)
}

func (h *handler) upload(w http.ResponseWriter, r *http.Request) {
	client, conn, p, host, ok := h.connect(w, r)
	if !ok {
		return
	}
	defer conn.Close()
	defer client.Close()

	dir := r.URL.Query().Get("path")
	name := r.URL.Query().Get("name")
	if dir == "" || name == "" {
		writeError(w, http.StatusBadRequest, "path and name required")
		return
	}
	remote := path.Join(dir, path.Base(name))
	dst, err := client.Create(remote)
	if err != nil {
		writeError(w, http.StatusBadGateway, "create: "+err.Error())
		return
	}
	defer dst.Close()
	rec, _ := h.d.Store.RecordSFTPTransfer(r.Context(), store.SFTPTransferInput{
		UserID: &p.UserID, HostID: &host.ID, Direction: "upload", RemotePath: remote, Status: "started",
	})
	// Enforce the configured cap (0 = unlimited) and REJECT oversized uploads
	// rather than silently truncating. The partial remote file is removed.
	body := r.Body
	if limit := h.d.Cfg.MaxUploadBytes; limit > 0 {
		body = http.MaxBytesReader(w, r.Body, limit)
	}
	n, cerr := io.Copy(dst, body)
	if cerr != nil {
		_ = dst.Close()
		_ = client.Remove(remote)
		if rec != nil {
			_ = h.d.Store.CompleteSFTPTransfer(r.Context(), rec.ID, n, "failed")
		}
		var mbe *http.MaxBytesError
		if errors.As(cerr, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("file exceeds the %d-byte upload limit", h.d.Cfg.MaxUploadBytes))
			return
		}
		writeError(w, http.StatusBadGateway, "write: "+cerr.Error())
		return
	}
	h.finishTransfer(r, rec, n)
	h.audit(r, p, "sftp.upload", host.ID, remote, n)
	writeJSON(w, http.StatusOK, map[string]any{"path": remote, "size": n})
}

func (h *handler) transfers(w http.ResponseWriter, r *http.Request) {
	hostID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid host id")
		return
	}
	list, err := h.d.Store.ListSFTPTransfers(r.Context(), store.SFTPTransferFilter{HostID: &hostID, Limit: 100})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transfers": list})
}

func (h *handler) finishTransfer(r *http.Request, rec *store.SFTPTransfer, n int64) {
	if rec == nil {
		return
	}
	_ = h.d.Store.CompleteSFTPTransfer(r.Context(), rec.ID, n, "completed")
}

func (h *handler) audit(r *http.Request, p *auth.Principal, action string, hostID uuid.UUID, remote string, n int64) {
	_, _ = h.d.Store.AppendAudit(r.Context(), models.AuditEvent{
		ActorID: &p.UserID, ActorName: p.Username, Action: action,
		TargetKind: "host", TargetID: hostID.String(),
		Detail: map[string]any{"path": remote, "bytes": n},
	})
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
