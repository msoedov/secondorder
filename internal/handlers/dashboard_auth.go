package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/msoedov/mesa/internal/templates"
)

// clientIP returns the best-guess remote address, honoring X-Forwarded-For
// and X-Real-IP when present (e.g. behind a reverse proxy).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// browserName extracts a coarse browser label from a User-Agent string.
func browserName(ua string) string {
	switch {
	case ua == "":
		return "-"
	case strings.Contains(ua, "Edg/"):
		return "Edge"
	case strings.Contains(ua, "OPR/") || strings.Contains(ua, "Opera"):
		return "Opera"
	case strings.Contains(ua, "Firefox/"):
		return "Firefox"
	case strings.Contains(ua, "Chrome/"):
		return "Chrome"
	case strings.Contains(ua, "Safari/"):
		return "Safari"
	case strings.Contains(ua, "curl/"):
		return "curl"
	case strings.Contains(ua, "Wget/"):
		return "wget"
	default:
		return "other"
	}
}

// AccessLog logs every dashboard request with the client IP, method and path.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		// \x1b[36m = cyan, \x1b[0m = reset -- tint passes the message through.
		slog.Info("\x1b[36mui\x1b[0m",
			"ip", clientIP(r),
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"ms", time.Since(start).Milliseconds(),
			"browser", browserName(r.Header.Get("User-Agent")),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// GenerateDashboardToken creates a cryptographically random 32-byte hex token.
func GenerateDashboardToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

var authTmpl = template.Must(template.New("auth").Parse(templates.AuthHTML()))

// DashboardAuth wraps an http.Handler, requiring a valid token via query param or cookie.
// If token is empty, auth is disabled and all requests pass through.
// API routes (/api/) are excluded -- they use their own agent-key auth.
func DashboardAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	cookieName := "so_dash_token"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for API and webhook routes (they have their own auth)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		// Check query param first -- allows one-click URL login
		if q := r.URL.Query().Get("token"); q != "" {
			if subtle.ConstantTimeCompare([]byte(q), []byte(token)) == 1 {
				// Set session cookie so user doesn't need the param on every request
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    token,
					Path:     "/",
					HttpOnly: true,
					Secure:   r.TLS != nil,
					SameSite: http.SameSiteLaxMode,
				})
				// Strip token from URL and redirect to clean path
				clean := *r.URL
				q := clean.Query()
				q.Del("token")
				clean.RawQuery = q.Encode()
				http.Redirect(w, r, clean.String(), http.StatusFound)
				return
			}
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}

		// Check cookie
		if c, err := r.Cookie(cookieName); err == nil {
			if subtle.ConstantTimeCompare([]byte(c.Value), []byte(token)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}

		// No valid credential -- render login page
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		authTmpl.Execute(w, nil)
	})
}
