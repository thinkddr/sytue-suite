// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android && sytuepruebas

package portapapeles

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// TestLeeElHTMLDelPortapapelesDeWayland: sin X11, con otra aplicación que ha copiado HTML
// en el compositor, se lee por el control de datos —sin foco ni ventana, y sin
// `wl-paste`—, también uno grande, que no cabe en el búfer de una tubería. Una copia que
// solo es texto, o ninguna, es «no hay HTML».
//
// Necesita un compositor con control de datos: sway sin pantalla, por ejemplo
// `WLR_BACKENDS=headless sway`, y su WAYLAND_DISPLAY.
func TestLeeElHTMLDelPortapapelesDeWayland(t *testing.T) {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("sin compositor Wayland")
	}
	t.Setenv("DISPLAY", "")
	servir := func(tipo string, datos []byte) func() {
		t.Helper()
		parar, err := servirWaylandParaPrueba(tipo, datos)
		if err != nil {
			t.Fatal(err)
		}
		return parar
	}

	html := "<p>Hola, <b>Grace</b> — año</p>"
	parar := servir("text/html", []byte(html))
	nativo, err := leerControlDeDatos("text/html")
	if err != nil || string(nativo) != html {
		parar()
		t.Fatalf("por el control de datos: %q, %v", nativo, err)
	}
	got, err := HTML()
	parar()
	if err != nil || got != html {
		t.Fatalf("HTML(): %q, %v", got, err)
	}

	grande := strings.Repeat("<p>Un párrafo largo de un correo.</p>", 6000) // más que una tubería
	parar = servir("text/html", []byte(grande))
	got, err = HTML()
	parar()
	if err != nil || got != grande {
		t.Fatalf("grande: %d bytes de %d, %v", len(got), len(grande), err)
	}

	parar = servir("text/plain;charset=utf-8", []byte("solo texto"))
	_, err = leerControlDeDatos("text/html")
	parar()
	if !errors.Is(err, ErrSinHTML) {
		t.Errorf("una copia sin HTML: %v", err)
	}
	if _, err := leerControlDeDatos("text/html"); !errors.Is(err, ErrSinHTML) {
		t.Errorf("sin copia: %v", err)
	}
}
