// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// hayMenu dice si hay un menú abierto con ese nombre accesible.
func hayMenu(c fyne.Canvas, rotulo string) bool {
	for _, o := range c.Overlays().List() {
		var mirar func(fyne.CanvasObject) bool
		mirar = func(o fyne.CanvasObject) bool {
			if m, ok := o.(*ui.Menu); ok && m.Rotulo == rotulo {
				return true
			}
			for _, h := range hijos(o) {
				if mirar(h) {
					return true
				}
			}
			return false
		}
		if mirar(o) {
			return true
		}
	}
	return false
}

// TestLosAtajosDeLaSeleccionYLosMenus: «x» marca la conversación del cursor (la
// abierta), Escape vacía la selección —y sin nada marcado no es suya—, «l» abre
// «Etiquetar» y «v» «Mover a…» sobre sus disparadores. Y la hoja de «?» los lista.
//
// VISTO EN ROJO (2026-11-05): sin el caso «x» en `Atajo` no se marca nada, y sin
// guardar los disparadores en `grupoTriaje` «l» y «v» no abren ningún menú.
func TestLosAtajosDeLaSeleccionYLosMenus(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	c := w.Canvas()
	tr := traductor(t)

	esc := &fyne.KeyEvent{Name: fyne.KeyEscape}
	if b.TeclaSuelta(c, esc) {
		t.Error("Escape sin nada marcado no puede ser de la selección")
	}
	b.Atajo(c, 'j') // abre la primera
	b.Atajo(c, 'x')
	if !b.P.Bandeja.Marcadas[b.hiloID] || b.P.Bandeja.CuantasMarcadas() != 1 {
		t.Fatalf("«x» no marca la conversación abierta: %v", b.P.Bandeja.Marcadas)
	}
	if !b.TeclaSuelta(c, esc) || b.P.Bandeja.CuantasMarcadas() != 0 {
		t.Error("Escape no vacía la selección")
	}

	w.Canvas().Capture() // la cabecera del lector, con sus disparadores, ya pintada
	if !b.Atajo(c, 'l') || !hayMenu(c, tr.T("reader.labelConversation", nil)) {
		t.Fatal("«l» no abre «Etiquetar»")
	}
	ui.Descartar(c)
	if !b.Atajo(c, 'v') || !hayMenu(c, tr.T("reader.moveToFolder", nil)) {
		t.Fatal("«v» no abre «Mover a…»")
	}
	ui.Descartar(c)

	b.verAtajos()
	for _, clave := range []string{"shortcuts.select", "shortcuts.clearSelection", "shortcuts.label", "shortcuts.move"} {
		if !hayTexto(c, tr.T(clave, nil)) {
			t.Errorf("la hoja de atajos no lista %q", clave)
		}
	}
}

// hayTexto dice si alguna sección de la hoja de atajos abierta lleva esa descripción.
func hayTexto(c fyne.Canvas, texto string) bool {
	var mirar func(fyne.CanvasObject) bool
	mirar = func(o fyne.CanvasObject) bool {
		if x, ok := o.(*canvas.Text); ok && x.Text == texto {
			return true
		}
		for _, h := range hijos(o) {
			if mirar(h) {
				return true
			}
		}
		return false
	}
	for _, o := range c.Overlays().List() {
		if mirar(o) {
			return true
		}
	}
	return false
}

// TestXNoMarcaSiLaAbiertaNoEstaEnLaLista: abierta desde un aviso del Hub y fuera de la
// lista, «x» no tiene casilla que marcar (la web devuelve sin hacer nada); antes marcaba
// la fila del cursor, otra conversación.
//
// VISTO EN ROJO (2026-11-05): con la búsqueda de la abierta que caía al cursor, marca h1.
func TestXNoMarcaSiLaAbiertaNoEstaEnLaLista(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.hiloID = "fuera"
	if b.Atajo(w.Canvas(), 'x') || b.P.Bandeja.CuantasMarcadas() != 0 {
		t.Errorf("«x» con la abierta fuera de la lista ha marcado %v", b.P.Bandeja.Marcadas)
	}
}
