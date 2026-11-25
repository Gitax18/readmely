// Package clientip finds the address of the client behind the web server (audit F13).
//
// The browser talks to the web app, whose /api/* rewrite forwards to the API, so every request
// reaches the API from the web server's address. Rate limits keyed on that address would be one
// limit for the whole site. The web server passes the browser's address in X-Forwarded-For.
//
// The header is believed only when the connection itself comes from a trusted proxy. It is read
// from the right: each proxy appends the address it received the request from, so the right-most
// entry that is not a trusted proxy is the client as seen by our own infrastructure. Entries to
// its left were written by the client and can say anything, so they are never used.
package clientip

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// maxHops bounds how much of a long header is read.
const maxHops = 16

// Resolver knows which peers are trusted proxies.
type Resolver struct {
	trusted []netip.Prefix
}

// New returns a Resolver that believes X-Forwarded-For only from these prefixes.
func New(trusted []netip.Prefix) *Resolver { return &Resolver{trusted: trusted} }

func (r *Resolver) isTrusted(a netip.Addr) bool {
	for _, p := range r.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// IsProxy reports whether a is one of the trusted proxies. When Addr returns such an address, the
// request came from our own web server with no visitor address attached (its own server-side
// calls), not from a visitor.
func (r *Resolver) IsProxy(a netip.Addr) bool { return a.IsValid() && r.isTrusted(a) }

// Addr returns the client address for req. It never fails: without a usable peer address it
// returns the zero Addr, which callers treat as one shared "unknown" client.
func (r *Resolver) Addr(req *http.Request) netip.Addr {
	peer := peerAddr(req.RemoteAddr)
	if !peer.IsValid() || !r.isTrusted(peer) {
		return peer
	}
	hops := forwardedHops(req.Header.Values("X-Forwarded-For"))
	for i := len(hops) - 1; i >= 0 && len(hops)-i <= maxHops; i-- {
		a, err := netip.ParseAddr(hops[i])
		if err != nil {
			// A trusted proxy never writes garbage, so this came from the client. Stop here and
			// use the last address our own infrastructure vouched for.
			return peer
		}
		a = a.Unmap()
		if !r.isTrusted(a) {
			return a
		}
		peer = a
	}
	return peer
}

func peerAddr(remote string) netip.Addr {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

// forwardedHops joins every X-Forwarded-For header (a proxy may add a second one) into one list.
func forwardedHops(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// RateKey is the part of an address that rate limits count against. An IPv6 user usually
// controls a whole /64, so limiting single IPv6 addresses would be easy to evade.
func RateKey(a netip.Addr) string {
	if !a.IsValid() {
		return "unknown"
	}
	if a.Is4() {
		return a.String()
	}
	p, _ := a.Prefix(64)
	return p.String()
}
