package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSPAHandler_ServesIndexHTML(t *testing.T) {
	handler := newSPAHandler("")
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/app/")
	if err != nil {
		t.Fatalf("GET /app/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /app/: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/html; charset=utf-8")
	}
}

func TestSPAHandler_ServesSubRoutes(t *testing.T) {
	handler := newSPAHandler("")
	server := httptest.NewServer(handler)
	defer server.Close()

	for _, path := range []string{"/app/droplets", "/app/castellarius", "/app/doctor"} {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status = %d, want %d", path, resp.StatusCode, http.StatusOK)
		}

		ct := resp.Header.Get("Content-Type")
		if ct != "text/html; charset=utf-8" {
			t.Errorf("GET %s: Content-Type = %q, want %q", path, ct, "text/html; charset=utf-8")
		}
	}
}

func TestSPAHandler_RedirectsAppRoot(t *testing.T) {
	mux := http.NewServeMux()
	spa := newSPAHandler("")
	mux.Handle("/app/", spa)
	mux.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app/", http.StatusMovedPermanently)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(server.URL + "/app")
	if err != nil {
		t.Fatalf("GET /app: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("GET /app: status = %d, want %d", resp.StatusCode, http.StatusMovedPermanently)
	}

	loc := resp.Header.Get("Location")
	if loc != "/app/" {
		t.Errorf("Location = %q, want %q", loc, "/app/")
	}
}

func TestSPAHandler_InjectsAuthMetaTag(t *testing.T) {
	handler := newSPAHandler("secret-key")
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/app/")
	if err != nil {
		t.Fatalf("GET /app/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /app/: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}

	if len(body) == 0 {
		t.Fatal("response body is empty")
	}

	if !contains(string(body), `meta name="cistern-auth" content="required"`) {
		t.Error("index.html should contain auth meta tag when apiKey is configured")
	}
}

func TestSPAHandler_NoAuthMetaTagWithoutKey(t *testing.T) {
	handler := newSPAHandler("")
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/app/")
	if err != nil {
		t.Fatalf("GET /app/: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}

	if contains(string(body), "cistern-auth") {
		t.Error("index.html should NOT contain auth meta tag when no apiKey is configured")
	}
}

func TestSPAHandler_SecurityHeadersOnIndexHTML(t *testing.T) {
	handler := newSPAHandler("")
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/app/")
	if err != nil {
		t.Fatalf("GET /app/: %v", err)
	}
	defer resp.Body.Close()

	for _, tc := range []struct{ header, want string }{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
	} {
		got := resp.Header.Get(tc.header)
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.header, got, tc.want)
		}
	}

	csp := resp.Header.Get("Content-Security-Policy")
	if !contains(csp, "connect-src 'self' ws://") {
		t.Errorf("CSP connect-src should restrict WebSocket to same host, got: %s", csp)
	}
	if contains(csp, "connect-src 'self' ws:") && !contains(csp, "ws://") {
		t.Errorf("CSP connect-src should not allow ws: wildcard, got: %s", csp)
	}
}

func TestSPAHandler_SecurityHeadersOnAssets(t *testing.T) {
	handler := newSPAHandler("")
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/app/assets/index-bUUc_DK2.js")
	if err != nil {
		t.Fatalf("GET /app/assets/: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options on asset = %q, want %q", resp.Header.Get("X-Content-Type-Options"), "nosniff")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSanitizeCSPHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"localhost:5737", "localhost:5737"},
		{"127.0.0.1:8080", "127.0.0.1:8080"},
		{"example.com", "example.com"},
		{"host-with-hyphens.example.com", "host-with-hyphens.example.com"},
		{"evil<script>", "evilscript"},
		{"host;injection", "hostinjection"},
		{"", ""},
	}
	for _, tc := range tests {
		got := sanitizeCSPHost(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeCSPHost(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestAuthMiddleware_WSPeekExempt(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := apiAuthMiddleware(okHandler, "test-key")

	for _, path := range []string{"/ws/aqueducts/virgo/peek", "/ws/aqueducts/marcia/peek"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want %d (WS peek should be exempt from middleware auth)", path, w.Code, http.StatusOK)
		}
	}
}

func TestAuthMiddleware_WSNonPeekRequiresAuth(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := apiAuthMiddleware(okHandler, "test-key")

	for _, path := range []string{"/ws/aqueducts/virgo", "/ws/aqueducts/virgo/stream", "/ws/aqueducts/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s: status = %d, want %d (non-peek WS paths should require auth)", path, w.Code, http.StatusUnauthorized)
		}
	}
}

func TestAuthMiddleware_SPAExempt(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := apiAuthMiddleware(okHandler, "test-key")

	for _, path := range []string{"/app", "/app/", "/app/droplets", "/app/assets/index.js"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want %d (SPA should be exempt from auth)", path, w.Code, http.StatusOK)
		}
	}
}

func TestAuthMiddleware_BearerToken(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := apiAuthMiddleware(okHandler, "correct-key")

	tests := []struct {
		name string
		path string
		auth string
		want int
	}{
		{"no auth on API", "/api/droplets", "", http.StatusUnauthorized},
		{"wrong key", "/api/droplets", "Bearer wrong-key", http.StatusUnauthorized},
		{"correct key", "/api/droplets", "Bearer correct-key", http.StatusOK},
		{"no auth on SSE", "/api/dashboard/events", "", http.StatusUnauthorized},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestAuthMiddleware_QueryToken(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := apiAuthMiddleware(okHandler, "correct-key")

	tests := []struct {
		name string
		path string
		want int
	}{
		{"valid token in query", "/api/dashboard/events?token=correct-key", http.StatusOK},
		{"invalid token in query", "/api/dashboard/events?token=wrong-key", http.StatusUnauthorized},
		{"empty token in query", "/api/dashboard/events?token=", http.StatusUnauthorized},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestAuthMiddleware_OptionsExempt(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := apiAuthMiddleware(okHandler, "test-key")

	req := httptest.NewRequest(http.MethodOptions, "/api/droplets", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("OPTIONS /api/droplets: status = %d, want %d (CORS preflight should be exempt)", w.Code, http.StatusOK)
	}
}

func TestSPAHandler_ServesUnknownSubRoutesAsIndexHTML(t *testing.T) {
	handler := newSPAHandler("")
	server := httptest.NewServer(handler)
	defer server.Close()

	for _, path := range []string{"/app/droplets/nonexistent-droplet-id", "/app/castellarious-typo"} {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status = %d, want %d", path, resp.StatusCode, http.StatusOK)
		}

		ct := resp.Header.Get("Content-Type")
		if ct != "text/html; charset=utf-8" {
			t.Errorf("GET %s: Content-Type = %q, want %q", path, ct, "text/html; charset=utf-8")
		}
	}
}

func TestSPAHandler_AssetsPathRequiresTrailingSlash(t *testing.T) {
	handler := newSPAHandler("")
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/app/")
	if err != nil {
		t.Fatalf("GET /app/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /app/: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestSPAHandler_IndexHTMLHasNoCaching(t *testing.T) {
	handler := newSPAHandler("")
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/app/")
	if err != nil {
		t.Fatalf("GET /app/: %v", err)
	}
	defer resp.Body.Close()

	cc := resp.Header.Get("Cache-Control")
	if cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", cc, "no-cache")
	}
}
