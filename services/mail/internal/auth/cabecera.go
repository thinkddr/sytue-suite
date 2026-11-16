// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"strings"
	"unicode"
)

// CabeceraAuthenticationResults compone la cabecera `Authentication-Results`
// (RFC 8601) que el MX antepone al mensaje guardado, para que cualquier cliente
// (IMAP, el .eml descargado) vea el veredicto que dio Sytue al recibirlo y no
// tenga que fiarse de lo que trajera el remitente.
//
// authservID es el nombre del MX (MAIL_HOSTNAME); los dominios van como
// propiedades `smtp.mailfrom` y `header.from` cuando se conocen. Devuelve la línea
// completa con su CRLF final.
func CabeceraAuthenticationResults(authservID string, spf, dkim, dmarc Result, mailFromDomain, fromDomain string) string {
	authservID = valorSeguro(authservID)
	if authservID == "" {
		authservID = "sytue"
	}
	var b strings.Builder
	b.WriteString("Authentication-Results: ")
	b.WriteString(authservID)
	met := func(nombre string, r Result, prop, valor string) {
		if r == "" {
			r = ResultNone
		}
		b.WriteString(";\r\n\t")
		b.WriteString(nombre)
		b.WriteString("=")
		b.WriteString(string(r))
		if v := valorSeguro(valor); v != "" {
			b.WriteString(" ")
			b.WriteString(prop)
			b.WriteString("=")
			b.WriteString(v)
		}
	}
	met("spf", spf, "smtp.mailfrom", mailFromDomain)
	met("dkim", dkim, "header.d", "")
	met("dmarc", dmarc, "header.from", fromDomain)
	b.WriteString("\r\n")
	return b.String()
}

// valorSeguro deja solo lo que cabe en un token de la cabecera: el dominio llega
// del remitente y un CRLF o un «;» colado inyectaría cabeceras o métodos falsos.
func valorSeguro(s string) string {
	return strings.Map(func(r rune) rune {
		if r > unicode.MaxASCII || r <= ' ' || strings.ContainsRune(";()\"\\=", r) {
			return -1
		}
		return r
	}, strings.ToLower(strings.TrimSpace(s)))
}
