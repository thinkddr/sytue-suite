// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package clientip resuelve la IP real de quien hace una petición cuando hay
// proxies delante, y solo cree a los que son nuestros.
//
// VIVÍA DENTRO DE SYTUE ID, y por eso el resto de la suite no lo tenía: el
// middleware de registro de `platform/pkg/httpx` leía la PRIMERA entrada de
// X-Forwarded-For sin mirar quién conectaba, así que cualquiera podía envenenar
// la columna de IP del registro de peticiones de TODOS los servicios. Se movió
// aquí entero (auditoría del 2026-09-21, docs/security.md §6.3): la plomería no
// se copia, se importa.
package clientip

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// TrustedProxies son las redes cuyo `X-Forwarded-For` se cree.
//
// POR QUÉ EXISTE. La cabecera la escribe quien quiera: es texto que viaja en la
// petición. Leerla sin mirar de dónde viene convierte en falsificable TODO lo que
// cuelga de la IP del cliente:
//
//   - los buckets por IP del login y de /oauth2/* (una IP inventada por intento
//     es un límite de fuerza bruta que no limita nada);
//   - las condiciones por IP del Acceso Condicional (`block` desde fuera de la
//     red de la empresa se esquiva afirmando una IP de dentro);
//   - la IP del registro de auditoría y la ubicación del listado de sesiones, que
//     es justo lo que se mira cuando hay que reconstruir un incidente.
//
// CÓMO SE RESUELVE. Solo se cree la cabecera si quien CONECTA es un proxy de la
// lista, y dentro de ella se recorre de DERECHA a IZQUIERDA: cada salto añade la
// dirección de quien le habló, así que la primera entrada que no sea un proxy
// nuestro es el cliente real. Lo que el cliente haya inventado queda a su
// izquierda y no se mira. Las entradas de la izquierda son precisamente las que
// controla el atacante, y eran las que se leían antes.
// El valor CERO no es «no creer a nadie», sino «las redes privadas»: es el que
// tiene una Config construida a mano (tests, subcomandos), y ahí el defecto seguro
// y útil es el del despliegue real. Para no creer a nadie está NoTrustedProxies.
// errEntrada es lo que devuelve ParseTrustedProxies ante una lista mal escrita.
// Aquí no se importa el dominio de ningún servicio: cada uno lo envuelve con el
// suyo si le hace falta.
var errEntrada = errors.New("lista de proxies de confianza no válida")

type TrustedProxies struct {
	redes []netip.Prefix
	// explicita distingue el valor cero de una lista elegida a propósito —incluida
	// la lista VACÍA de `none`, que sí significa «a nadie»—.
	explicita bool
}

