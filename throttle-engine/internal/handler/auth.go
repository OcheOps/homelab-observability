package handler

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"github.com/oche/homelab-observability/throttle-engine/internal/config"
)

// BasicAuth wraps an http.Handler with HTTP Basic Authentication.
// The /healthz and /api/v1/alert endpoints are exempt (Prometheus and Alertmanager need them).
func BasicAuth(cfg *config.AuthConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks and alert webhook (internal services)
		if r.URL.Path == "/healthz" || r.URL.Path == "/api/v1/alert" {
			next.ServeHTTP(w, r)
			return
		}

		if cfg.Username == "" || cfg.Password == "" {
			next.ServeHTTP(w, r)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Throttle Engine"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Constant-time comparison to prevent timing attacks
		userHash := sha256.Sum256([]byte(user))
		passHash := sha256.Sum256([]byte(pass))
		expectedUserHash := sha256.Sum256([]byte(cfg.Username))
		expectedPassHash := sha256.Sum256([]byte(cfg.Password))

		userMatch := subtle.ConstantTimeCompare(userHash[:], expectedUserHash[:]) == 1
		passMatch := subtle.ConstantTimeCompare(passHash[:], expectedPassHash[:]) == 1

		if !userMatch || !passMatch {
			w.Header().Set("WWW-Authenticate", `Basic realm="Throttle Engine"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
