// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"net/url"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Enlaces de una celda: el destino de `=HYPERLINK(destino; [nombre])` y la URL escrita tal
// cual. Es `sheets/enlaces.ts` de la web: la celda se pinta subrayada, Ctrl/⌘+clic y
// Alt+Intro la abren, y la celda activa enseña un chip con el destino. Solo `http`,
// `https` y `mailto`: un `javascript:` o un `file:` que llegue de un libro importado o de
// un coeditor no se vuelve nunca algo que se pulsa.

// pistaEnlace es el corte barato antes de mirar la celda: el pintor pregunta por cada
// celda visible en cada pintado.
var pistaEnlace = regexp.MustCompile(`(?i)hyperlink|https?:|mailto:`)

// reEsquema es el esquema escrito al principio («https:»). Sin él no hay enlace: un
// «www.ejemplo.org» o cualquier texto con un punto se queda en texto.
var reEsquema = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*):`)

// hrefSeguro es el enlace que se puede abrir para un texto, o "" si no es uno permitido:
// esquema http, https o mailto escrito, sin espacios dentro (lo que separa una URL de una
// frase que empieza por «http»), con máquina si es web y con destinatario si es correo.
func hrefSeguro(texto string) string {
	t := strings.TrimSpace(texto)
	if t == "" || strings.ContainsFunc(t, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return ""
	}
	m := reEsquema.FindStringSubmatch(t)
	if m == nil {
		return ""
	}
	u, err := url.Parse(t)
	if err != nil {
		return ""
	}
	switch strings.ToLower(m[1]) {
	case "http", "https":
		if u.Hostname() == "" {
			return ""
		}
	case "mailto":
		if u.Opaque == "" {
			return ""
		}
	default:
		return ""
	}
	return u.String()
}

// Enlace es lo que abre la celda: el destino de su `=HYPERLINK(…)` de raíz, evaluado con
// el motor (`HYPERLINK(A1&"/x")` sigue a A1), o su literal si es una URL. "" si no hay.
func (l *Libro) Enlace(c, f int) string {
	html := l.HTML(c, f)
	if html == "" || !pistaEnlace.MatchString(html) {
		return ""
	}
	crudo := strings.TrimSpace(textoPlano(html))
	if !strings.HasPrefix(crudo, "=") {
		return hrefSeguro(crudo)
	}
	l.sincronizarOcultas()
	destino, ok := l.motor.DestinoHyperlink(l.Hoja(), c, f)
	if !ok {
		return ""
	}
	return hrefSeguro(destino)
}

// rotuloEnlace es lo que enseña el chip: la máquina de una web (sin «www.») o la
// dirección de un correo.
func rotuloEnlace(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	if u.Scheme == "mailto" {
		if para, err := url.PathUnescape(u.Opaque); err == nil {
			return para
		}
		return u.Opaque
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

// abrirEnlace lleva el enlace al sistema por las puertas de la suite (`api.AbrirFuera`
// para la web, `api.AbrirCorreo` para el correo), que vuelven a filtrar lo que pasa: es
// la última antes del navegador. Un `mailto:` se recompone con su destinatario, su asunto
// y su cuerpo; el resto de campos (`cc`, `bcc`…) no viaja, que es por donde un libro
// ajeno añadiría destinatarios sin que se vean en el chip.
func abrirEnlace(a api.Abridor, href string) error {
	seguro := hrefSeguro(href)
	if seguro == "" {
		return api.ErrNoSeAbre
	}
	u, err := url.Parse(seguro)
	if err != nil {
		return api.ErrNoSeAbre
	}
	if u.Scheme != "mailto" {
		return api.AbrirFuera(a, seguro)
	}
	para, err := url.PathUnescape(u.Opaque)
	if err != nil {
		return api.ErrNoSeAbre
	}
	q := u.Query()
	return api.AbrirCorreo(a, para, q.Get("subject"), q.Get("body"))
}

// AbrirEnlace abre el enlace de la celda (c, f), si lo tiene. Lo usan Ctrl/⌘+clic,
// Alt+Intro y el chip. `AlAbrirEnlace` lo sustituye (pruebas); si no, va al sistema.
func (r *Rejilla) AbrirEnlace(c, f int) bool {
	href := r.L.Enlace(c, f)
	if href == "" {
		return false
	}
	if r.AlAbrirEnlace != nil {
		r.AlAbrirEnlace(href)
		return true
	}
	if a := fyne.CurrentApp(); a != nil {
		_ = abrirEnlace(a, href)
	}
	return true
}
