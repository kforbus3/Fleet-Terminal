package auth

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fleet-terminal/backend/internal/models"
)

// Handler exposes auth HTTP endpoints.
type Handler struct{ svc *Service }

// NewHandler builds the auth Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount attaches auth routes under the given router.
func (h *Handler) Mount(r chi.Router) {
	r.Post("/auth/login", h.login)
	r.Post("/auth/refresh", h.refresh)
	r.Group(func(pr chi.Router) {
		pr.Use(h.svc.RequireAuth)
		pr.Post("/auth/logout", h.logout)
		pr.Get("/auth/me", h.me)
		pr.Post("/auth/change-password", h.changePassword)
	})
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ip, ua := clientMeta(r)
	u, err := h.svc.Authenticate(r.Context(), req.Username, req.Password)
	if err != nil {
		_ = h.svc.store.RecordAuthEvent(r.Context(), models.AuthEvent{
			Username: req.Username, Event: "login_failure", IP: ip, UserAgent: ua,
			Detail: map[string]any{"reason": err.Error()},
		})
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	tokens, err := h.svc.CreateSession(r.Context(), u, ip, ua, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	_ = h.svc.store.RecordAuthEvent(r.Context(), models.AuthEvent{
		UserID: &u.ID, Username: u.Username, Event: "login_success", IP: ip, UserAgent: ua,
	})
	_, _ = h.svc.store.AppendAudit(r.Context(), models.AuditEvent{
		ActorID: &u.ID, ActorName: u.Username, Action: "auth.login", IP: ip,
	})
	h.setAuthCookies(w, tokens)
	u.Roles, _ = h.svc.store.UserRoleNames(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"accessToken":     tokens.Access,
		"accessExpiresAt": tokens.AccessExpiry,
		"csrfToken":       tokens.CSRF,
		"user":            u,
		"mustChangePassword": u.MustChangePw,
	})
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	rc, err := r.Cookie(RefreshCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "no refresh token")
		return
	}
	// The session id is conveyed via a companion cookie set at login.
	sc, err := r.Cookie("fleet_sid")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "no session")
		return
	}
	sid, err := uuid.Parse(sc.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "bad session")
		return
	}
	tokens, err := h.svc.Refresh(r.Context(), sid, rc.Value)
	if err != nil {
		h.clearAuthCookies(w)
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	h.setAuthCookies(w, tokens)
	writeJSON(w, http.StatusOK, map[string]any{
		"accessToken":     tokens.Access,
		"accessExpiresAt": tokens.AccessExpiry,
		"csrfToken":       tokens.CSRF,
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	p := MustPrincipal(r)
	if p != nil {
		_ = h.svc.Logout(r.Context(), p.SessionID)
		_, _ = h.svc.store.AppendAudit(r.Context(), models.AuditEvent{
			ActorID: &p.UserID, ActorName: p.Username, Action: "auth.logout",
		})
	}
	h.clearAuthCookies(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p := MustPrincipal(r)
	u, err := h.svc.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	u.Roles, _ = h.svc.store.UserRoleNames(r.Context(), u.ID)
	u.Groups, _ = h.svc.store.UserGroupNames(r.Context(), u.ID)
	perms := make([]string, 0, len(p.Permissions))
	for k := range p.Permissions {
		perms = append(perms, k)
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "permissions": perms, "isSuperAdmin": u.IsSuperAdmin})
}

type changePwReq struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	p := MustPrincipal(r)
	var req changePwReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	hash, err := h.svc.store.GetPasswordHash(r.Context(), p.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if ok, _ := VerifyPassword(req.CurrentPassword, hash); !ok {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	if err := DefaultPolicy.Validate(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	newHash, err := HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash failed")
		return
	}
	if err := h.svc.store.SetPasswordHash(r.Context(), p.UserID, newHash); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	_, _ = h.svc.store.AppendAudit(r.Context(), models.AuditEvent{
		ActorID: &p.UserID, ActorName: p.Username, Action: "auth.password_change",
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_changed"})
}

// --- cookie + helper plumbing ---

func (h *Handler) setAuthCookies(w http.ResponseWriter, t *Tokens) {
	secure := h.svc.cfg.CookieSecure
	domain := h.svc.cfg.CookieDomain
	http.SetCookie(w, &http.Cookie{
		Name: RefreshCookie, Value: t.Refresh, Path: "/api/v1/auth", Domain: domain,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
		Expires: time.Now().Add(h.svc.cfg.RefreshTokenTTL),
	})
	http.SetCookie(w, &http.Cookie{
		Name: "fleet_sid", Value: t.Session.ID.String(), Path: "/api/v1/auth", Domain: domain,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
		Expires: time.Now().Add(h.svc.cfg.RefreshTokenTTL),
	})
	// CSRF cookie is readable by JS (double-submit), so HttpOnly is false.
	http.SetCookie(w, &http.Cookie{
		Name: CSRFCookie, Value: t.CSRF, Path: "/", Domain: domain,
		HttpOnly: false, Secure: secure, SameSite: http.SameSiteStrictMode,
		Expires: time.Now().Add(h.svc.cfg.RefreshTokenTTL),
	})
}

func (h *Handler) clearAuthCookies(w http.ResponseWriter) {
	for _, c := range []struct{ name, path string }{
		{RefreshCookie, "/api/v1/auth"}, {"fleet_sid", "/api/v1/auth"}, {CSRFCookie, "/"},
	} {
		http.SetCookie(w, &http.Cookie{Name: c.name, Value: "", Path: c.path, MaxAge: -1})
	}
}

func clientMeta(r *http.Request) (ip, ua string) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host, r.UserAgent()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
