package ip_test

import (
	"net/http"
	"testing"

	"github.com/etamong-playground/httpx/ip"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		xri        string
		remoteAddr string
		want       string
	}{
		{
			name:       "X-Forwarded-For single",
			xff:        "1.2.3.4",
			remoteAddr: "9.9.9.9:1234",
			want:       "1.2.3.4",
		},
		{
			name:       "X-Forwarded-For multiple takes rightmost valid",
			xff:        "1.2.3.4, 5.6.7.8",
			remoteAddr: "9.9.9.9:1234",
			want:       "5.6.7.8",
		},
		{
			name:       "X-Forwarded-For skips invalid entries",
			xff:        "spoofed, 5.6.7.8",
			remoteAddr: "9.9.9.9:1234",
			want:       "5.6.7.8",
		},
		{
			name:       "X-Real-IP fallback",
			xri:        "10.0.0.1",
			remoteAddr: "9.9.9.9:1234",
			want:       "10.0.0.1",
		},
		{
			name:       "X-Real-IP invalid falls back to RemoteAddr",
			xri:        "not-an-ip",
			remoteAddr: "9.9.9.9:1234",
			want:       "9.9.9.9",
		},
		{
			name:       "RemoteAddr with port",
			remoteAddr: "192.168.1.1:5000",
			want:       "192.168.1.1",
		},
		{
			name:       "RemoteAddr without port",
			remoteAddr: "192.168.1.1",
			want:       "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For takes priority over X-Real-IP",
			xff:        "1.1.1.1",
			xri:        "2.2.2.2",
			remoteAddr: "3.3.3.3:80",
			want:       "1.1.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{
				RemoteAddr: tt.remoteAddr,
				Header:     http.Header{},
			}
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				r.Header.Set("X-Real-IP", tt.xri)
			}
			got := ip.ClientIP(r)
			if got != tt.want {
				t.Errorf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
