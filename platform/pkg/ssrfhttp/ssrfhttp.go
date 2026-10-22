// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ssrfhttp es la barrera SSRF (Server-Side Request Forgery) reutilizable
// de la suite: cualquier servicio que abra una conexión de red a un destino que
// aporta el usuario (una URL REST externa, el host de un PostgreSQL externo, un
// feed ICS…) debe encauzarla por aquí. Implementa defensa en DOBLE CAPA,
// clean-room sobre net/http y net:
//
//	Barrera 1 (ValidateURL / ValidateHost): ANTES de conectar, valida el esquema
//	  http/https y RESUELVE el host; rechaza si CUALQUIER IP resuelta es
//	  privada/loopback/link-local/ULA/CGNAT/multicast/unspecified (metadata
//	  169.254.169.254, ::1, 10.x, fc00::/7, 100.64.0.0/10, …).
//	Barrera 2 (SafeDialContext): en el momento de CONECTAR, vuelve a resolver,
//	  valida TODAS las IPs y conecta a la IP EXACTA validada. Cierra la ventana
//	  TOCTOU / DNS-rebinding entre (1) y el connect.
//
// El dialer (barrera 2) sirve tanto al *http.Transport como al DialFunc de pgx
// (conexión a PostgreSQL externo): la misma comprobación de IP cubre HTTP y TCP
// crudo. Sin proxy (no exfiltrar el destino por HTTP(S)_PROXY); redirects
// acotados que RE-validan cada salto por la barrera 1. Nunca alcanza un destino
// interno, ni por redirect ni por rebinding.
//
// Nota de origen: replica el patrón probado de services/calendar (suscripciones
// ICS), extraído aquí sin acoplarlo a ningún dominio para que REST y PostgreSQL
// (services/docs, conectores de datos) lo compartan. calendar mantiene su copia
// hasta su migración; este paquete es el hogar común de aquí en adelante.
package ssrfhttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"
)

// Errores públicos: destino bloqueado por la barrera SSRF y URL/host inválidos.
var (
	// ErrBlocked: el destino resuelve a una IP no enrutable públicamente.
	ErrBlocked = errors.New("ssrfhttp: destino no permitido")
	// ErrInvalidURL: esquema no http/https, host vacío o URL no parseable.
	ErrInvalidURL = errors.New("ssrfhttp: URL inválida")
)

const (
	// dialTimeout acota el establecimiento de conexión (resolución + connect).
	dialTimeout = 10 * time.Second
	// resolveTimeout acota la resolución DNS de la barrera 1.
	resolveTimeout = 5 * time.Second
)

// IsBlockedIP es el denylist SSRF: reservadas/privadas/locales. Reusa los
// helpers de net.IP (IsPrivate cubre RFC1918 + ULA fc00::/7) y añade CGNAT
// 100.64.0.0/10. nil => bloqueada (no resolvimos nada usable).
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true // 100.64.0.0/10 (CGNAT, RFC 6598): enrutable internamente
	}
	return ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast()
}

// politicaIP es la política de bloqueo EFECTIVA; en producción es IsBlockedIP. Las
// barreras la consultan a través de aquí para que los tests herméticos —cuyo
// destino es loopback (httptest escucha en 127.0.0.1)— puedan relajarla sin tocar
// la lógica de producción.
//
// ATÓMICA, y no una `var` pelada, porque la LEEN goroutines que este paquete no
// controla: `SafeDialContext` corre dentro del pool de conexiones de
// `http.Transport`, y una conexión que quedó abierta de un test anterior sigue
// marcando el número mientras el siguiente instala su política. El detector de
// carreras lo cazó el 2026-09-05 en `TestRedirectAPrivadaSeCorta`: escritura
// desde el test, lectura desde el dialer de una conexión ajena. En producción
// nadie escribe después del arranque, así que la carrera era solo de los tests
// —pero el `-race` del CI no distingue, y habría enrojecido a destiempo—.
var politicaIP atomic.Pointer[func(net.IP) bool]

