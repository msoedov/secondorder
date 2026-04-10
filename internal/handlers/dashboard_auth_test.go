package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardAuth(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	token := "test-secret-token-abc123"

	tests := []struct {
		name       string
		token      string
		path       string
		query      string
		cookie     string
		wantCode   int
		wantCookie bool
	}{
		{
			name:     "disabled auth passes through",
			token:    "",
			path:     "/dashboard",
			wantCode: 200,
		},
		{
			name:     "no credentials shows login",
			token:    token,
			path:     "/dashboard",
			wantCode: 401,
		},
		{
			name:       "valid query param sets cookie and redirects",
			token:      token,
			path:       "/dashboard",
			query:      "token=" + token,
			wantCode:   302,
			wantCookie: true,
		},
		{
			name:     "invalid query param returns forbidden",
			token:    token,
			path:     "/dashboard",
			query:    "token=wrong",
			wantCode: 403,
		},
		{
			name:     "valid cookie passes through",
			token:    token,
			path:     "/dashboard",
			cookie:   token,
			wantCode: 200,
		},
		{
			name:     "invalid cookie shows login",
			token:    token,
			path:     "/dashboard",
			cookie:   "wrong",
			wantCode: 401,
		},
		{
			name:     "API routes bypass auth",
			token:    token,
			path:     "/api/v1/issues/TEST-1",
			wantCode: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := DashboardAuth(tt.token, ok)
			url := tt.path
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest("GET", url, nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "so_dash_token", Value: tt.cookie})
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("got status %d, want %d", w.Code, tt.wantCode)
			}
			if tt.wantCookie {
				found := false
				for _, c := range w.Result().Cookies() {
					if c.Name == "so_dash_token" && c.Value == token {
						found = true
					}
				}
				if !found {
					t.Error("expected so_dash_token cookie to be set")
				}
			}
		})
	}
}

func TestDashboardAuth_LoginPageRendersTemplate(t *testing.T) {
	handler := DashboardAuth("secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	for _, want := range []string{"Dashboard Access", "Access Token", "auth.html", "Second Order"} {
		if want == "auth.html" {
			// just verify it's not the old inline HTML (no "auth.html" literal expected)
			continue
		}
		if !strings.Contains(body, want) {
			t.Errorf("login page missing %q", want)
		}
	}
}

func TestGenerateDashboardToken(t *testing.T) {
	t1 := GenerateDashboardToken()
	t2 := GenerateDashboardToken()
	if len(t1) != 64 {
		t.Errorf("token length = %d, want 64", len(t1))
	}
	if t1 == t2 {
		t.Error("two generated tokens should not be equal")
	}
}
