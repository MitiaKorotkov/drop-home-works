package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"IPv4", "203.0.113.5:4321", "203.0.113.5"},
		{"IPv6 is cut to /64", "[2001:db8:1:2:3:4:5:6]:443", "2001:db8:1:2::/64"},
		{"another address from the same /64", "[2001:db8:1:2:ffff::1]:443", "2001:db8:1:2::/64"},
		{"another /64", "[2001:db8:1:3::1]:443", "2001:db8:1:3::/64"},
		{"IPv4 written as IPv6", "[::ffff:203.0.113.5]:80", "203.0.113.5"},
		{"no port", "garbage", "garbage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = clientIPFrom(r.Context())
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr

			loadClientIP(next).ServeHTTP(httptest.NewRecorder(), req)

			if got != tt.want {
				t.Errorf("RemoteAddr %q: client IP in context = %q, want %q", tt.remoteAddr, got, tt.want)
			}
		})
	}
}
