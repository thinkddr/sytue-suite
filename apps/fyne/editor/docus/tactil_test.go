// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElDedoSeleccionaYAgranda: en un táctil, mantener pulsado selecciona la palabra y
// salen los dos tiradores; arrastrar el del final lo lleva más allá sin mover el otro, y
// arrastrar lejos de los tiradores desplaza sin tocar la selección. Se vio en ROJO con el
// arrastre del móvil solo desplazando, como antes: la selección no crecía.
func TestElDedoSeleccionaYAgranda(t *testing.T) {
	antes := esMovil
	esMovil = func() bool { return true }
	t.Cleanup(func() { esMovil = antes })
	f := folioCon(t, `<p>Hola mundo bonito</p>`)
	c := software.NewCanvas()
	c.SetContent(container.NewStack(f))
	c.Resize(fyne.NewSize(900, 800))
	en := func(q punto, abajo float32) fyne.Position {
		x, y, alto, _ := f.caretEnMaqueta(q)
		z := f.escala()
		ox, oy := f.pag.origen()
		return fyne.NewPos(ox+x*z, oy+(y+alto*abajo)*z)
	}
	f.pag.TappedSecondary(&fyne.PointEvent{Position: en(punto{0, 7}, 0.5)})
	if a, b := f.orden(); a != (punto{0, 5}) || b != (punto{0, 10}) {
		t.Fatalf("la pulsación larga selecciona «mundo»: %v–%v", a, b)
	}
	ini, fin, ok := f.tiradoresTexto()
	if !ok || fin.X <= ini.X {
		t.Fatalf("los tiradores: %v %v %v", ini, fin, ok)
	}
	circulos := 0
	for _, o := range f.pintarTiradoresTexto(nil, tema.Claro) {
		if _, es := o.(*canvas.Circle); es {
			circulos++
		}
	}
	if circulos != 2 {
		t.Errorf("se pintan %d tiradores", circulos)
	}
	destino := en(punto{0, 17}, 1)
	destino.Y += 2 * radioTiradorTexto
	f.pag.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: destino}, Dragged: fyne.NewDelta(destino.X-fin.X, destino.Y-fin.Y)})
	f.pag.DragEnd()
	if a, b := f.orden(); a != (punto{0, 5}) || b != (punto{0, 17}) {
		t.Errorf("el tirador del final llega a «bonito»: %v–%v", a, b)
	}
	lejos := fyne.NewPos(ini.X, ini.Y+300)
	f.pag.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: lejos}, Dragged: fyne.NewDelta(0, 40)})
	f.pag.DragEnd()
	if a, b := f.orden(); a != (punto{0, 5}) || b != (punto{0, 17}) {
		t.Errorf("arrastrar lejos no toca la selección: %v–%v", a, b)
	}
}
