package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"html/template"
	"net/http"
	"strings"

	"github.com/msoedov/secondorder/internal/templates"
)

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
