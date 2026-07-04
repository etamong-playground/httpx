// Package ip provides client IP extraction for HTTP requests.
// It implements a right-anchored X-Forwarded-For strategy to prevent
// spoofing by untrusted clients prepending arbitrary IPs.
package ip

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP extracts the client IP address from request headers.
// It prefers the rightmost valid X-Forwarded-For entry to prevent spoofing,
// then falls back to X-Real-IP and finally the request's RemoteAddr.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(parts[i])
			if ip != "" && net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
