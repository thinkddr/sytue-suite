// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// AsaArrastre limita el arrastre a un control de 44 px con el dedo. El texto y los
// otros controles de la fila siguen seleccionándose y desplazando la lista.
// AlPulsar es la alternativa accesible (por ejemplo, el selector «Mover a»).
type AsaArrastre struct {
	Boton
	Puede       func() bool
	AlArrastrar func(fyne.Position)
	AlSoltar    func(fyne.Position)
	AlCancelar  func()
	fin         fyne.Position
	arrastrando bool
	cancelado   bool
}

var _ fyne.Draggable = (*AsaArrastre)(nil)

func NuevaAsaArrastre(rotulo string) *AsaArrastre {
	a := &AsaArrastre{Boton: Boton{
		Rotulo: rotulo, Variante: Fantasma, Talla: TallaSM,
		Icono: iconos.Arrastrar, TamIcono: 16, SoloIcono: true, Atenuado: true,
	}}
	a.ExtendBaseWidget(a)
	return a
}

func (a *AsaArrastre) Dragged(e *fyne.DragEvent) {
	if a.cancelado {
		return
	}
	if a.inerte() || (a.Puede != nil && !a.Puede()) {
		a.cancelado = true
		a.Cancelar()
		return
	}
	if !a.arrastrando {
		a.arrastrando = true
		if c := lienzoDe(a); c != nil {
			enfocar(c, a)
		}
	}
	a.fin = e.AbsolutePosition
	a.pulsacionDePuntero(true)
	if a.AlArrastrar != nil {
		a.AlArrastrar(a.fin)
	}
}

func (a *AsaArrastre) DragEnd() {
	if a.cancelado {
		a.cancelado = false
		return
	}
	if a.inerte() || (a.Puede != nil && !a.Puede()) {
		a.Cancelar()
		a.cancelado = false
		return
	}
	if !a.arrastrando {
		return
	}
	a.arrastrando = false
	a.pulsacionDePuntero(false)
	if a.AlSoltar != nil {
		a.AlSoltar(a.fin)
	}
}

func (a *AsaArrastre) FocusLost() {
	a.Cancelar()
	a.Boton.FocusLost()
}

func (a *AsaArrastre) Cancelar() {
	if !a.arrastrando {
		return
	}
	a.arrastrando = false
	a.cancelado = true
	a.pulsacionDePuntero(false)
	if a.AlCancelar != nil {
		a.AlCancelar()
	}
}

func (a *AsaArrastre) CancelarArrastre() { a.Cancelar() }

func (a *AsaArrastre) TypedKey(e *fyne.KeyEvent) {
	if e.Name == fyne.KeyEscape && a.arrastrando {
		a.Cancelar()
		return
	}
	a.Boton.TypedKey(e)
}

// AutoScrollArrastre sigue el borde mientras se mantiene el puntero allí. La app
// aporta el desplazador real: List.ScrollToOffset o Scroll.Scrolled, sin copiar
// motores de listas ni capturar sus eventos. Parar se llama al soltar o cancelar.
type AutoScrollArrastre struct {
	zona  fyne.CanvasObject
	pos   fyne.Position
	mover func(dx, dy float32)
	reloj *time.Timer
	serie uint64
}

func (a *AutoScrollArrastre) Actualizar(zona fyne.CanvasObject, pos fyne.Position, mover func(dx, dy float32)) {
	a.zona, a.pos, a.mover = zona, pos, mover
	if a.reloj == nil {
		serie := a.serie
		// Un paso breve por el borde no debe desplazar la vista antes de soltar.
		a.reloj = time.AfterFunc(180*time.Millisecond, func() {
			fyne.Do(func() {
				if a.serie == serie {
					a.paso()
				}
			})
		})
	}
}

func (a *AutoScrollArrastre) Parar() {
	a.serie++
	if a.reloj != nil {
		a.reloj.Stop()
		a.reloj = nil
	}
	a.zona, a.mover = nil, nil
}

func (a *AutoScrollArrastre) paso() {
	a.reloj = nil
	if a.zona == nil || a.mover == nil || !a.zona.Visible() {
		return
	}
	app := fyne.CurrentApp()
	if app == nil || app.Driver().CanvasForObject(a.zona) == nil {
		a.Parar()
		return
	}
	inicio, tam := app.Driver().AbsolutePositionForObject(a.zona), a.zona.Size()
	x, y := a.pos.X-inicio.X, a.pos.Y-inicio.Y
	if x < 0 || y < 0 || x > tam.Width || y > tam.Height {
		return
	}
	dx, dy := pasoBordeArrastre(x, tam.Width), pasoBordeArrastre(y, tam.Height)
	if dx == 0 && dy == 0 {
		return
	}
	serie := a.serie
	a.mover(dx, dy)
	if a.serie != serie || a.zona == nil {
		return
	}
	a.reloj = time.AfterFunc(35*time.Millisecond, func() {
		fyne.Do(func() {
			if a.serie == serie {
				a.paso()
			}
		})
	})
}

func pasoBordeArrastre(pos, lado float32) float32 {
	borde := min(float32(36), lado/4)
	if borde <= 0 {
		return 0
	}
	if pos < borde {
		return -12 * (1 - pos/borde)
	}
	if pos > lado-borde {
		return 12 * (1 - (lado-pos)/borde)
	}
	return 0
}
