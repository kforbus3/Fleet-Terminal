// Package api wires the HTTP router, middleware, and module handlers.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/fleet-terminal/backend/internal/admin"
	"github.com/fleet-terminal/backend/internal/app"
	"github.com/fleet-terminal/backend/internal/approvals"
	"github.com/fleet-terminal/backend/internal/auditapi"
	"github.com/fleet-terminal/backend/internal/auth"
	"github.com/fleet-terminal/backend/internal/bootstrap"
	"github.com/fleet-terminal/backend/internal/ca"
	"github.com/fleet-terminal/backend/internal/certificates"
	"github.com/fleet-terminal/backend/internal/config"
	"github.com/fleet-terminal/backend/internal/hosts"
	"github.com/fleet-terminal/backend/internal/identity"
	"github.com/fleet-terminal/backend/internal/metrics"
	"github.com/fleet-terminal/backend/internal/sessionsapi"
	"github.com/fleet-terminal/backend/internal/sshgw"
	"github.com/fleet-terminal/backend/internal/store"
	"github.com/fleet-terminal/backend/internal/terminal"
)

// Server holds shared dependencies for HTTP handlers. Module handlers are
// attached in registerRoutes; each milestone extends that surface.
type Server struct {
	Cfg     *config.Config
	DB      *pgxpool.Pool
	Log     *slog.Logger
	Version string

	Store   *store.Store
	Auth    *auth.Service
	CA      *ca.CA
	Issuer  *identity.Issuer
	Gateway *sshgw.Gateway

	router chi.Router
}

// NewServer constructs a Server and builds its router.
func NewServer(cfg *config.Config, db *pgxpool.Pool, log *slog.Logger, version string) *Server {
	st := store.New(db)
	authSvc := auth.NewService(st, cfg, log)
	caMgr := ca.New(st, cfg)
	vault := identity.NewVault()
	issuer := identity.NewIssuer(st, caMgr, cfg, log, vault)
	gateway := sshgw.New(cfg, log, vault)

	s := &Server{
		Cfg: cfg, DB: db, Log: log, Version: version,
		Store:   st,
		Auth:    authSvc,
		CA:      caMgr,
		Issuer:  issuer,
		Gateway: gateway,
	}

	// On login, mint an ephemeral SSH identity bound to the session; on logout,
	// zeroize the key and revoke its certificates.
	authSvc.SetSessionHooks(
		func(ctx context.Context, userID, sessionID uuid.UUID, username string) {
			principals := dedupe([]string{"fleet", username})
			if _, err := issuer.Issue(ctx, sessionID, userID, username, principals); err != nil {
				log.Warn("issue ephemeral identity", "err", err)
			}
		},
		func(ctx context.Context, _ uuid.UUID, sessionID uuid.UUID, _ string) {
			issuer.DestroySession(ctx, sessionID)
		},
	)

	s.router = s.buildRouter()
	return s
}

// InitBackground initializes the CA and starts background schedulers. Call once
// after construction, before serving.
func (s *Server) InitBackground(ctx context.Context) error {
	if err := s.CA.EnsureUserCA(ctx); err != nil {
		return err
	}
	go s.renewalLoop(ctx)
	go s.reaperLoop(ctx)
	return nil
}

// reaperLoop periodically expires elapsed just-in-time access grants.
func (s *Server) reaperLoop(ctx context.Context) {
	deps := &app.Deps{Store: s.Store, Cfg: s.Cfg, Log: s.Log, Auth: s.Auth}
	t := time.NewTicker(1 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			approvals.Reaper(ctx, deps)
		}
	}
}

func (s *Server) renewalLoop(ctx context.Context) {
	t := time.NewTicker(1 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Issuer.RenewExpiring(ctx)
		}
	}
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

	deps := &app.Deps{Store: s.Store, Cfg: s.Cfg, Log: s.Log, Auth: s.Auth, CA: s.Issuer, Gateway: s.Gateway}

	// M2 — first-run wizard + authentication.
	bootstrap.NewHandler(s.Store, s.Cfg).Mount(r)
	auth.NewHandler(s.Auth).Mount(r)

	// M3 — host inventory.
	hosts.Mount(r, deps)

	// M4 — certificate authority & lifecycle.
	certificates.Mount(r, deps, s.CA)

	// M5 — SSH gateway browser terminal.
	terminal.Mount(r, deps, s.Gateway)

	// Orchestrated modules (admin, audit, sessions, approvals).
	admin.Mount(r, deps)
	auditapi.Mount(r, deps)
	sessionsapi.Mount(r, deps)
	approvals.Mount(r, deps)
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
