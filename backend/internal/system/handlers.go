// Package system exposes operational endpoints: background-job status and
// database backup.
package system

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/fleet-terminal/backend/internal/app"
	"github.com/fleet-terminal/backend/internal/jobs"
	"github.com/fleet-terminal/backend/internal/models"
)

// Mount attaches system routes (admin-gated).
func Mount(r chi.Router, d *app.Deps, reg *jobs.Registry) {
	h := &handler{d: d, reg: reg}
	r.Group(func(pr chi.Router) {
		pr.Use(d.Auth.RequireAuth)
		pr.With(d.Auth.RequirePermission("System.Configure")).Get("/system/jobs", h.jobs)
	})
}

type handler struct {
	d   *app.Deps
	reg *jobs.Registry
}

func (h *handler) jobs(w http.ResponseWriter, r *http.Request) {
	enrollJobs, err := h.d.Store.ListEnrollmentJobs(r.Context(), 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list enrollment jobs")
		return
	}
	if enrollJobs == nil {
		enrollJobs = []models.EnrollmentJob{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schedulers":      h.reg.Snapshot(),
		"enrollmentJobs":  enrollJobs,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
