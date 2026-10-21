// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// ErrExternalOrigin rejects discovery or credentials crossing an unapproved origin.
var ErrExternalOrigin = errors.New("api: external HTTPS origin rejected")

// PublicHTTPSClient extends the native transport for user-selected public services.
// Origins are exact, redirects and proxies are disabled, and default dialing pins
// public DNS results. An injected client is trusted application/test plumbing.
func PublicHTTPSClient(base *http.Client, origins ...string) (*http.Client, error) {
	allowed := make(map[string]struct{}, len(origins))
	for _, raw := range origins {
		origin, err := PublicHTTPSOrigin(raw)
		if err != nil {
			return nil, err
		}
		allowed[origin] = struct{}{}
	}
	var client http.Client
	if base != nil {
		client = *base
	} else {
		transport, ok := Transporte.(*http.Transport)
		if !ok {
			return nil, ErrExternalOrigin
		}
		t := transport.Clone()
		t.Proxy = nil
		t.DialContext = dialPublicHTTPS
		client.Transport = t
	}
	if client.Transport == nil {
		client.Transport = Transporte
	}
	client.Transport = originTransport{base: client.Transport, origins: allowed}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client, nil
}

// PublicHTTPSOrigin accepts an HTTPS authority without credentials or local IPs.
func PublicHTTPSOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || strings.HasSuffix(u.Hostname(), ".") || strings.ContainsAny(u.Hostname(), "%\x00 ") {
		return "", ErrExternalOrigin
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return "", ErrExternalOrigin
	}
	if ip, err := netip.ParseAddr(host); err == nil && !publicAddress(ip) {
		return "", ErrExternalOrigin
	}
	return "https://" + strings.ToLower(u.Host), nil
}

type originTransport struct {
	base    http.RoundTripper
	origins map[string]struct{}
}

func (t originTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	origin, err := PublicHTTPSOrigin(r.URL.String())
	if err != nil {
		return nil, ErrExternalOrigin
	}
	if _, ok := t.origins[origin]; !ok {
		return nil, ErrExternalOrigin
	}
	return t.base.RoundTrip(r)
}

func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !netip.MustParsePrefix("100.64.0.0/10").Contains(ip) && !netip.MustParsePrefix("198.18.0.0/15").Contains(ip)
}

func dialPublicHTTPS(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrExternalOrigin
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, ErrExternalOrigin
	}
	for _, ip := range ips {
		if !publicAddress(ip) {
			return nil, ErrExternalOrigin
		}
	}
	dialer := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, ErrExternalOrigin
}
