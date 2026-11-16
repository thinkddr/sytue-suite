// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Los comentarios anclados a una celda en la rejilla (CALC-11 de la web): el triángulo de
// las celdas con hilos abiertos, el resalte de la celda de un hilo al pasar por él en el
// panel, y el «ir a la celda» de su botón. El ancla y su identidad están en anclas.go.

// LadoMarcaComentario es el `backgroundSize: 7px 7px` del triángulo de la web, en px de
// maqueta (se escala con el zoom).
const LadoMarcaComentario float32 = 7

// MarcaComentario es el violeta de marca (`CONTENT_PALETTE[9]`) del triángulo y del
// resalte. Va sobre papel blanco en los dos temas, como la celda.
var MarcaComentario = func() color.NRGBA {
	c, _ := tokens.ParseColor(editor.PaletaContenido[9])
	return c
}()

// nuevaMarcaComentario es el triángulo de la esquina superior derecha: la mitad de un
// cuadrado por encima de su diagonal (`linear-gradient(225deg, violeta 50%, transparent 50%)`).
func nuevaMarcaComentario() *canvas.Raster {
	return canvas.NewRasterWithPixels(func(x, y, w, h int) color.Color {
		if x*h >= y*w {
			return MarcaComentario
		}
		return color.Transparent
	})
}

// PonerComentarios da a la rejilla los hilos del libro, para marcar sus celdas.
func (r *Rejilla) PonerComentarios(cs []api.DocComment) {
	r.comentarios = cs
	r.Refresh()
}

// ResaltarComentario enmarca la celda de un ancla (nil: ninguna), como hace el panel de
// comentarios al pasar el puntero por un hilo. Solo se ve si es de la hoja activa: pasar
// por el panel no cambia de hoja ni mueve la selección.
func (r *Rejilla) ResaltarComentario(a *api.AnclaCelda) {
	r.resalteComentario = a
	r.Refresh()
}

// celdaResaltada es la celda del resalte en la hoja activa, si la hay.
func (r *Rejilla) celdaResaltada() (Celda, bool) {
	if r.resalteComentario == nil {
		return Celda{}, false
	}
	hoja, k, ok := r.L.CeldaDeAncla(*r.resalteComentario)
	if !ok || hoja != r.L.Activa() {
		return Celda{}, false
	}
	if fu, ok := r.L.FusionEn(k.C, k.F); ok {
		k = Celda{C: fu.C1, F: fu.F1}
	}
	return k, true
}

// AnclaActiva es el ancla de la celda activa: la de «Nuevo comentario».
func (c *Lienzo) AnclaActiva() api.AnclaCelda {
	return c.Libro.AnclaDeCelda(c.Rejilla.sel.C, c.Rejilla.sel.F)
}

// IrAComentario selecciona la celda de un ancla y la trae a la vista, cambiando de hoja si
// hace falta (`revealCommentCell`). false si la celda ya no existe.
func (c *Lienzo) IrAComentario(a api.AnclaCelda) bool {
	hoja, k, ok := c.Libro.CeldaDeAncla(a)
	if !ok {
		return false
	}
	c.CambiarHoja(hoja)
	c.Rejilla.Seleccionar(k.C, k.F, false)
	if cv := fyne.CurrentApp().Driver().CanvasForObject(c.Rejilla); cv != nil {
		cv.Focus(c.Rejilla)
	}
	return true
}

// pintarMarcaComentario coloca el triángulo en la esquina de arriba a la derecha de la
// caja de la celda, por dentro de su línea derecha.
func pintarMarcaComentario(m *canvas.Raster, si bool, x, y, w, anchoDer, z float32) {
	if !si {
		m.Hide()
		return
	}
	lado := LadoMarcaComentario * z
	m.Move(fyne.NewPos(x+w-anchoDer-lado, y))
	m.Resize(fyne.NewSize(lado, lado))
	m.Show()
}

// pintarResalteComentario enmarca la celda del hilo con el puntero encima en el panel.
func (rr *rejillaRenderer) pintarResalteComentario(vw vista) {
	k, ok := rr.r.celdaResaltada()
	if !ok || vw.filas.oculta(k.F) {
		marco(rr.marcoComentario, MarcaComentario, fyne.Position{}, fyne.Size{}, 0, 0, 0, false)
		return
	}
	w, h, _ := vw.caja(k.C, k.F)
	finX, finY := vw.finFijas()
	var minX, minY float32
	if k.C >= vw.fc {
		minX = finX
	}
	if k.F >= vw.ff {
		minY = finY
	}
	marco(rr.marcoComentario, MarcaComentario, fyne.NewPos(vw.x(k.C), vw.y(k.F)), fyne.NewSize(w, h), GrosorSeleccion*vw.z, minX, minY, true)
}
