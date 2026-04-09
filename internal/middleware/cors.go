package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CORSMiddleware sets Access-Control headers when ALLOWED_ORIGINS is set.
// Use "*" for development or comma-separated origins, e.g. https://my-app.vercel.app
func CORSMiddleware(next http.Handler) http.Handler {
	origins := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))
	if origins == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqOrigin := r.Header.Get("Origin")
		if origins == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if reqOrigin != "" {
			for _, o := range strings.Split(origins, ",") {
				if strings.TrimSpace(o) == reqOrigin {
					w.Header().Set("Access-Control-Allow-Origin", reqOrigin)
					break
				}
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
