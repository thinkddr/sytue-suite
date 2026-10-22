// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// Lectura es un documento de texto pintado para LEERLO: sin folio, sin cursor y sin
// edición. Es lo que otra app enseña de un documento de Sytue Docs —la vista previa de
// una nota `doc` en Notepads, que en la web es el `<iframe>` del export—, y se maqueta y
// se pinta con lo mismo que el folio (`maquetar` y `pintarCaja`), así que se ve igual
// que en el editor y no con una copia que se vaya separando de él.
type Lectura struct {
	widget.BaseWidget
	bloques []rico.Bloque
	// folio es el que pinta: sin página ni cursor, solo sus cajas y sus láminas (las
	// imágenes, que así no se vuelven a subir en cada repintado).
	folio Folio
}

// NuevaLectura lee el HTML de un documento (el registro `doc:html`).
func NuevaLectura(html string) *Lectura {
	l := &Lectura{bloques: rico.Parse(html).Bloques()}
	l.ExtendBaseWidget(l)
	return l
}

// AltoPara es lo que ocupa a ese ancho.
func (l *Lectura) AltoPara(ancho float32) float32 {
	_, alto := maquetar(l.bloques, max(ancho, 1))
	return alto
}

// CreateRenderer pinta los bloques al ancho que le den.
func (l *Lectura) CreateRenderer() fyne.WidgetRenderer {
	return &lecturaRenderer{l: l}
}

type lecturaRenderer struct {
	l       *Lectura
	ancho   float32
	objetos []fyne.CanvasObject
}

// MinSize es el alto al ancho que tiene. Sin ancho todavía, nada: maquetar a un píxel
// partiría el documento letra a letra para nada, y el fotograma siguiente ya lo mide.
func (r *lecturaRenderer) MinSize() fyne.Size {
	if w := r.l.Size().Width; w > 0 {
		return fyne.NewSize(0, r.l.AltoPara(w))
	}
	return fyne.NewSize(0, 0)
}

func (r *lecturaRenderer) Layout(t fyne.Size) {
	if t.Width == r.ancho && r.objetos != nil {
		return
	}
	r.ancho = t.Width
	f := &r.l.folio
	f.cajas, _ = maquetar(r.l.bloques, max(t.Width, 1))
	r.objetos = f.pintarContenido(r.objetos[:0], fyne.Position{}, 0, 0, 1)
}

func (r *lecturaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *lecturaRenderer) Refresh()                     { r.ancho = -1; r.Layout(r.l.Size()) }
func (r *lecturaRenderer) Destroy()                     {}
