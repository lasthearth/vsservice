package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/rs/cors"
)

// TestCorsRejectsLookalikeOrigins pins the regression that made the previous
// origin list exploitable: rs/cors matches a `*` wildcard as prefix+suffix, so
// "http://localhost*" also matched "http://localhost.attacker.com". Combined
// with AllowCredentials that let an attacker-registered domain read every
// response on behalf of a logged-in user.
func TestCorsRejectsLookalikeOrigins(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		{"https://lasthearth.ru", true},
		{"https://map.lasthearth.ru", true},
		{"http://localhost:3000", true},
		{"http://127.0.0.1:5173", true},

		{"http://localhost.attacker.com", false},
		{"http://localhostevil.io", false},
		{"http://127.0.0.1.attacker.com", false},
		{"https://lasthearth.ru.attacker.com", false},
		{"http://0.0.0.0.attacker.com", false},
		{"https://attacker.com", false},
	}

	// AppEnv "dev" so the dev origins are in play too — the strictest case is
	// prod, and anything rejected in dev is rejected in prod as well.
	s := &Server{c: config.Config{
		AppEnv:             "dev",
		CorsAllowedOrigins: []string{"https://lasthearth.ru", "https://*.lasthearth.ru"},
		CorsDevOrigins:     []string{"http://localhost:3000", "http://127.0.0.1:5173"},
	}}

	handler := cors.New(cors.Options{
		AllowedOrigins:   s.corsAllowedOrigins(),
		AllowedMethods:   []string{http.MethodGet},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}).Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/anything", nil)
			req.Header.Set("Origin", tt.origin)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			allowed := rec.Header().Get("Access-Control-Allow-Origin") != ""
			if allowed != tt.want {
				t.Errorf("origin %q: allowed=%v, want %v (ACAO=%q)",
					tt.origin, allowed, tt.want, rec.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

// TestCorsDevOriginsExcludedInProd checks that localhost is not reachable from a
// prod deployment.
func TestCorsDevOriginsExcludedInProd(t *testing.T) {
	s := &Server{c: config.Config{
		AppEnv:             "prod",
		CorsAllowedOrigins: []string{"https://lasthearth.ru"},
		CorsDevOrigins:     []string{"http://localhost:3000"},
	}}

	got := s.corsAllowedOrigins()
	if len(got) != 1 || got[0] != "https://lasthearth.ru" {
		t.Errorf("want only the prod origin, got %v", got)
	}
}
