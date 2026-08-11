package httpserver

import (
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization, ngrok-skip-browser-warning")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("[%s] %s %s ip=%s -> %d (%dms)",
			time.Now().UTC().Format(time.RFC3339Nano),
			r.Method, r.URL.RequestURI(), clientIP(r), rw.code, time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			logAuthFail(r, "missing_token")
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Unauthorized: Missing Authorization Bearer token",
			})
			return
		}
		if s.cfg.AuthToken == "" {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Server: SYNC_PASSWORD not set in .env",
			})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token != s.cfg.AuthToken {
			logAuthFail(r, "invalid_token")
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Unauthorized: Invalid token",
			})
			return
		}
		next(w, r)
	}
}

// logAuthFail writes a stable line for fail2ban (no token/password).
func logAuthFail(r *http.Request, reason string) {
	log.Printf("AUTH_FAIL ip=%s method=%s path=%s reason=%s",
		clientIP(r), r.Method, r.URL.Path, reason)
}

// clientIP: X-Real-IP → first X-Forwarded-For → RemoteAddr (host only).
func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return stripHostPort(ip)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = xff[:i]
		}
		if ip := strings.TrimSpace(xff); ip != "" {
			return stripHostPort(ip)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func stripHostPort(ip string) string {
	if host, _, err := net.SplitHostPort(ip); err == nil {
		return host
	}
	return ip
}
