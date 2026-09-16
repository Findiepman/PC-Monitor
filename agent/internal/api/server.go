// Package api serves the REST endpoints, the live websocket and the embedded
// frontend.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/findiepman/dashd/internal/audit"
	"github.com/findiepman/dashd/internal/auth"
	"github.com/findiepman/dashd/internal/provider"
)

const cookieName = "dashd_session"

type Server struct {
	Hostname       string
	InsecureCookie bool
	Auth           *auth.Authenticator
	Registry       *provider.Registry
	Hub            *Hub
	Audit          *audit.Log
	Static         fs.FS
	Log            *slog.Logger
}

type ctxKey struct{}

type caller struct {
	token string
	user  string
	ip    string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.authed(s.logout))
	mux.HandleFunc("GET /api/session", s.authed(s.session))
	mux.HandleFunc("GET /api/meta", s.authed(s.meta))
	mux.HandleFunc("GET /api/units", s.authed(s.units))
	mux.HandleFunc("POST /api/units/{id}/{action}", s.authed(s.action))
	mux.HandleFunc("GET /api/audit", s.authed(s.audit))
	mux.HandleFunc("GET /ws", s.authed(s.ws))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "No such endpoint.")
	})
	mux.Handle("/", s.spa())
	return securityHeaders(sameOrigin(mux))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// sameOrigin rejects state-changing requests whose Origin doesn't match the
// Host they were sent to. SameSite=Strict already covers this in modern
// browsers; this is the second lock.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			origin := r.Header.Get("Origin")
			u, err := url.Parse(origin)
			if origin == "" || err != nil || u.Host != r.Host {
				writeError(w, http.StatusForbidden, "Request came from another site and was blocked.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authed(h func(http.ResponseWriter, *http.Request, caller)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Sign in to continue.")
			return
		}
		sess, ok := s.Auth.Session(c.Value)
		if !ok {
			writeError(w, http.StatusUnauthorized, "Your session ended. Sign in again.")
			return
		}
		h(w, r, caller{token: c.Value, user: sess.User, ip: clientIP(r)})
	}
}

// clientIP trusts CF-Connecting-IP only when the request arrived over
// loopback, i.e. through cloudflared on the same machine.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
			return cf
		}
	}
	return host
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Send username, password and code as JSON.")
		return
	}
	ip := clientIP(r)
	token, sess, err := s.Auth.Login(ip, body.Username, body.Password, strings.TrimSpace(body.Code))
	switch {
	case errors.Is(err, auth.ErrLocked):
		s.Log.Warn("login locked out", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "Too many failed attempts. Wait 15 minutes and try again.")
		return
	case errors.Is(err, auth.ErrInvalid):
		s.Log.Warn("login failed", "ip", ip, "user", body.Username)
		writeError(w, http.StatusUnauthorized, "Username, password or code is wrong.")
		return
	case err != nil:
		s.Log.Error("login error", "err", err)
		writeError(w, http.StatusInternalServerError, "Login is misconfigured on the server. Check the dashd logs.")
		return
	}
	s.Log.Info("login", "ip", ip, "user", sess.User)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  sess.Expires,
		HttpOnly: true,
		Secure:   !s.InsecureCookie,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"user": sess.User, "expires": sess.Expires})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, c caller) {
	s.Auth.Logout(c.token)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: !s.InsecureCookie, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) session(w http.ResponseWriter, r *http.Request, c caller) {
	sess, _ := s.Auth.Session(c.token)
	writeJSON(w, http.StatusOK, map[string]any{"user": sess.User, "expires": sess.Expires})
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request, c caller) {
	type prov struct {
		Name  string `json:"name"`
		Label string `json:"label"`
	}
	var provs []prov
	for _, p := range s.Registry.Providers() {
		provs = append(provs, prov{p.Name(), p.Label()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"hostname": s.Hostname, "providers": provs})
}

func (s *Server) units(w http.ResponseWriter, r *http.Request, c caller) {
	writeJSON(w, http.StatusOK, s.Hub.Units())
}

func (s *Server) action(w http.ResponseWriter, r *http.Request, c caller) {
	id, action := r.PathValue("id"), r.PathValue("action")
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	err := s.Registry.Do(ctx, id, action)
	entry := audit.Entry{Time: time.Now(), User: c.user, IP: c.ip, Unit: id, Action: action, OK: err == nil}
	if err != nil {
		entry.Error = err.Error()
	}
	// Refused requests for unknown units or actions aren't worth recording.
	if !errors.Is(err, provider.ErrUnknownUnit) && !errors.Is(err, provider.ErrBadAction) {
		if aerr := s.Audit.Append(entry); aerr != nil {
			s.Log.Error("audit write failed", "err", aerr)
		}
		s.Hub.Broadcast("audit", entry)
	}
	s.Hub.RefreshUnits()

	switch {
	case errors.Is(err, provider.ErrUnknownUnit):
		writeError(w, http.StatusNotFound, "No unit called "+id+".")
	case errors.Is(err, provider.ErrBadAction):
		writeError(w, http.StatusBadRequest, action+" isn't something dashd can do.")
	case err != nil:
		s.Log.Warn("action failed", "unit", id, "action", action, "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		s.Log.Info("action", "unit", id, "action", action, "user", c.user)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request, c caller) {
	writeJSON(w, http.StatusOK, s.Audit.Recent(100))
}

// spa serves the built frontend, falling back to index.html for client routes.
func (s *Server) spa() http.Handler {
	files := http.FileServerFS(s.Static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if p != "" {
			if st, err := fs.Stat(s.Static, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(s.Static, "index.html")
		if err != nil {
			http.Error(w, "The frontend isn't built. Run `npm run build` in web/ and rebuild dashd.", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
