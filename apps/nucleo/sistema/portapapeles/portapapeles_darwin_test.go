// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios && cgo && sytuepruebas

package portapapeles

import (
	"errors"
	"testing"
)

// TestLeeElHTMLDelPortapapelesDeMac: `public.html` se lee tal cual; si solo hay el
// archivo web de Safari, el HTML de su recurso principal; y un portapapeles con solo
// texto es «no hay HTML». Usa un portapapeles con nombre propio: el general es de quien
// está delante.
//
// Se corre en un Mac: `go test -tags sytuepruebas ./sistema/portapapeles/`.
func TestLeeElHTMLDelPortapapelesDeMac(t *testing.T) {
	const nombre = "eu.sytue.pruebas.portapapeles"
	t.Cleanup(func() { soltarParaPrueba(nombre) })
	html := "<p>Hola, <b>Grace</b> — año</p>"

	ponerParaPrueba(nombre, "public.html", []byte(html), false)
	if got, err := leerMac(nombre); err != nil || aUTF8(got) != html {
		t.Fatalf("public.html: %q, %v", got, err)
	}
	ponerParaPrueba(nombre, "com.apple.webarchive", []byte(html), true)
	if got, err := leerMac(nombre); err != nil || aUTF8(got) != html {
		t.Fatalf("archivo web: %q, %v", got, err)
	}
	ponerParaPrueba(nombre, "public.utf8-plain-text", []byte("solo texto"), false)
	if _, err := leerMac(nombre); !errors.Is(err, ErrSinHTML) {
		t.Errorf("solo texto: %v", err)
	}
}
