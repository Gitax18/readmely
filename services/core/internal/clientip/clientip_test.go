package clientip

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func req(remote string, xff ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	r.RemoteAddr = remote
	for _, v := range xff {
		r.Header.Add("X-Forwarded-For", v)
	}
	return r
}

func TestAddr(t *testing.T) {
	web := New([]netip.Prefix{netip.MustParsePrefix("172.28.0.10/32"), netip.MustParsePrefix("10.1.0.0/16")})
	tests := []struct {
		name string
		r    *http.Request
		want string
	}{
		{"direct caller uses the socket", req("198.51.100.4:5000"), "198.51.100.4"},
		{"an untrusted caller cannot set the header", req("198.51.100.4:5000", "203.0.113.9"), "198.51.100.4"},
		{"behind the web server", req("172.28.0.10:40000", "203.0.113.9"), "203.0.113.9"},
		{"a forged prefix behind the web server is ignored", req("172.28.0.10:40000", "1.2.3.4, 203.0.113.9"), "203.0.113.9"},
		{"several trusted hops are skipped", req("172.28.0.10:40000", "1.2.3.4, 203.0.113.9, 10.1.2.3"), "203.0.113.9"},
		{"a second header counts as later hops", req("172.28.0.10:40000", "1.2.3.4", "203.0.113.9"), "203.0.113.9"},
		{"garbage from the client falls back to the last trusted hop", req("172.28.0.10:40000", "203.0.113.9, not-an-ip"), "172.28.0.10"},
		{"empty header behind the proxy uses the proxy", req("172.28.0.10:40000", ""), "172.28.0.10"},
		{"no header behind the proxy uses the proxy", req("172.28.0.10:40000"), "172.28.0.10"},
		{"only trusted hops ends at the left-most of them", req("172.28.0.10:40000", "10.1.9.9"), "10.1.9.9"},
		{"IPv4-mapped IPv6 is plain IPv4", req("[::ffff:198.51.100.4]:5000"), "198.51.100.4"},
		{"IPv6 behind the proxy", req("172.28.0.10:40000", "2001:db8::42"), "2001:db8::42"},
		{"a port in the header is not an address", req("172.28.0.10:40000", "203.0.113.9:1234"), "172.28.0.10"},
		{"an unparseable socket address", req("nonsense"), "invalid IP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := web.Addr(tt.r).String(); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNoTrustedProxiesMeansTheHeaderIsNeverRead(t *testing.T) {
	r := New(nil)
	if got := r.Addr(req("172.28.0.10:40000", "203.0.113.9")).String(); got != "172.28.0.10" {
		t.Fatalf("got %s", got)
	}
}

func TestOnlyTheLastSixteenHopsAreRead(t *testing.T) {
	web := New([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	h := "203.0.113.9"
	for range 20 {
		h += ", 10.0.0.1"
	}
	if got := web.Addr(req("10.0.0.2:1", h)).String(); got != "10.0.0.1" {
		t.Fatalf("a header of trusted hops longer than the limit should end at a trusted hop, got %s", got)
	}
}

func TestRateKey(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.9":          "203.0.113.9",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::1": "2001:db8:1:2::/64",
	} {
		if got := RateKey(netip.MustParseAddr(in)); got != want {
			t.Errorf("RateKey(%s) = %s, want %s", in, got, want)
		}
	}
	if RateKey(netip.Addr{}) != "unknown" {
		t.Error("the zero address must map to one shared key")
	}
}
