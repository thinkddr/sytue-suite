// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package subscriptions obtiene, cachea y refresca calendarios ICS externos
// (el «Añadir calendario por URL»). El fetch remoto de un feed ICS es una
// petición HTTP a una URL que aporta el usuario: superficie SSRF de primer
// orden. Este fichero implementa la barrera SSRF en DOBLE CAPA (defensa en
// profundidad), clean-room sobre net/http:
//
//	Barrera 1 (validateURL): ANTES de la petición, valida esquema http/https y
//	  RESUELVE el host; rechaza si CUALQUIER IP resuelta es privada/loopback/
//	  link-local/ULA/CGNAT/multicast/unspecified (metadata 169.254.169.254, ::1,
//	  10.x, fc00::/7, ...).
//	Barrera 2 (dialer): en el momento de CONECTAR, vuelve a resolver, valida
//	  TODAS las IPs y conecta a la IP EXACTA validada. Cierra la ventana TOCTOU /
//	  DNS-rebinding entre (1) y el connect.
//
// Además: sin proxy (no exfiltrar el destino por HTTP(S)_PROXY), redirects
// acotados que RE-validan cada salto por el mismo dialer, timeout corto y cap
// del cuerpo. Nunca sigue file://; nunca alcanza un destino interno, ni por
// redirect. El contenido ICS se trata SIEMPRE como datos (se parsea, jamás se
// ejecuta).
package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

const (
	fetchTimeout      = 15 * time.Second
	fetchMaxRedirects = 5
	fetchUserAgent    = "Sytue-Calendar-ICS/1"
)

// ssrfSafeTransport construye el *http.Transport con la barrera 2 (el dialer que
// re-resuelve, valida TODAS las IPs y conecta a la ya validada). Sin proxy: no
// exfiltrar el destino por HTTP(S)_PROXY.
func ssrfSafeTransport() *http.Transport {
	base := &net.Dialer{Timeout: fetchTimeout, KeepAlive: fetchTimeout}
	return &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   fetchTimeout,
		ExpectContinueTimeout: time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("ssrf: sin IPs para %q", host)
			}
			for _, ipa := range ips {
				if ssrfhttp.IsBlockedIP(ipa.IP) {
					return nil, errors.New("ssrf: destino no permitido")
				}
			}
			// Conectar a la IP concreta ya validada (no re-resolver por el SO).
			return base.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
}

// newClient construye el cliente HTTP del refresco de suscripciones: barrera 2
// (dialer) + redirects acotados que RE-validan cada salto por la barrera 1 (un
// redirect a un destino privado se corta igual que la petición original).
func newClient() *http.Client {
	return &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= fetchMaxRedirects {
				return errors.New("ssrf: demasiados redirects")
			}
			return validateURL(req.Context(), req.URL.String())
		},
		Transport: ssrfSafeTransport(),
	}
}

// validateURL es la barrera 1: esquema http/https, host, y todas las IPs del
// host públicas. Se ejecuta ANTES de la petición (y en cada redirect).
func validateURL(ctx context.Context, rawURL string) error {
	if !domain.ValidSubscriptionURL(rawURL) {
		return domain.ErrInvalidURL
	}
	host := hostOf(rawURL)
	if host == "" {
		return domain.ErrInvalidURL
	}
	// Un literal IP se valida directo; un nombre se resuelve.
	if ip := net.ParseIP(host); ip != nil {
		if ssrfhttp.IsBlockedIP(ip) {
			return domain.ErrInvalidURL
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("%w: no se resuelve el host: %w", domain.ErrInvalidURL, err)
	}
	if len(ips) == 0 {
		return domain.ErrInvalidURL
	}
	for _, ipa := range ips {
		if ssrfhttp.IsBlockedIP(ipa.IP) {
			return domain.ErrInvalidURL
		}
	}
	return nil
}

// hostOf extrae el hostname de una URL.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
