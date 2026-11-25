// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// LA BARRA PEGADA AL TECLADO DEL TELÉFONO. Escribiendo en el teléfono, la barra de
// formato se va de encima de la nota (donde queda fuera de la vista en cuanto se baja un
// poco) y se pone justo encima del teclado en pantalla, como la de Confluence o la de
// Notas: lo que se usa mientras se escribe, donde está el pulgar. Se va con el teclado o
// con el foco, y vuelve arriba. Las barras de tabla y de código siguen encima del cuerpo:
// son del bloque, no de la escritura.
//
// El alto del teclado lo dice el sistema (`ui.AltoTeclado`); se mira cada décima de
// segundo mientras el cuerpo tiene el foco, que es lo que tarda en salir y en cambiar de
// alto (la fila de sugerencias, otro idioma). La barra va en el portal de la pantalla
// (flotante.go), encima de todo, a lo ancho.

// intervaloTeclado es cada cuánto se mira el teclado mientras se escribe.
const intervaloTeclado = 100 * time.Millisecond

// enTeclado dice si la barra de formato va pegada al teclado: en el teléfono, con el
// cuerpo enfocado, el teclado fuera y un portal donde ponerla.
func (e *EditorNota) enTeclado() bool {
	return e.tecladoAlto > 0 && e.zona.enfocado && e.portal != nil && ui.EsMovil()
}

// vigilarTeclado empieza a mirar el teclado (al enfocar el cuerpo en el teléfono).
func (z *zonaEdicion) vigilarTeclado() {
	if !ui.EsMovil() || z.e.portal == nil || z.pararTeclado != nil {
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(z)
	if c == nil {
		return
	}
	parar := make(chan struct{})
	z.pararTeclado = parar
	go func() {
		reloj := time.NewTicker(intervaloTeclado)
		defer reloj.Stop()
		for {
			select {
			case <-parar:
				return
			case <-reloj.C:
			}
			alto := ui.AltoTeclado(c)
			fyne.Do(func() { z.e.ponerTeclado(alto) })
		}
	}()
}

// dejarTeclado deja de mirarlo (al perder el foco) y devuelve la barra arriba.
func (z *zonaEdicion) dejarTeclado() {
	if z.pararTeclado != nil {
		close(z.pararTeclado)
		z.pararTeclado = nil
	}
	z.e.ponerTeclado(0)
}

// ponerTeclado apunta el alto del teclado y, si ha cambiado, recoloca la barra.
func (e *EditorNota) ponerTeclado(alto float32) {
	if e.tecladoAlto == alto {
		return
	}
	e.tecladoAlto = alto
	e.Refresh()
	llamar0(e.AlCrecer)
}

// colocarEnTeclado pone la barra de formato encima del teclado, a lo ancho del portal:
// lo que el teclado pisa de él es su alto menos lo que queda de pantalla por debajo del
// portal (en iOS la zona de la app ya acaba en el teclado, y entonces no pisa nada).
func (e *EditorNota) colocarEnTeclado() {
	c := fyne.CurrentApp().Driver().CanvasForObject(e)
	pc := e.portal.c
	if c == nil {
		return
	}
	abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(pc)
	debajo := max(c.Size().Height-abs.Y-pc.Size().Height, 0)
	tapa := max(e.tecladoAlto-debajo, 0)
	ancho := pc.Size().Width
	h := altoDe(e.barra, ancho)
	e.barra.Move(fyne.NewPos(0, max(pc.Size().Height-tapa-h, 0)))
	e.barra.Resize(fyne.NewSize(ancho, h))
}
