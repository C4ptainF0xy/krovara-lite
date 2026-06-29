package httpx

import (
	"net/http"
	"strings"
)

func CORS(allowed []string) func(http.Handler) http.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		if o = strings.TrimSpace(o); o != "" {
			set[o] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if _, ok := set[origin]; ok {
					h := w.Header()
					h.Set("Access-Control-Allow-Origin", origin)
					h.Set("Access-Control-Allow-Credentials", "true")
					h.Add("Vary", "Origin")
					if r.Method == http.MethodOptions {

						h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
						if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
							h.Set("Access-Control-Allow-Headers", reqHeaders)
						} else {
							h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
						}
						h.Set("Access-Control-Max-Age", "600")
						w.WriteHeader(http.StatusNoContent)
						return
					}
				} else if r.Method == http.MethodOptions {

					w.WriteHeader(http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
