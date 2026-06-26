// Package sessionsapi exposes read-only access to recorded SSH sessions and
// their replay recordings. All routes are gated by authentication plus the
// Session.Replay permission.
package sessionsapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fleet-terminal/backend/internal/app"
	"github.com/fleet-terminal/backend/internal/models"
	"github.com/fleet-terminal/backend/internal/store"
)

// Mount attaches session routes to r, gated by authentication and permissions.
func Mount(r chi.Router, d *app.Deps) {
	h := &handler{d: d}
	r.Group(func(pr chi.Router) {
		pr.Use(d.Auth.RequireAuth)

		pr.With(d.Auth.RequirePermission("Session.Replay")).Get("/sessions", h.list)
		pr.With(d.Auth.RequirePermission("Session.Replay")).Get("/sessions/{id}", h.get)
		pr.With(d.Auth.RequirePermission("Session.Replay")).Get("/sessions/{id}/recording", h.recording)
	})
}

type handler struct{ d *app.Deps }

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	f := store.SSHSessionFilter{Limit: limit, Offset: offset}
	if user := r.URL.Query().Get("user"); user != "" {
		id, err := uuid.Parse(user)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		f.UserID = &id
	}
	if host := r.URL.Query().Get("host"); host != "" {
		id, err := uuid.Parse(host)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid host id")
			return
		}
		f.HostID = &id
	}
	sessions, err := h.d.Store.ListSSHSessions(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list sessions")
		return
	}
	if sessions == nil {
		sessions = []models.SSHSession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "count": len(sessions)})
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	sess, err := h.d.Store.GetSSHSession(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (h *handler) recording(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	rec, err := h.d.Store.GetRecordingBySession(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "recording not found")
		return
	}
	// Resolve the on-disk path; relative paths are taken under the configured
	// recording directory.
	path := rec.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(h.d.Cfg.RecordingDir, path)
	}
	cast, err := os.ReadFile(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "recording file not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recording": rec, "cast": string(cast)})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
