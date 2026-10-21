// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"strconv"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// CertiPDF ya desplaza la portada completa. El mismo gesto sigue moviendo todas sus
// secciones después de girar el teléfono o estrechar una ventana de escritorio.
func TestLaPortadaPDFDesplazaCrearYRecientesJuntos(t *testing.T) {
	p, ventana := portadaMontada(t, fyne.NewSize(1280, 640))
	defer ventana.Close()
	p.Portada.Lista = nil
	for i := range 30 {
		p.Portada.Lista = append(p.Portada.Lista, api.File{ID: strconv.Itoa(i), Name: "Documento " + strconv.Itoa(i) + ".pdf"})
	}
	p.Portada.Recomponer()
	for _, tam := range []fyne.Size{fyne.NewSize(1280, 640), fyne.NewSize(360, 640), fyne.NewSize(820, 360), fyne.NewSize(1280, 640)} {
		ventana.Resize(tam)
		_ = ventana.Canvas().Capture()
		m := p.Portada.marco
		p.Portada.scroll.ScrollToTop()
		ultimo := m.tarjetas[len(m.tarjetas)-1]
		piezas := []fyne.CanvasObject{m.titulo, m.acceso, ultimo}
		antes := make([]float32, len(piezas))
		for i, pieza := range piezas {
			antes[i] = absoluta(pieza).Y
		}
		test.Scroll(ventana.Canvas(), fyne.NewPos(100, 60), 0, -80)
		for i, pieza := range piezas {
			if got := antes[i] - absoluta(pieza).Y; !cerca32b(got, 80) {
				t.Errorf("a %v una sección se desplaza %v, la portada debe bajar 80", tam, got)
			}
		}
		p.Portada.scroll.ScrollToBottom()
		pos := absoluta(ultimo)
		if pos.Y < 0 || pos.Y+ultimo.Size().Height > tam.Height+0.1 || pos.X < 0 || pos.X+ultimo.Size().Width > tam.Width+0.1 {
			t.Errorf("a %v no se alcanza entero el último PDF: %v/%v", tam, pos, ultimo.Size())
		}
	}
}
