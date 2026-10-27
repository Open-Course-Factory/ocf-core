package services

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrAiEndpointInvalid is returned when an AI endpoint is not an https URL to
// a public host: the server POSTs the organization's key to it, so anything
// else is a server-side request forgery waiting to happen.
var ErrAiEndpointInvalid = errors.New("AI endpoint must be an https URL to a public host")

// sharedAddressSpace (RFC 6598, carrier-grade NAT) is neither private nor
// public by netip's reckoning, and some clouds serve metadata from it.
var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

// isPublicAddr reports whether the AI client may connect to addr: no
// loopback, private, link-local (which holds 169.254.169.254, the cloud
// metadata address), unspecified, multicast or shared address space.
func isPublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsValid() &&
		!addr.IsLoopback() &&
		!addr.IsPrivate() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsInterfaceLocalMulticast() &&
		!addr.IsMulticast() &&
		!addr.IsUnspecified() &&
		!sharedAddressSpace.Contains(addr)
}

// validateAiEndpoint is the save-time check, so a bad URL fails early with
// a clear error. It does not resolve hostnames: the dialer in
// NewAiProviderClient re-checks every address at connect time, which is what
// actually stops DNS rebinding.
func validateAiEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return ErrAiEndpointInvalid
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return ErrAiEndpointInvalid
	}
	if addr, err := netip.ParseAddr(host); err == nil && !isPublicAddr(addr) {
		return ErrAiEndpointInvalid
	}
	return nil
}

// NewAiProviderClient builds the HTTP client Generate uses. Every connection
// is checked against allowAddr after DNS resolution, redirects are never
// followed (a 3xx is answered as a provider failure) and no proxy is used,
// since a proxy would make the connect-time check meaningless. rootCAs nil
// means the system roots; tests pass their own along with a looser allowAddr.
func NewAiProviderClient(allowAddr func(netip.Addr) bool, rootCAs *x509.CertPool) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			addrPort, err := netip.ParseAddrPort(address)
			if err != nil || !allowAddr(addrPort.Addr()) {
				return fmt.Errorf("refused to connect to non-public address %s", address)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: 90 * time.Second,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         dialer.DialContext,
			TLSClientConfig:     &tls.Config{RootCAs: rootCAs, MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout: 10 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
