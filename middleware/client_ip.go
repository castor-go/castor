package middleware

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/castor-go/castor"
)

var clientIPCtxKey = &castor.ContextKey{Name: "clientIP"}

const xForwardedForHeader = "X-Forwarded-For"

// ClientIPFromHeader stores the client IP from a proxy-managed header.
// The last header value wins. If trustedIPPrefixes are provided, the header
// is only trusted when r.RemoteAddr is inside one of them; otherwise a valid
// RemoteAddr is used. Invalid input leaves the client IP unset.
func ClientIPFromHeader(trustedHeader string, trustedIPPrefixes ...string) func(http.Handler) http.Handler {
	header := http.CanonicalHeaderKey(trustedHeader)
	prefixes := mustParsePrefixes(trustedIPPrefixes)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var found netip.Addr
			if peer, untrusted := untrustedPeer(r, prefixes); untrusted {
				found = peer
			} else if values := r.Header.Values(header); len(values) > 0 {
				found, _ = parseHeaderAddr(values[len(values)-1])
			}
			if found.IsValid() {
				ctx := context.WithValue(r.Context(), clientIPCtxKey, found)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIPFromXFF stores the client IP from X-Forwarded-For, walking the
// chain right-to-left and skipping trusted proxy prefixes. A direct client
// outside the trusted prefixes is detected from r.RemoteAddr. At least one
// trusted prefix is required.
func ClientIPFromXFF(trustedIPPrefixes ...string) func(http.Handler) http.Handler {
	if len(trustedIPPrefixes) == 0 {
		panic("middleware.ClientIPFromXFF: at least one trusted IP prefix is required")
	}
	prefixes := mustParsePrefixes(trustedIPPrefixes)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var found netip.Addr
			if peer, untrusted := untrustedPeer(r, prefixes); untrusted {
				found = peer
			} else {
				walkXFF(r.Header.Values(xForwardedForHeader), func(value string) bool {
					ip, ok := parseHeaderAddr(value)
					if !ok {
						return true
					}
					if inAnyPrefix(ip, prefixes) {
						return false
					}
					found = ip
					return true
				})
			}
			if found.IsValid() {
				ctx := context.WithValue(r.Context(), clientIPCtxKey, found)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIPFromXFFTrustedProxies stores the client IP from X-Forwarded-For by
// counting trusted proxies. It reads only X-Forwarded-For and fails closed if
// the chain is too short.
func ClientIPFromXFFTrustedProxies(numTrustedProxies int) func(http.Handler) http.Handler {
	if numTrustedProxies < 1 {
		panic("middleware.ClientIPFromXFFTrustedProxies: numTrustedProxies must be >= 1")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			remaining := numTrustedProxies
			var entry string
			walkXFF(r.Header.Values(xForwardedForHeader), func(value string) bool {
				remaining--
				if remaining == 0 {
					entry = value
					return true
				}
				return false
			})
			if entry == "" {
				next.ServeHTTP(w, r)
				return
			}
			ip, ok := parseHeaderAddr(entry)
			if ok {
				ctx := context.WithValue(r.Context(), clientIPCtxKey, ip)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIPFromRemoteAddr stores the IP from r.RemoteAddr, for servers that
// are directly exposed to the internet.
func ClientIPFromRemoteAddr(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, ok := parseRemoteAddr(r.RemoteAddr)
		if ok {
			ctx := context.WithValue(r.Context(), clientIPCtxKey, ip)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// GetClientIP returns the client IP stored by a ClientIPFrom* middleware.
func GetClientIP(ctx context.Context) string {
	ip := GetClientIPAddr(ctx)
	if !ip.IsValid() {
		return ""
	}
	return ip.String()
}

// GetClientIPAddr returns the client IP as a netip.Addr.
func GetClientIPAddr(ctx context.Context) netip.Addr {
	return castor.GetValue[netip.Addr](ctx, clientIPCtxKey)
}

// walkXFF walks the merged X-Forwarded-For chain right-to-left without
// allocating. visit returns true to stop.
func walkXFF(headers []string, visit func(entry string) bool) {
	for _, header := range slices.Backward(headers) {
		for header != "" {
			var entry string
			if i := strings.LastIndexByte(header, ','); i >= 0 {
				entry, header = header[i+1:], header[:i]
			} else {
				entry, header = header, ""
			}
			entry = strings.TrimSpace(entry)
			if entry != "" && visit(entry) {
				return
			}
		}
	}
}

func mustParsePrefixes(cidrs []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, len(cidrs))
	for i, cidr := range cidrs {
		prefixes[i] = netip.MustParsePrefix(cidr)
	}
	return prefixes
}

func untrustedPeer(r *http.Request, prefixes []netip.Prefix) (netip.Addr, bool) {
	if len(prefixes) == 0 {
		return netip.Addr{}, false
	}
	peer, ok := parseRemoteAddr(r.RemoteAddr)
	if !ok {
		return netip.Addr{}, true
	}
	return peer, !inAnyPrefix(peer.WithZone(""), prefixes)
}

func parseRemoteAddr(remoteAddr string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

func inAnyPrefix(ip netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func parseHeaderAddr(value string) (netip.Addr, bool) {
	ip, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap().WithZone(""), true
}