// redesPrivadas son loopback, RFC 1918, CGNAT (RFC 6598), enlace local y
// ULA/loopback de IPv6. Es el despliegue real —Traefik habla con el Pod desde
// dentro del clúster— y NO incluye ninguna red pública, así que una petición que
// llegue directa de internet nunca aporta su propio X-Forwarded-For.
var redesPrivadas = mustPrefixes(
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"100.64.0.0/10", "169.254.0.0/16",
	"::1/128", "fc00::/7", "fe80::/10",
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

// DefaultTrustedProxies es la lista por defecto (redes privadas): la del
// despliegue en Kubernetes detrás de Traefik.
func DefaultTrustedProxies() TrustedProxies {
	return TrustedProxies{redes: redesPrivadas, explicita: true}
}

// NoTrustedProxies no cree a nadie: la IP es siempre la de quien conecta. Es lo
// correcto cuando el servicio se publica sin proxy delante.
func NoTrustedProxies() TrustedProxies { return TrustedProxies{explicita: true} }

// Normalize deja una dirección como la quiere Postgres: sin puerto, sin
// corchetes de IPv6 y sin zona de enlace local. Devuelve "" si no es una IP.
//
// Vino de domain.NormalizeIP, que sigue existiendo como alias: `[::1]:54321`
// llegaba a la columna `inet` con corchetes y Postgres lo rechazaba con 22P02,
// que en el login por IPv6 —el caso NORMAL en local, porque localhost resuelve
// a ::1 antes que a 127.0.0.1— salía como un 500.
func Normalize(v string) string {
	if v == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(v); err == nil {
		v = h
	}
	v = strings.Trim(v, "[]")
	if i := strings.IndexByte(v, '%'); i >= 0 {
		v = v[:i]
	}
	if net.ParseIP(v) == nil {
		return ""
	}
	return v
}

// ParseTrustedProxies lee una lista separada por comas de CIDR o direcciones
// sueltas ("10.0.0.0/8, 192.0.2.7"). Una dirección suelta equivale a su /32 o
// /128. El valor "none" (o la cadena vacía) significa no creer a nadie.
func ParseTrustedProxies(csv string) (TrustedProxies, error) {
	out := TrustedProxies{explicita: true}
	csv = strings.TrimSpace(csv)
	if csv == "" || strings.EqualFold(csv, "none") {
		return out, nil
	}
	for _, raw := range strings.Split(csv, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if p, err := netip.ParsePrefix(raw); err == nil {
			out.redes = append(out.redes, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			return TrustedProxies{}, fmt.Errorf("%w: %q no es una IP ni un CIDR", errEntrada, raw)
		}
		out.redes = append(out.redes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

// efectivas resuelve el valor cero a las redes privadas.
func (t TrustedProxies) efectivas() []netip.Prefix {
	if t.explicita {
		return t.redes
	}
	return redesPrivadas
}

// trusts dice si esa dirección es uno de nuestros proxies.
func (t TrustedProxies) trusts(a netip.Addr) bool {
	a = a.Unmap()
	for _, red := range t.efectivas() {
		if red.Contains(a) {
			return true
		}
	}
	return false
}

// ClientIP resuelve la IP del cliente a partir de la dirección del peer TCP
// (r.RemoteAddr) y de la cabecera X-Forwarded-For. Devuelve "" cuando ninguna de
// las dos da una dirección legible, que es lo mismo que devolvía la versión
// anterior y lo que las columnas `inet` esperan como «no se sabe».
func (t TrustedProxies) ClientIP(remoteAddr, xff string) string {
	peer := Normalize(remoteAddr)
	if peer == "" {
		// Sin peer legible no hay forma de saber si la cabecera viene de un proxy
		// nuestro, así que no se usa: fail-closed.
		return ""
	}
	pa, err := netip.ParseAddr(peer)
	if err != nil || !t.trusts(pa) {
		return peer
	}
	// Peer de confianza: la entrada más a la derecha que NO sea otro proxy nuestro
	// es quien habló con el primer salto de la cadena.
	partes := strings.Split(xff, ",")
	for i := len(partes) - 1; i >= 0; i-- {
		ip := Normalize(strings.TrimSpace(partes[i]))
		if ip == "" {
			continue
		}
		a, err := netip.ParseAddr(ip)
		if err != nil {
			continue
		}
		if !t.trusts(a) {
			return ip
		}
	}
	// Cadena entera de proxies nuestros (o sin cabecera): el peer es lo mejor que hay.
	return peer
}

// Identifica dice si una IP que ha devuelto ClientIP es la de quien hace la petición
// y no la de uno de nuestros proxies. Cuando la cadena entera es nuestra —el túnel del
// piloto entrega sin la IP de la persona—, ClientIP devuelve el último proxy, y un
// límite por esa IP sería un límite para todo el mundo a la vez.
func (t TrustedProxies) Identifica(ip string) bool {
	a, err := netip.ParseAddr(ip)
	return err == nil && !t.trusts(a)
}

type ctxKeyTrustedProxies struct{}

// WithTrustedProxies deja la lista en el contexto de la petición para que la
// resuelvan los helpers de cada paquete (http, scim) sin arrastrar la
// configuración por catorce firmas.
func WithTrustedProxies(ctx context.Context, t TrustedProxies) context.Context {
	return context.WithValue(ctx, ctxKeyTrustedProxies{}, t)
}

// TrustedProxiesFrom recupera la lista del contexto. Sin ella —código que no pasa
// por el router, tests— se devuelve la de por defecto: creer a las redes privadas
// nunca es peor que creer a cualquiera, que es lo que se hacía antes.
func TrustedProxiesFrom(ctx context.Context) TrustedProxies {
	if t, ok := ctx.Value(ctxKeyTrustedProxies{}).(TrustedProxies); ok {
		return t
	}
	return DefaultTrustedProxies()
}
