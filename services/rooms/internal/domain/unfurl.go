// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"io"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// LinkUnfurl es la vista previa (OpenGraph) cacheada de una URL http/https que
// aparece en un mensaje (Ola 16). Title/Description/Image/SiteName pueden ir vacíos
// (la URL no expuso metadatos): una tarjeta sin nada útil NO se pinta. FetchedAt
// marca cuándo se obtuvo (el servicio la considera fresca dentro de su TTL).
type LinkUnfurl struct {
	URL         string    `json:"url"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	ImageURL    string    `json:"image_url,omitempty"`
	SiteName    string    `json:"site_name,omitempty"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// HasContent indica si el unfurl tiene algo que mostrar (título o imagen). Una URL
// sin metadatos se cachea igualmente (resultado negativo) pero el cliente degrada
// sin tarjeta.
func (u LinkUnfurl) HasContent() bool {
	return strings.TrimSpace(u.Title) != "" || strings.TrimSpace(u.ImageURL) != ""
}

// maxUnfurlFieldLen acota cada campo de metadatos (defensa: una página hostil no
// llena la BD ni el DOM del cliente con megas de texto).
const maxUnfurlFieldLen = 500

// ParseOpenGraph extrae los metadatos OpenGraph (og:title/description/image/
// site_name) de un documento HTML, con el <title> como respaldo del título.
// Clean-room: tokeniza con golang.org/x/net/html (sin construir el árbol) y para
// al cerrar <head> —los metadatos válidos viven en el head, así que no recorre el
// body—. Devuelve un LinkUnfurl con FetchedAt/URL SIN fijar (los pone el servicio).
//
// No sigue ni resuelve recursos: og:image se devuelve tal cual venga (el servicio
// valida su esquema http/https). Puro y testeable.
func ParseOpenGraph(r io.Reader) LinkUnfurl {
	var u LinkUnfurl
	var htmlTitle string
	z := html.NewTokenizer(r)
	for {
		switch z.Next() {
		case html.ErrorToken:
			// EOF o error de lectura: devolvemos lo acumulado (respaldo con <title>).
			applyTitleFallback(&u, htmlTitle)
			return u
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "meta":
				prop, content := metaProp(z, hasAttr)
				assignOG(&u, prop, content)
			case "title":
				// El texto del título es el siguiente token de texto.
				if z.Next() == html.TextToken {
					htmlTitle = strings.TrimSpace(string(z.Text()))
				}
			case "body":
				// Los metadatos OG viven en <head>: al abrir <body> ya no hay más.
				applyTitleFallback(&u, htmlTitle)
				return u
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "head" {
				applyTitleFallback(&u, htmlTitle)
				return u
			}
		default:
			// TextToken/CommentToken/DoctypeToken: no aportan metadatos OG; se ignoran.
		}
	}
}

// applyTitleFallback usa el <title> del documento si no hubo og:title.
func applyTitleFallback(u *LinkUnfurl, htmlTitle string) {
	if u.Title == "" && htmlTitle != "" {
		u.Title = clampField(htmlTitle)
	}
}

// metaProp lee el par (property|name, content) de una etiqueta <meta>. Reconoce
// tanto property="og:…" (OpenGraph) como name="…" (algunos sitios usan name para
// description/og). Devuelve la clave en minúsculas.
func metaProp(z *html.Tokenizer, hasAttr bool) (key, content string) {
	for hasAttr {
		var k, v []byte
		k, v, hasAttr = z.TagAttr()
		switch strings.ToLower(string(k)) {
		case "property", "name":
			if key == "" { // property gana si aparecen ambos; el primero fija la clave
				key = strings.ToLower(strings.TrimSpace(string(v)))
			}
		case "content":
			content = string(v)
		}
	}
	return key, content
}

// assignOG asigna un par meta a su campo del unfurl si es un OG conocido. El primer
// valor no vacío gana (no lo pisa un duplicado posterior).
func assignOG(u *LinkUnfurl, key, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	switch key {
	case "og:title":
		if u.Title == "" {
			u.Title = clampField(content)
		}
	case "og:description", "description":
		if u.Description == "" {
			u.Description = clampField(content)
		}
	case "og:image", "og:image:url", "og:image:secure_url":
		if u.ImageURL == "" {
			u.ImageURL = clampField(content)
		}
	case "og:site_name":
		if u.SiteName == "" {
			u.SiteName = clampField(content)
		}
	}
}

// clampField recorta un campo de metadatos a su tope en runas (sin cortar a mitad
// de rune).
func clampField(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxUnfurlFieldLen {
		return s
	}
	// Recorte seguro por runas.
	runes := []rune(s)
	if len(runes) <= maxUnfurlFieldLen {
		return s
	}
	return string(runes[:maxUnfurlFieldLen])
}
