package enrollment

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fleet-terminal/backend/internal/app"
	"github.com/fleet-terminal/backend/internal/auth"
)

// Mount attaches enrollment routes. Enrollment uses the caller's live session
// certificate, so routes require authentication + Host.Enroll.
func Mount(r chi.Router, d *app.Deps, svc *Service) {
	h := &handler{d: d, svc: svc}
	r.Group(func(pr chi.Router) {
		pr.Use(d.Auth.RequireAuth)
		pr.With(d.Auth.RequirePermission("Host.Enroll")).Post("/hosts/{id}/enroll", h.enroll)
		pr.With(d.Auth.RequirePermission("Host.Enroll")).Get("/enrollment/jobs", h.listJobs)
		pr.With(d.Auth.RequirePermission("Host.Enroll")).Get("/enrollment/jobs/{id}", h.getJob)
	})
}

type handler struct {
	d   *app.Deps
	svc *Service
}

type enrollReq struct {
	Method        string `json:"method"`        // "password" | "trusted"
	BootstrapUser string `json:"bootstrapUser"` // SSH user for password bootstrap
	Password      string `json:"password"`      // SSH/sudo password for bootstrap
	ViaJump       bool   `json:"viaJump"`       // route bootstrap through the jump host
}

func (h *handler) enroll(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r)
	hostID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid host id")
		return
	}
	host, err := h.d.Store.GetHost(r.Context(), hostID)
	if err != nil {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}
	var req enrollReq
	_ = json.NewDecoder(r.Body).Decode(&req) // body optional; defaults to trusted
	if req.Method == "password" && req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required for password bootstrap")
		return
	}
	res, err := h.svc.Enroll(r.Context(), p.SessionID, host, &p.UserID, EnrollParams{
		Method: req.Method, BootstrapUser: req.BootstrapUser, Password: req.Password, ViaJump: req.ViaJump,
	})
	if err != nil {
		// Surface the failed job so the UI can show which step failed.
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *handler) listJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	jobs, err := h.d.Store.ListEnrollmentJobs(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list jobs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (h *handler) getJob(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	job, err := h.d.Store.GetEnrollmentJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
