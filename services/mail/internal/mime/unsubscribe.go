// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mime

import (
	"bytes"
	"net/mail"
	"net/url"
	"strings"
)

// Unsubscribe son los métodos de baja que anuncia un mensaje (RFC 2369
// List-Unsubscribe + RFC 8058 one-click). Solo se exponen los métodos SEGUROS:
// una URL https (nunca http en claro: no se sigue un enlace inseguro) y una
// dirección mailto. OneClick indica que el remitente admite la baja en un clic
// por POST (cabecera List-Unsubscribe-Post) sobre esa URL https.
type Unsubscribe struct {
	// HTTPS es la primera URL https de List-Unsubscribe ("" si no hay).
	HTTPS string
	// Mailto es la dirección de la primera entrada mailto ("" si no hay).
	Mailto string
	// MailtoSubject es el ?subject= de esa entrada mailto (RFC 6068), si lo trae.
	MailtoSubject string
	// OneClick: hay List-Unsubscribe-Post: List-Unsubscribe=One-Click Y una URL
	// https, así que la baja se puede completar por POST en un clic (RFC 8058).
	OneClick bool
}

// Has indica si el mensaje ofrece algún método de baja utilizable.
func (u Unsubscribe) Has() bool { return u.HTTPS != "" || u.Mailto != "" }

// ParseUnsubscribe extrae los métodos de baja de las cabeceras de un mensaje
// crudo RFC 5322. Clean-room sobre RFC 2369 (List-Unsubscribe: lista de <URI>
// separadas por comas) y RFC 8058 (List-Unsubscribe-Post). Nunca falla: si las
// cabeceras no existen o no parsean, devuelve un Unsubscribe vacío (Has()==false).
func ParseUnsubscribe(raw []byte) Unsubscribe {
	var out Unsubscribe
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return out
	}
	for _, uri := range parseURIList(m.Header.Get("List-Unsubscribe")) {
		switch strings.ToLower(schemeOf(uri)) {
		case "https":
			if out.HTTPS == "" {
				out.HTTPS = uri
			}
		case "mailto":
			if out.Mailto == "" {
				out.Mailto, out.MailtoSubject = parseMailto(uri)
			}
		}
		// http en claro y otros esquemas se ignoran deliberadamente.
	}
	// One-click solo aplica con destino https (RFC 8058 lo exige) y si el remitente
	// lo anuncia con List-Unsubscribe-Post: List-Unsubscribe=One-Click.
	if out.HTTPS != "" {
		post := strings.ToLower(strings.TrimSpace(m.Header.Get("List-Unsubscribe-Post")))
		out.OneClick = strings.Contains(post, "list-unsubscribe=one-click")
	}
	return out
}

// parseURIList extrae las URIs entre <> de una cabecera List-Unsubscribe
// (RFC 2369 §2: "<uri>, <uri>"). Tolera espacios/CFWS entre entradas.
func parseURIList(v string) []string {
	var out []string
	for {
		lt := strings.IndexByte(v, '<')
		if lt < 0 {
			break
		}
		rest := v[lt+1:]
		gt := strings.IndexByte(rest, '>')
		if gt < 0 {
			break
		}
		if uri := strings.TrimSpace(rest[:gt]); uri != "" {
			out = append(out, uri)
		}
		v = rest[gt+1:]
	}
	return out
}

// schemeOf devuelve el esquema de una URI ("" si no tiene ':').
func schemeOf(uri string) string {
	if i := strings.IndexByte(uri, ':'); i > 0 {
		return uri[:i]
	}
	return ""
}

// parseMailto extrae la dirección y el ?subject= de una URI mailto (RFC 6068).
func parseMailto(uri string) (addr, subject string) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", ""
	}
	addr = u.Opaque // "mailto:foo@bar" => Opaque="foo@bar"; el ?query va en RawQuery.
	if addr == "" {
		addr = u.Path
	}
	if i := strings.IndexByte(addr, '?'); i >= 0 {
		addr = addr[:i]
	}
	return strings.TrimSpace(addr), u.Query().Get("subject")
}
