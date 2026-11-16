// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestLosToposDeLaReglaSonLosDeLaWeb: `withMarkerAt` de `Ruler.tsx` con un folio A4 de
// 794 y 96 de margen (columna de 602): nada pasa del margen ni deja menos de 24 px.
func TestLosToposDeLaReglaSonLosDeLaWeb(t *testing.T) {
	const m, w = 96, 794
	casos := []struct {
		nombre string
		id     int
		x      float32
		cur    sangrias
		quiere sangrias
	}{
		{"izquierda más allá del margen", marcaIzquierda, 0, sangrias{}, sangrias{}},
		{"izquierda conserva la primera línea", marcaIzquierda, m + 50, sangrias{}, sangrias{izq: 50, pri: -50}},
		{"izquierda deja 24 px", marcaIzquierda, w, sangrias{der: 10}, sangrias{izq: 568, pri: -568, der: 10}},
		{"primera no sale del margen", marcaPrimera, 0, sangrias{izq: 30}, sangrias{izq: 30, pri: -30}},
		{"primera deja 24 px", marcaPrimera, w, sangrias{}, sangrias{pri: 578}},
		{"derecha no pasa del borde", marcaDerecha, w, sangrias{}, sangrias{}},
		{"derecha deja 24 px", marcaDerecha, 0, sangrias{izq: 100}, sangrias{izq: 100, der: 478}},
	}
	for _, c := range casos {
		if got := conMarcadorEn(c.id, c.x, m, w, c.cur); got != c.quiere {
			t.Errorf("%s: %+v, quiere %+v", c.nombre, got, c.quiere)
		}
	}
}

func reglaDe(t *testing.T, f *Folio) *Regla {
	t.Helper()
	w := test.NewWindow(f)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1000, 800))
	r := test.WidgetRenderer(f.pag).(*paginaRenderer).regla
	if r.Size().Width == 0 {
		t.Fatal("la regla no se ha colocado")
	}
	return r
}

// TestArrastrarLaReglaSangraElParrafo: arrastrar la izquierda escribe `margin-left` y
// la primera línea se queda donde estaba (`text-indent` negativo); todo el arrastre es
// UN paso de deshacer.
func TestArrastrarLaReglaSangraElParrafo(t *testing.T) {
	const orig = "<p>Hola mundo</p>"
	f := folioCon(t, orig)
	r := reglaDe(t, f)
	z := r.Size().Width / AnchoPagina
	mk := r.marcadores[marcaIzquierda]
	for range 2 {
		mk.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: 20 * z}})
	}
	mk.DragEnd()
	if got := f.Serializar(); !strings.Contains(got, "margin-left: 40px") || !strings.Contains(got, "text-indent: -40px") {
		t.Fatalf("tras arrastrar la izquierda 40 px: %s", got)
	}
	f.deshacer()
	if got := f.Serializar(); got != orig {
		t.Errorf("deshacer tiene que revertir el arrastre entero de una vez: %s", got)
	}

	md := r.marcadores[marcaDerecha]
	md.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: -30 * z}})
	md.DragEnd()
	if got := f.Serializar(); !strings.Contains(got, "margin-right: 30px") {
		t.Errorf("arrastrar la derecha 30 px a la izquierda: %s", got)
	}
}

// TestUnArrastreLentoTambienMueveLaRegla: con zoom, cada evento puede traer menos de
// medio px de maqueta; sumados, tienen que mover el marcador igual que uno rápido.
func TestUnArrastreLentoTambienMueveLaRegla(t *testing.T) {
	f := folioCon(t, "<p>Hola mundo</p>")
	r := reglaDe(t, f)
	z := r.Size().Width / AnchoPagina
	mk := r.marcadores[marcaDerecha]
	for range 40 {
		mk.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: -0.25 * z}})
	}
	mk.DragEnd()
	if got := f.Serializar(); !strings.Contains(got, "margin-right: 10px") {
		t.Fatalf("40 pasos de 0,25 px: %s", got)
	}
	f.deshacer()
	if got := f.Serializar(); strings.Contains(got, "margin-right") {
		t.Errorf("el arrastre lento también es un solo paso de deshacer: %s", got)
	}
}

// TestLaReglaSeMueveConLasFlechas: con el foco en «primera línea», → la mueve 0,25 cm
// (9 px redondeados) y ← la devuelve; cada flecha es un paso de deshacer.
func TestLaReglaSeMueveConLasFlechas(t *testing.T) {
	const orig = "<p>Hola mundo</p>"
	f := folioCon(t, orig)
	r := reglaDe(t, f)
	mp := r.marcadores[marcaPrimera]
	if mp.AccessibilityLabel() == "" {
		t.Error("el marcador necesita nombre accesible")
	}
	test.Tap(mp)
	if !mp.foco {
		t.Fatal("tocar el marcador tiene que darle el foco")
	}
	mp.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if got := f.Serializar(); !strings.Contains(got, "text-indent: 9px") {
		t.Fatalf("→ en la primera línea: %s", got)
	}
	f.deshacer()
	if got := f.Serializar(); got != orig {
		t.Errorf("deshacer la flecha: %s", got)
	}
}
