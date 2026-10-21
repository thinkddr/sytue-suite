// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLaPaginaNoEncontradaCaeDondeLaWeb compara con la captura de la web a 1280×800
// (`ronda-1/404-despues-light.png`): la columna mide 275 y queda centrada, así que la
// ilustración empieza en 262,5 y el botón en 506 con sus 31,5. VISTO EN ROJO poniendo el
// `mb-6` de la ilustración en `mb-4`: el botón sube a 499.
func TestLaPaginaNoEncontradaCaeDondeLaWeb(t *testing.T) {
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	p := NuevaPaginaNoEncontrada(tr, nil)
	w := test.NewWindow(p)
	defer w.Close()
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	p.Resize(fyne.NewSize(1280, 800))
	r := test.WidgetRenderer(p).(*noEncontradaRenderer)
	r.Layout(fyne.NewSize(1280, 800))

	casi(t, "el alto de la columna", p.AltoColumnaNoEncontrada(1280), 296)
	casi(t, "la ilustración", r.ilus.Objeto().Position().Y, 252)
	casi(t, "su ancho", r.ilus.Objeto().Size().Width, 160)
	casi(t, "el botón", p.boton.Position().Y, 512)
	casi(t, "su alto", p.boton.Size().Height, 36)
	casi(t, "centrado", p.boton.Position().X+p.boton.Size().Width/2, 640)
	if len(r.titulo) != 1 || len(r.frase) != 1 {
		t.Errorf("título en %d líneas y frase en %d: en la web, una y una", len(r.titulo), len(r.frase))
	}
	if got := p.Titulo; got != "No encontramos esta página" {
		t.Errorf("el título es %q: la clave `notFound.title` no se resuelve", got)
	}

	img := w.Canvas().Capture()
	if got, quiero := px(img, 5, 5), tema.Color("bg", tema.Claro); !cerca(got, quiero) {
		t.Errorf("el fondo pinta %v y `bg` es %v", got, quiero)
	}
}
