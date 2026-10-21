// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android && cgo && sytuepruebas

package portapapeles

import (
	"errors"
	"os"
	"strings"
	"testing"
	"unicode/utf16"
)

// TestLeeElHTMLDelPortapapelesDeX11: con otra aplicación dueña del CLIPBOARD, se lee su
// `text/html` entero; también por INCR (un HTML grande, por trozos, como los navegadores)
// y en UTF-16 con BOM (Firefox). Un dueño que solo tiene texto, o ningún dueño, es «no
// hay HTML», que deja pegar el texto.
//
// Necesita un servidor X: `xvfb-run -a go test -tags sytuepruebas ./sistema/portapapeles/`.
func TestLeeElHTMLDelPortapapelesDeX11(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("sin servidor X (xvfb-run)")
	}
	servir := func(tipo string, datos []byte, trozo int) func() {
		t.Helper()
		parar, err := servirParaPrueba(tipo, datos, trozo)
		if err != nil {
			t.Fatal(err)
		}
		return parar
	}

	html := "<p>Hola, <b>Grace</b> — año</p>"
	parar := servir("text/html", []byte(html), 0)
	got, err := HTML()
	parar()
	if err != nil || got != html {
		t.Fatalf("de una vez: %q, %v", got, err)
	}

	grande := strings.Repeat("<p>Un párrafo largo de un correo.</p>", 400)
	parar = servir("text/html", []byte(grande), 1000)
	got, err = HTML()
	parar()
	if err != nil || got != grande {
		t.Fatalf("por INCR: %d bytes de %d, %v", len(got), len(grande), err)
	}

	u := utf16.Encode([]rune("<p>Firefox — ñ</p>"))
	le := []byte{0xFF, 0xFE}
	for _, c := range u {
		le = append(le, byte(c), byte(c>>8))
	}
	parar = servir("text/html", le, 0)
	got, err = HTML()
	parar()
	if err != nil || got != "<p>Firefox — ñ</p>" {
		t.Fatalf("en UTF-16: %q, %v", got, err)
	}

	parar = servir("UTF8_STRING", []byte("solo texto"), 0)
	_, err = HTML()
	parar()
	if !errors.Is(err, ErrSinHTML) {
		t.Errorf("un dueño sin HTML: %v", err)
	}
	if _, err := HTML(); !errors.Is(err, ErrSinHTML) {
		t.Errorf("sin dueño: %v", err)
	}
}
