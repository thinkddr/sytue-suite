// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package dominio prueba que quien da de alta un dominio propio lo controla —un
// TXT en `_sytue.<dominio>` con un valor que solo conoce quien lo pidió— y
// normaliza el nombre que la gente escribe o que llega de un borde.
//
// VIVÍA DENTRO DE SYTUE ID (`services/id/internal/domain/onboarding.go`), que lo
// usa para el dominio de la organización y los de correo. Sytue Web lo
// necesita para los dominios de sus webs, así que se movió aquí: la plomería no
// se copia, se importa. ID conserva alias con los nombres de siempre.
package dominio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
)

// recordHost es la etiqueta bajo la que se pide el TXT de verificación
// (_sytue.acme.com): no toca el TXT raíz del cliente, donde viven SPF y demás.
const recordHost = "_sytue"

// recordPrefix precede al valor aleatorio en el TXT, para que quien mire la
// zona DNS sepa de quién es el registro.
const recordPrefix = "sytue-verification="

// RecordName es el nombre EXACTO del registro TXT que el cliente debe crear.
func RecordName(host string) string { return recordHost + "." + host }

// RecordValue es el valor EXACTO del registro TXT para un token dado.
func RecordValue(token string) string { return recordPrefix + token }

// NewDomainToken genera el valor aleatorio (128 bits) del registro de
// verificación. Solo tiene que ser inadivinable: prueba que quien lo publica en
// la zona DNS es quien lo pidió aquí.
func NewDomainToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("dominio: leer aleatoriedad: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Resolver es el subconjunto de *net.Resolver que usamos para verificar dominios.
// Se abstrae para inyectar un resolver de prueba en test (sin red); en producción
// es net.DefaultResolver y la consulta es DNS de verdad.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// DNSStatus es el resultado REAL de consultar el DNS del cliente. (No se llama
// Status: en Sytue ID ese nombre ya es el estado de ciclo de vida de usuarios y
// organizaciones.)
type DNSStatus string

const (
	// DomainVerified: el TXT existe y su valor coincide. Es el ÚNICO estado que
	// da el dominio por verificado.
	DomainVerified DNSStatus = "verified"
	// DomainPending: el registro todavía no se ve (no existe o aún no propaga).
	DomainPending DNSStatus = "pending"
	// DomainMismatch: hay TXT en ese nombre pero ninguno es el nuestro (típico:
	// se copió a medias, o se pegó en otro dominio).
	DomainMismatch DNSStatus = "mismatch"
	// DomainUnreachable: no hemos podido consultar el DNS (timeout, servidor
	// caído). NO es culpa del registro del cliente y no se le dice que lo revise.
	DomainUnreachable DNSStatus = "unreachable"
)

// DNSCheck es el resultado de la comprobación, con lo que se ENCONTRÓ de
// verdad (para poder enseñárselo al usuario cuando no coincide).
type DNSCheck struct {
	Status DNSStatus
	Found  []string
}

// CheckDomainTXT consulta el TXT de _sytue.<host> y compara con el valor
// esperado. No escribe nada ni asume nada: devuelve lo que el DNS respondió.
//
// Un TXT largo llega troceado en varias cadenas; el nuestro cabe de sobra en una
// (255 bytes), pero se concatenan por si algún proveedor lo parte igualmente.
func CheckDomainTXT(ctx context.Context, res Resolver, host, token string) DNSCheck {
	txts, err := res.LookupTXT(ctx, RecordName(host))
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			// NXDOMAIN/NODATA: el registro aún no está publicado (o no propaga).
			return DNSCheck{Status: DomainPending}
		}
		return DNSCheck{Status: DomainUnreachable}
	}
	want := RecordValue(token)
	found := make([]string, 0, len(txts))
	for _, raw := range txts {
		v := strings.TrimSpace(strings.Trim(strings.TrimSpace(raw), `"`))
		if v == "" {
			continue
		}
		if strings.EqualFold(v, want) {
			return DNSCheck{Status: DomainVerified}
		}
		found = append(found, v)
	}
	if len(found) == 0 {
		return DNSCheck{Status: DomainPending}
	}
	return DNSCheck{Status: DomainMismatch, Found: found}
}

// ErrNoValido envuelve todo lo que Normalizar rechaza.
var ErrNoValido = errors.New("dominio: no válido")

// Normalizar limpia lo que la gente pega de verdad (https://, una ruta, un
// correo entero, el puerto, mayúsculas, el punto final) y devuelve el nombre de
// host en minúsculas. NO quita `www.`: para una web, `www.acme.com` y `acme.com`
// son dos hosts distintos. (Quien sí lo quita es el alta de Sytue ID, que
// verifica la organización y no un host.)
//
// Rechaza lo que no es un nombre DNS de al menos dos etiquetas y las IP
// literales: una extensión que no empieza por letra es una IP para el navegador
// (`1.2.3.4`, pero también `127.1` o `0x7f.1`), y ningún dominio de nivel
// superior empieza por cifra.
//
// ponytail: un nombre con acentos (`panadería.es`) se rechaza en vez de pasarlo
// a punycode; el relevo es `idna.Lookup.ToASCII` (golang.org/x/net ya está en
// go.mod) cuando el alta de dominios de Pages lo pida.
func Normalizar(raw string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.Index(d, "://"); i >= 0 {
		d = d[i+3:]
	}
	if i := strings.IndexAny(d, "/?#"); i >= 0 {
		d = d[:i]
	}
	if i := strings.LastIndexByte(d, '@'); i >= 0 {
		d = d[i+1:] // un correo entero, o un usuario delante del host
	}
	if i := strings.IndexByte(d, ':'); i >= 0 {
		d = d[:i] // puerto (y una IPv6 entre corchetes se queda en «[» y cae abajo)
	}
	d = strings.Trim(strings.TrimSpace(d), ".")
	if d == "" {
		return "", fmt.Errorf("%w: vacío", ErrNoValido)
	}
	if len(d) > 253 {
		return "", fmt.Errorf("%w: demasiado largo", ErrNoValido)
	}
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("%w: falta la extensión", ErrNoValido)
	}
	for _, l := range labels {
		if !etiquetaValida(l) {
			return "", fmt.Errorf("%w: parte inválida: %q", ErrNoValido, l)
		}
	}
	if tld := labels[len(labels)-1]; tld[0] < 'a' || tld[0] > 'z' {
		return "", fmt.Errorf("%w: %q es una IP, no un dominio", ErrNoValido, d)
	}
	return d, nil
}

// etiquetaValida: 1 a 63 caracteres [a-z0-9-], sin guion en los extremos.
func etiquetaValida(l string) bool {
	if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for _, r := range l {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
