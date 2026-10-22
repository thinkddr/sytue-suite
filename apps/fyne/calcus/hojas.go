// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las operaciones de la barra de hojas (`switchSheet`, `addSheet`, `renameSheet`,
// `duplicateSheet`, `deleteSheet` y `moveSheetTo` en SheetCanvas.tsx). Viven en el
// lienzo porque tocan las tres piezas: el libro cambia, la rejilla vuelve a A1 y la
// barra de fórmulas se entera.
//
// Las de ESTRUCTURA no se hacen en solo lectura. En la web las veta la protección del
// libro (`guardStructure`); aquí además el documento que no se entiende, que es el
// otro motivo por el que un lienzo se cierra.

// CambiarHoja activa la hoja i. Cambiar de hoja se puede siempre, también en solo
// lectura: es mirar, no escribir.
func (c *Lienzo) CambiarHoja(i int) {
	if i == c.Libro.Activa() {
		return
	}
	c.Libro.Activar(i)
	c.trasCambiarHoja()
}

// AnadirHoja crea «Hoja N» al final, con N el número de hojas más uno, como la web.
func (c *Lienzo) AnadirHoja() {
	if c.Rejilla.SoloLectura || !c.estructura() {
		return
	}
	c.Libro.Anadir(c.t("sheet.defaultName", i18n.Vars{"n": len(c.Libro.Hojas()) + 1}))
	c.trasCambiarHoja()
}

// DuplicarHoja copia la activa justo detrás.
func (c *Lienzo) DuplicarHoja() {
	if c.Rejilla.SoloLectura || !c.estructura() {
		return
	}
	c.Libro.Duplicar()
	c.trasCambiarHoja()
}

// BorrarHoja quita la activa sin preguntar; PedirBorrarHoja es la que pregunta.
func (c *Lienzo) BorrarHoja() {
	if c.Rejilla.SoloLectura || len(c.Libro.Hojas()) <= 1 || !c.estructura() {
		return
	}
	c.Libro.Borrar()
	c.trasCambiarHoja()
}

// RenombrarHoja pone otro nombre a la hoja i; PedirRenombrarHoja es la que lo pide.
func (c *Lienzo) RenombrarHoja(i int, nombre string) {
	if c.Rejilla.SoloLectura || !c.estructura() {
		return
	}
	c.Libro.Renombrar(i, nombre)
	c.Pestanas.Refresh()
	c.avisar()
}

// MoverHoja lleva la hoja `de` a la posición `a`.
func (c *Lienzo) MoverHoja(de, a int) {
	if c.Rejilla.SoloLectura || de == a || !c.estructura() {
		return
	}
	c.Libro.Mover(de, a)
	c.Pestanas.Refresh()
	c.avisar()
}

// PedirBorrarHoja es `deleteSheet`: una confirmación de peligro con el nombre de la
// hoja, porque su contenido se pierde.
func (c *Lienzo) PedirBorrarHoja() {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(c)
	if lienzo == nil || c.Rejilla.SoloLectura || len(c.Libro.Hojas()) <= 1 {
		return
	}
	// useConfirm.tsx: `<p class="text-text text-sm leading-relaxed">`, repartido al
	// ancho que tenga el cuerpo (sin ancho fijo: en la hoja de 390 pedía 350 en 342).
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relajado := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
	cuerpo := ui.NuevoParrafo(c.t("dialog.sheet.delete", i18n.Vars{"name": c.Libro.Hoja()}),
		relajado, ui.Normal, "text", v)
	var d *ui.Dialogo
	d = ui.Confirmacion(c.t("sheet.delete", nil), c.t("ui.cancel", nil), c.t("sheet.delete", nil), true, cuerpo,
		func() { d.Cerrar() },
		func() { d.Cerrar(); c.BorrarHoja() })
	d.RotuloCerrar = c.t("ui.close", nil)
	d.Mostrar(lienzo)
}

// PedirRenombrarHoja es `renameSheet`: el diálogo de pregunta con el nombre actual.
func (c *Lienzo) PedirRenombrarHoja(i int) {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(c)
	if lienzo == nil || c.Rejilla.SoloLectura || i < 0 || i >= len(c.Libro.Hojas()) || !c.estructura() {
		return
	}
	d := ui.Pregunta(c.t("dialog.sheet.rename", nil), c.Libro.Hojas()[i], c.t("ui.cancel", nil),
		c.t("ui.save", nil), c.t("ui.required", nil), func(nombre string) { c.RenombrarHoja(i, nombre) })
	d.RotuloCerrar = c.t("ui.close", nil)
	d.Mostrar(lienzo)
}

// trasCambiarHoja es lo que la web hace tras cada cambio de hoja: selección a A1, sin
// rango ni celda abierta.
func (c *Lienzo) trasCambiarHoja() {
	c.Rejilla.Cancelar()
	c.Rejilla.desX, c.Rejilla.desY = 0, 0
	c.Rejilla.Seleccionar(0, 0, false)
	c.avisar()
}

func (c *Lienzo) avisar() {
	if c.AlCambiar != nil {
		c.AlCambiar()
	}
}
