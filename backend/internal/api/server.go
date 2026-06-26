// Package api wires the HTTP router, middleware, and module handlers.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/fleet-terminal/backend/internal/app"
	"github.com/fleet-terminal/backend/internal/auth"
	"github.com/fleet-terminal/backend/internal/bootstrap"
	"github.com/fleet-terminal/backend/internal/config"
	"github.com/fleet-terminal/backend/internal/hosts"
	"github.com/fleet-terminal/backend/internal/metrics"
	"github.com/fleet-terminal/backend/internal/store"
)

// Server holds shared dependencies for HTTP handlers. Module handlers are
// attached in registerRoutes; each milestone extends that surface.
type Server struct {
	Cfg     *config.Config
	DB      *pgxpool.Pool
	Log     *slog.Logger
	Version string

	Store *store.Store
	Auth  *auth.Service

	router chi.Router
}

// NewServer constructs a Server and builds its router.
func NewServer(cfg *config.Config, db *pgxpool.Pool, log *slog.Logger, version string) *Server {
	st := store.New(db)
	s := &Server{
		Cfg: cfg, DB: db, Log: log, Version: version,
		Store: st,
		Auth:  auth.NewService(st, cfg, log),
	}
	s.router = s.buildRouter()
	return s
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler { return s.router }

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.recoverer)
	r.Use(s.metricsMW)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{s.Cfg.PublicURL, "http://localhost:5173", "http://localhost:8080"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Operational endpoints (unauthenticated).
	r.Get("/health", s.handleHealth)
	r.Get("/ready", s.handleReady)
	r.Get("/version", s.handleVersion)
	r.Handle("/metrics", promhttp.Handler())

	// Versioned API surface. Module routers mount here.
	r.Route("/api/v1", func(api chi.Router) {
		s.registerRoutes(api)
	})

	return r
}

// registerRoutes is the single extension point where module handlers attach.
// Each milestone mounts its module here.
func (s *Server) registerRoutes(r chi.Router) {
	r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"pong": "ok"})
	})

	deps := &app.Deps{Store: s.Store, Cfg: s.Cfg, Log: s.Log, Auth: s.Auth}

	// M2 — first-run wizard + authentication.
	bootstrap.NewHandler(s.Store, s.Cfg).Mount(r)
	auth.NewHandler(s.Auth).Mount(r)

	// M3 — host inventory.
	hosts.Mount(r, deps)

	// Additional modules (admin, audit, approvals, terminal, …) mount here as
	// they are integrated.
	s.mountModules(r, deps)
}

// mountModules is the integration seam for modules delivered by the orchestrated
// build. It is defined in modules.go so generated wiring stays isolated.
func (s *Server) mountModules(r chi.Router, d *app.Deps) {
	_ = r
	_ = d
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 2*time.Second)
	defer cancel()
	if err := s.DB.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.Version})
}

// recoverer converts panics into 500s and logs them.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.Log.Error("panic recovered", "err", rec, "path", r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// metricsMW records request counts and latency.
func (s *Server) metricsMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		metrics.HTTPRequests.WithLabelValues(r.Method, route, strconv.Itoa(ww.Status())).Inc()
		metrics.HTTPDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
