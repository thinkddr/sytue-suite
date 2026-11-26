// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominio

import (
	"net/mail"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Límites de los campos con formato.
const (
	MaxLongURL      = 2048
	MaxLongCorreo   = 254 // RFC 5321
	MaxLongTelefono = 32
)

// normalizarFormato valida un texto con formato y devuelve su forma canónica. La
// interfaz lo pinta como enlace (`href`), así que aquí se cierra la puerta a todo lo
// que no sea http(s) o mailto/tel construidos por ella: un «javascript:» guardado en una
// celda sería un XSS para quien la pulse.
func normalizarFormato(t Tipo, s string) (string, bool) {
	if !utf8.ValidString(s) || tieneControl(s) {
		return "", false
	}
	switch t { //nolint:exhaustive // solo los tres tipos de texto con formato
	case TipoURL:
		return normalizarURL(s)
	case TipoCorreo:
		return normalizarCorreo(s)
	case TipoTelefono:
		return normalizarTelefono(s)
	}
	return "", false
}

// normalizarURL acepta http(s) con host; sin esquema («sytue.eu/x») supone https,
// que es lo que quiere decir quien lo escribe así.
func normalizarURL(s string) (string, bool) {
	if utf8.RuneCountInString(s) > MaxLongURL || strings.ContainsAny(s, " \\") {
		return "", false
	}
	if !strings.Contains(s, "://") {
		if strings.Contains(strings.SplitN(s, "/", 2)[0], ":") && !strings.Contains(s, ".") {
			return "", false // «javascript:alert(1)», «mailto:x»: un esquema, no un host
		}
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", false
	}
	host := u.Hostname()
	if host == "" || strings.HasPrefix(host, ".") || strings.Contains(host, "..") {
		return "", false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String(), true
}

// normalizarCorreo exige una dirección sola (sin nombre ni <>) con dominio con punto, y
// pone el dominio en minúsculas (la parte local la decide el servidor de correo).
func normalizarCorreo(s string) (string, bool) {
	if len(s) > MaxLongCorreo || strings.ContainsAny(s, " <>,;\"") {
		return "", false
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Name != "" || a.Address != s {
		return "", false
	}
	i := strings.LastIndexByte(s, '@')
	local, dominio := s[:i], strings.ToLower(s[i+1:])
	if local == "" || !strings.Contains(dominio, ".") || strings.HasPrefix(dominio, ".") || strings.HasSuffix(dominio, ".") {
		return "", false
	}
	return local + "@" + dominio, true
}

// normalizarTelefono admite lo que la gente escribe (+34 600 12 34 56, (91) 555-12-12)
// pero solo eso: dígitos, un «+» inicial y separadores; los espacios repetidos se
// funden. Entre 3 y 20 dígitos (E.164 son 15, más una extensión corta).
func normalizarTelefono(s string) (string, bool) {
	if utf8.RuneCountInString(s) > MaxLongTelefono {
		return "", false
	}
	digitos := 0
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digitos++
		case r == '+' && i == 0:
		case strings.ContainsRune(" -.()/", r):
		default:
			return "", false
		}
	}
	if digitos < 3 || digitos > 20 {
		return "", false
	}
	return strings.Join(strings.Fields(s), " "), true
}

// codigoMoneda dice si s tiene forma de código ISO 4217 (tres letras mayúsculas). No se
// coteja con la lista: una moneda nueva no debe exigir desplegar el servicio, y quien la
// pinta (Intl en la web) ya sabe qué hacer con un código que no conoce.
func codigoMoneda(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}
