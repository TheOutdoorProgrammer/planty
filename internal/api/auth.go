package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

func bearerAuth(token string, next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		presented := ""
		if strings.HasPrefix(header, prefix) && len(header) > len(prefix) {
			presented = header[len(prefix):]
		}
		got := sha256.Sum256([]byte(presented))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", `Bearer realm="planty"`)
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":      "A valid Planty bearer token is required.",
				"code":       "unauthorized",
				"request_id": w.Header().Get(requestIDHeader),
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
