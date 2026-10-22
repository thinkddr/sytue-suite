// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/software"
)

// # La opacidad de GRUPO de las entradas
//
// `sytue-modal-in`, `sytue-pop-in` y `sytue-toast-in` animan `opacity` sobre el panel
// ENTERO: el navegador compone el subárbol en una capa y la funde de una vez. Fyne no
// tiene opacidad de grupo —cada primitiva lleva su color—, y bajar el alfa de cada una
// por separado no es lo mismo: donde se solapan (el texto sobre su caja) se ve la caja a
// través del texto, un velo más oscuro en mitad de la animación.
//
// Lo que sí es lo mismo es hacer lo que hace el navegador: pintar el panel UNA vez a un
// mapa de bits con el pintor de software —que es Go puro y el mismo que usan los
// guardianes— y fundir ESE mapa, que es una `canvas.Image` y los dos pintores le aplican
// `Translucency` igual. Mientras dura, el panel de verdad está escondido, y al acabar
// vuelve y la imagen se va. El mismo mapa lleva también el `scale(.98)` del diálogo y del
// menú, que Fyne no sabe hacerle a un subárbol.
//
// ponytail: mientras dura (150-180 ms) el panel no se pulsa: el clic cae en el velo. En
// la web sí se podría. Es menos que el tiempo de reacción de un segundo clic apuntado; si
// alguna vez molesta, la imagen puede ir encima del panel con el puntero atravesándola.

// instantanea pinta `o` —ya colocado y con su tamaño— a la escala `esc`, con el fondo
// transparente, y lo devuelve a su sitio. Es el `will-change: opacity` del navegador:
// una capa del subárbol pintada una vez.
//
// El lienzo de software lo coloca en (0,0) y le da su tamaño al montarlo: por eso se
// guardan y se restauran la posición y el tamaño. No toca el lienzo de la ventana ni el
// foco: el panel sigue en su árbol.
func instantanea(o fyne.CanvasObject, esc float32) image.Image {
	pos, tam := o.Position(), o.Size()
	l := software.NewTransparentCanvas()
	l.SetPadded(false)
	l.SetScale(esc)
	l.SetContent(o)
	l.Resize(tam)
	img := l.Capture()
	o.Resize(tam)
	o.Move(pos)
	return img
}

// fundido es la entrada con opacidad de grupo de un panel: la imagen que se funde en su
// lugar y la sombra que la acompaña.
type fundido struct {
	img    *canvas.Image
	panel  fyne.CanvasObject
	sombra *Sombra
	// pos y tam son los del panel ya colocado; la sombra, los de su imagen.
	pos            fyne.Position
	tam            fyne.Size
	sPos           fyne.Position
	sTam           fyne.Size
	dx, dy, escala float32
	// anclaY es el `transform-origin` en alto: 0,5 el centro (el de por defecto), 0 el
	// borde de arriba (`origin-top` de Popover.tsx cuando cuelga hacia abajo), 1 el de abajo.
	anclaY   float32
	alAcabar func()
}

// fundir arranca la entrada de `panel`: de `dy` px desplazado, a `escala` y
// transparente, a su sitio, a su tamaño y opaco, en `dur` con CurvaEstandar. `poner` es
// quien pinta la imagen (la capa): la recibe al empezar y nil al acabar, y repinta.
//
// Sin lienzo (el panel no está montado) o con SinMovimiento no anima: sale ya puesto.
func fundir(c fyne.Canvas, panel fyne.CanvasObject, sombra *Sombra, dur time.Duration,
	dy, escala, anclaY float32, poner func(*fundido),
) {
	fundirDesplazado(c, panel, sombra, dur, 0, dy, escala, anclaY, poner)
}

func fundirDesplazado(c fyne.Canvas, panel fyne.CanvasObject, sombra *Sombra, dur time.Duration,
	dx, dy, escala, anclaY float32, poner func(*fundido),
) {
	if c == nil || SinMovimiento || panel.Size().IsZero() {
		return
	}
	f := nuevoFundido(c, panel, sombra, dy, escala)
	f.anclaY = anclaY
	f.dx = dx
	f.fotograma(0)
	f.alAcabar = func() { poner(nil) }
	poner(f)
	a := fyne.NewAnimation(dur, func(p float32) {
		f.fotograma(p)
		if p >= 1 {
			f.acabar()
		}
	})
	a.Curve = CurvaEstandar
	a.Start()
}

// nuevoFundido pinta el panel a su imagen, la pone en el primer fotograma y esconde el
// panel. Aparte de fundir para poder mirar un fotograma a mano: el driver de pruebas
// salta cada animación a su final.
func nuevoFundido(c fyne.Canvas, panel fyne.CanvasObject, sombra *Sombra, dy, escala float32) *fundido {
	f := &fundido{panel: panel, sombra: sombra, pos: panel.Position(), tam: panel.Size(), dy: dy, escala: escala, anclaY: 0.5}
	if sombra != nil {
		f.sPos, f.sTam = sombra.img.Position(), sombra.img.Size()
	}
	f.img = canvas.NewImageFromImage(instantanea(panel, c.Scale()))
	f.img.FillMode = canvas.ImageFillStretch
	f.img.ScaleMode = canvas.ImageScaleSmooth
	f.fotograma(0)
	panel.Hide()
	return f
}

// fotograma pone la imagen (y la sombra) en el punto `p` de la entrada.
func (f *fundido) fotograma(p float32) {
	s := f.escala + (1-f.escala)*p
	desp := f.dy * (1 - p)
	escalar := func(o fyne.CanvasObject, pos fyne.Position, tam fyne.Size) {
		// `transform-origin`: el punto de anclaje se queda, el tamaño cambia.
		w, h := tam.Width*s, tam.Height*s
		o.Move(fyne.NewPos(pos.X+(tam.Width-w)/2+f.dx*(1-p), pos.Y+(tam.Height-h)*f.anclaY+desp))
		o.Resize(fyne.NewSize(w, h))
	}
	escalar(f.img, f.pos, f.tam)
	f.img.Translucency = float64(1 - p)
	canvas.Refresh(f.img)
	if f.sombra != nil && !f.sTam.IsZero() {
		escalar(f.sombra.img, f.sPos, f.sTam)
		f.sombra.img.Translucency = float64(1 - p)
		canvas.Refresh(f.sombra.img)
	}
}

// acabar devuelve el panel y la sombra a su sitio y quita la imagen.
func (f *fundido) acabar() {
	if f.img == nil {
		return
	}
	f.img = nil
	f.panel.Move(f.pos)
	f.panel.Resize(f.tam)
	f.panel.Show()
	if f.sombra != nil && !f.sTam.IsZero() {
		f.sombra.img.Move(f.sPos)
		f.sombra.img.Resize(f.sTam)
		f.sombra.img.Translucency = 0
		canvas.Refresh(f.sombra.img)
	}
	if f.alAcabar != nil {
		f.alAcabar()
	}
}

// objeto es la imagen mientras dura la entrada; nil cuando ya ha acabado.
func (f *fundido) objeto() fyne.CanvasObject {
	if f == nil || f.img == nil {
		return nil
	}
	return f.img
}
