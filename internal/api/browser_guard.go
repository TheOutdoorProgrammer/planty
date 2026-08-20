package api

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// browserWriteGuard keeps the deliberately unauthenticated LAN API from being
// a write target for arbitrary web pages. Native clients do not send Origin or
// Sec-Fetch-Site and remain unaffected.
func browserWriteGuard(next http.Handler) http.Handler {
	allowed := allowedBrowserHosts(os.Getenv("PLANTY_ALLOWED_BROWSER_HOSTS"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isUnsafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		if r.ContentLength != 0 && !safeWriteContentType(r.Header.Get("Content-Type")) {
			http.Error(w, "writes require JSON, an image, or multipart form data", http.StatusUnsupportedMediaType)
			return
		}

		if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
			http.Error(w, "cross-site browser writes are not allowed", http.StatusForbidden)
			return
		}

		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		originURL, err := url.Parse(origin)
		if err != nil || originURL.Hostname() == "" {
			http.Error(w, "invalid browser origin", http.StatusForbidden)
			return
		}

		requestHost := hostOnly(r.Host)
		originHost := strings.ToLower(originURL.Hostname())
		if requestHost == "" || !strings.EqualFold(requestHost, originHost) {
			http.Error(w, "browser origin does not match this Planty host", http.StatusForbidden)
			return
		}
		if !browserHostAllowed(requestHost, allowed) {
			http.Error(w, "browser writes to this host are not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func safeWriteContentType(raw string) bool {
	media := strings.ToLower(strings.TrimSpace(strings.Split(raw, ";")[0]))
	switch media {
	case "application/json", "multipart/form-data", "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

func allowedBrowserHosts(raw string) map[string]bool {
	out := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		if host := hostOnly(strings.TrimSpace(item)); host != "" {
			out[host] = true
		}
	}
	return out
}

func browserHostAllowed(host string, explicit map[string]bool) bool {
	if explicit[host] || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}

func hostOnly(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(raw, "[]")
}