func init() {
	produccion := IsBlockedIP
	politicaIP.Store(&produccion)
}

// ipBlocked responde a la pregunta de las tres barreras: ¿esta IP está vetada?
func ipBlocked(ip net.IP) bool { return (*politicaIP.Load())(ip) }

// SetIPPolicyForTest sustituye TEMPORALMENTE la política de bloqueo de IPs por
// `policy` y devuelve una función que restaura la de producción (IsBlockedIP).
// EXCLUSIVO para tests herméticos que apuntan a loopback/privado (httptest); JAMÁS
// debe usarse en producción.
//
// El cambio es atómico, así que ya no hay carrera con lecturas concurrentes; lo
// que sigue sin ser es TRANSACCIONAL: dos tests en paralelo se pisan la política
// igual, y el segundo `restore()` deja la del primero. Los tests de este paquete
// no corren en paralelo entre sí.
func SetIPPolicyForTest(policy func(net.IP) bool) (restore func()) {
	prev := politicaIP.Swap(&policy)
	return func() { politicaIP.Store(prev) }
}

// SafeDialContext es la barrera 2: re-resuelve el host, valida TODAS sus IPs y
// conecta a la primera IP validada (no re-resuelve por el SO). Su firma coincide
// con http.Transport.DialContext y con pgconn.DialFunc, así que la MISMA barrera
// cubre HTTP y la conexión TCP a PostgreSQL externo.
func SafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("ssrfhttp: sin IPs para %q", host)
	}
	for _, ipa := range ips {
		if ipBlocked(ipa.IP) {
			return nil, ErrBlocked
		}
	}
	// Conectar a la IP concreta ya validada (cierra el rebinding: el SO no
	// vuelve a resolver un nombre que podría apuntar ahora a un destino interno).
	base := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}
	return base.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

// Transport construye el *http.Transport con la barrera 2 (SafeDialContext) y sin
// proxy (no exfiltrar el destino por HTTP(S)_PROXY).
func Transport() *http.Transport {
	return &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   dialTimeout,
		ExpectContinueTimeout: time.Second,
		DialContext:           SafeDialContext,
	}
}

// NewClient construye un *http.Client SSRF-safe: barrera 2 (dialer) + redirects
// acotados que RE-validan cada salto por la barrera 1 (un redirect a un destino
// privado se corta igual que la petición original). timeout <= 0 degrada a 15s;
// maxRedirects < 0 degrada a 5.
func NewClient(timeout time.Duration, maxRedirects int) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if maxRedirects < 0 {
		maxRedirects = 5
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: Transport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("ssrfhttp: demasiados redirects")
			}
			return ValidateURL(req.Context(), req.URL.String())
		},
	}
}

// ValidateURL es la barrera 1 para una URL completa: esquema http/https, host
// presente y TODAS las IPs del host públicas. Se ejecuta ANTES de la petición y
// en cada redirect.
func ValidateURL(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: esquema %q no permitido", ErrInvalidURL, u.Scheme)
	}
	return ValidateHost(ctx, u.Hostname())
}

// ValidateHost es la barrera 1 para un host suelto (sin esquema): resuelve y
// exige que todas sus IPs sean públicas. Es el pre-chequeo de una conexión TCP a
// un servicio externo (p. ej. el host de un PostgreSQL): la barrera 2 (dialer)
// la vuelve a aplicar en el connect.
func ValidateHost(ctx context.Context, host string) error {
	if host == "" {
		return fmt.Errorf("%w: host vacío", ErrInvalidURL)
	}
	// Un literal IP se valida directo; un nombre se resuelve.
	if ip := net.ParseIP(host); ip != nil {
		if ipBlocked(ip) {
			return ErrBlocked
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("%w: no se resuelve el host: %w", ErrInvalidURL, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("%w: host sin IPs", ErrInvalidURL)
	}
	for _, ipa := range ips {
		if ipBlocked(ipa.IP) {
			return ErrBlocked
		}
	}
	return nil
}
