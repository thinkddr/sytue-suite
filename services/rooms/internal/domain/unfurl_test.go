// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

func TestParseOpenGraphFullCard(t *testing.T) {
	html := `<!doctype html><html><head>
		<title>Página HTML (respaldo)</title>
		<meta property="og:site_name" content="Ejemplo">
		<meta property="og:title" content="Un título OG">
		<meta property="og:description" content="Una descripción de la página.">
		<meta property="og:image" content="https://cdn.example.com/card.png">
		<meta name="viewport" content="width=device-width">
	</head><body><h1>ignora esto</h1></body></html>`
	u := ParseOpenGraph(strings.NewReader(html))
	if u.SiteName != "Ejemplo" {
		t.Fatalf("site_name=%q", u.SiteName)
	}
	// og:title gana sobre <title>.
	if u.Title != "Un título OG" {
		t.Fatalf("title=%q (og:title debe ganar al <title>)", u.Title)
	}
	if u.Description != "Una descripción de la página." {
		t.Fatalf("description=%q", u.Description)
	}
	if u.ImageURL != "https://cdn.example.com/card.png" {
		t.Fatalf("image_url=%q", u.ImageURL)
	}
	if !u.HasContent() {
		t.Fatal("HasContent debe ser true con título+imagen")
	}
}

func TestParseOpenGraphTitleFallback(t *testing.T) {
	// Sin og:title: cae al <title> del documento; name="description" alimenta la desc.
	html := `<html><head><title>  Solo título  </title>
		<meta name="description" content="desc por name"></head><body></body></html>`
	u := ParseOpenGraph(strings.NewReader(html))
	if u.Title != "Solo título" {
		t.Fatalf("title=%q (debe caer al <title> recortado)", u.Title)
	}
	if u.Description != "desc por name" {
		t.Fatalf("description=%q (name=description debe valer de respaldo)", u.Description)
	}
}

func TestParseOpenGraphEmpty(t *testing.T) {
	// Documento sin metadatos ni título: unfurl vacío, HasContent false (negativo).
	u := ParseOpenGraph(strings.NewReader(`<html><head></head><body><p>hola</p></body></html>`))
	if u.HasContent() {
		t.Fatalf("HasContent debe ser false sin metadatos: %+v", u)
	}
}

func TestParseOpenGraphStopsAtBody(t *testing.T) {
	// Un og:title dentro del <body> NO debe leerse (los OG válidos viven en el head).
	html := `<html><head><title>T</title></head><body>
		<meta property="og:title" content="inyectado en body"></body></html>`
	u := ParseOpenGraph(strings.NewReader(html))
	if u.Title != "T" {
		t.Fatalf("title=%q (no debe tomar el og:title del body)", u.Title)
	}
}

func TestClampFieldRunes(t *testing.T) {
	long := strings.Repeat("á", maxUnfurlFieldLen+50) // multibyte: verifica recorte por runa
	got := clampField(long)
	if n := len([]rune(got)); n != maxUnfurlFieldLen {
		t.Fatalf("clampField dejó %d runas, quiero %d", n, maxUnfurlFieldLen)
	}
}
