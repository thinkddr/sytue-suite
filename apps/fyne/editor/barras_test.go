// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLaPestanaElegidaLlevaSuSubrayado: al pulsar otra pestaña, la cinta rehace la tira y
// la nueva activa tiene que llevar el indicador, no solo la letra en medio.
// VISTO EN ROJO (2026-11-06): tras pulsar «Vista» ninguna pestaña llevaba subrayado; el
// indicador de la nueva salía transparente porque solo Refresh lo pintaba.
func TestLaPestanaElegidaLlevaSuSubrayado(t *testing.T) {
	defer ui.TactilDePrueba(false)()
	c, _ := cintaAncha(t, 1280)
	test.Tap(c.tiras[len(c.tiras)-1])
	activa := c.tiras[len(c.tiras)-1]
	if !activa.Activa {
		t.Fatal("el fixture no ha cambiado de pestaña")
	}
	r, ok := test.WidgetRenderer(activa).(*pestanaCintaRenderer)
	if !ok {
		t.Fatal("la pestaña no usa su renderer")
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if got, want := r.indicador.FillColor, tema.Color("indicator", v); got != want {
		t.Fatalf("indicador de la pestaña elegida = %v, quiero %v", got, want)
	}
}

// TestElCarrilDelZoomSeVe: el carril del deslizador de la barra de estado es el
// `--sytue-border` de `.sytue-zoom` (editor.css), también a la derecha del pulgar.
// VISTO EN ROJO (2026-11-06): con `widget.Slider` el carril era el InputBackground del
// tema, la superficie, y sobre la barra blanca solo se veía el pulgar.
func TestElCarrilDelZoomSeVe(t *testing.T) {
	defer ui.TactilDePrueba(false)()
	ctx := &Contexto{Tipo: Documento, Zoom: 100, Estado: Listo, T: func(k string, _ i18n.Vars) string { return k }}
	b := NuevaBarraEstado(ctx)
	w := test.NewTempWindow(t, b)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(600, AltoEstado))
	img := w.Canvas().Capture()
	d := b.deslizador
	x := int(d.Position().X + d.Size().Width - 4)
	y := int(d.Position().Y + d.Size().Height/2)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	if want := tema.Color("border", v); got != want {
		t.Fatalf("carril a la derecha del pulgar en (%d,%d) = %v, quiero el borde %v", x, y, got, want)
	}
}

// TestArrastrarElZoomDentroDeSuDecenaNoRecompone: como el `Step` de `widget.Slider`, el
// deslizador solo avisa al cambiar de decena; cada aviso recompone el marco entero.
// VISTO EN ROJO (2026-11-06): con `ui.Deslizador` cada píxel del arrastre recomponía.
func TestArrastrarElZoomDentroDeSuDecenaNoRecompone(t *testing.T) {
	defer ui.TactilDePrueba(false)()
	n := 0
	ctx := &Contexto{Tipo: Documento, Zoom: 100, Estado: Listo, T: func(k string, _ i18n.Vars) string { return k }}
	ctx.Refrescar = func() { n++ }
	b := NuevaBarraEstado(ctx)
	b.deslizador.Poner(102)
	b.deslizador.Poner(98)
	if n != 0 || ctx.Zoom != 100 || b.deslizador.Valor != 100 {
		t.Fatalf("dentro de la decena: %d recomposiciones, zoom %d, pulgar en %v; quiero 0, 100 y 100", n, ctx.Zoom, b.deslizador.Valor)
	}
	b.deslizador.Poner(108)
	if n != 1 || ctx.Zoom != 110 {
		t.Fatalf("al pasar de decena: %d recomposiciones, zoom %d; quiero 1 y 110", n, ctx.Zoom)
	}
}
