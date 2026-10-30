// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image/color"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Escribir en una caja de texto
//
// En la web, doble clic sobre una caja de texto la vuelve `contenteditable`: se escribe
// encima, con su letra y su formato, la cinta formatea lo elegido (`exec`), y al salir
// —Escape o perder el foco— el HTML se guarda (`commitBoxHtml`).
//
// Aquí la caja se escribe con el folio de Docus en modo caja (`docus.NuevoFolioCaja`),
// que es el mismo editor de texto enriquecido que el de un documento: cursor, selección,
// teclado, portapapeles, deshacer y los comandos de la cinta. Se coloca exactamente donde
// el lienzo pinta la caja, con la hoja del lienzo y a la escala del escenario, así que lo
// que se ve al escribir es lo que se ve al terminar.
//
// El documento de la gente no se corrompe: el folio solo escribe un HTML que entiende
// ENTERO, y el que no entiende no se abre para escribir (se dice, y no se toca).

// escritura es la caja que se está escribiendo y su folio.
type escritura struct {
	id    string
	folio *docus.Folio
}

// EmpezarEscritura es `startEditBox`: la caja elegida pasa a escribirse, con el cursor
// al final.
func (e *Editor) EmpezarEscritura(id string) {
	c := e.m.caja(e.objetivo(), id)
	if c == nil || !esTexto(*c) || e.ctx.SoloLectura {
		return
	}
	e.TerminarEscritura()
	e.sel, e.extra = id, nil
	w := &escritura{id: id}
	ctx := &editor.Contexto{Tipo: editor.Presentacion, T: e.ctx.T, Refrescar: e.refrescarCinta}
	// Nace ya con la medida de la caja (en px del escenario, a escala 1) para repartir las
	// líneas una sola vez; la capa lo pone luego a la escala de verdad.
	tam := fyne.NewSize(float32(c.W)/100*AnchoEscenario, float32(c.H)/100*e.m.AltoEscenario())
	w.folio = docus.NuevoFolioCaja(ctx, w.caja(e, tam, 1, e.m.Visuales(e.activa).Tinta))
	if e.Programar != nil {
		w.folio.Programar = e.Programar
	}
	if !w.folio.Cargar(c.HTML) {
		// Algo que el folio no sabe escribir sin perderlo (un atributo que pegó el
		// navegador, una etiqueta rara): se enseña, pero no se toca.
		e.avisos.Notificar(ui.NuevoAviso(ui.AvisoAdvertencia, e.t("editor.readOnly"), ""))
		e.recomponer()
		return
	}
	e.escribiendo = w
	e.escenario.Oculta = id
	e.recomponer()
	w.folio.AlFinal()
	w.folio.Enfocar()
	e.refrescarCinta()
}

// caja es la geometría del folio para la caja pintada en `tam` a la escala `k`, con la
// tinta de la diapositiva y lo que hace al salir.
func (w *escritura) caja(e *Editor, tam fyne.Size, k float32, tinta color.NRGBA) docus.Caja {
	c := cajaFolio(tam, k, hojaCaja(tinta, e.m.Visuales(e.activa).Familia, ""))
	c.AlDesplazar = e.desliz.Scrolled
	c.Rotulo = e.t("slides.textBox")
	c.AlSalir = func() {
		e.TerminarEscritura()
		e.capa.enfocar() // Escape devuelve el teclado al escenario
	}
	c.AlPerderFoco = func() {
		// Perder el foco NO termina —la cinta se lo lleva al pulsarla y sus comandos son
		// para esta caja—; se guarda lo escrito, como el `blur` de la web. Terminan
		// Escape, pulsar fuera de la caja en el escenario o cambiar de diapositiva.
		if e.escribiendo == w {
			w.consolidar(e)
		}
	}
	return c
}

// refrescarCinta rehace el cromo: la cinta enseña el formato de lo elegido.
func (e *Editor) refrescarCinta() {
	if e.ctx.Refrescar != nil {
		e.ctx.Refrescar()
	}
}

// TerminarEscritura es `commitBoxHtml`: si el HTML ha cambiado, se guarda.
func (e *Editor) TerminarEscritura() {
	w := e.escribiendo
	if w == nil {
		return
	}
	e.escribiendo = nil
	e.escenario.Oculta = ""
	nuevo := w.folio.Serializar()
	if c := e.m.caja(e.objetivo(), w.id); c != nil && c.HTML != nuevo {
		e.cambiar(func() { c.HTML = nuevo }, true)
	}
	e.recomponer()
	e.refrescarCinta()
}

// Escribiendo dice qué caja se está escribiendo, o "".
func (e *Editor) Escribiendo() string {
	if e.escribiendo == nil {
		return ""
	}
	return e.escribiendo.id
}

// Folio es el editor de la caja que se escribe, o nil.
func (e *Editor) Folio() *docus.Folio {
	if e.escribiendo == nil {
		return nil
	}
	return e.escribiendo.folio
}

// colocar pone el folio encima de la caja, que es donde el lienzo la pinta.
func (w *escritura) colocar(e *Editor, pos fyne.Position, tam fyne.Size, k float32, tinta color.NRGBA) fyne.CanvasObject {
	w.folio.PonerCaja(w.caja(e, tam, k, tinta))
	o := w.folio.Objeto()
	o.Move(pos)
	o.Resize(tam)
	return o
}

// consolidar escribe en el modelo lo que va escrito, sin dejar de escribir: es lo que
// pasa al perder el foco, para que el autoguardado lo lleve.
func (w *escritura) consolidar(e *Editor) {
	nuevo := w.folio.Serializar()
	if c := e.m.caja(e.objetivo(), w.id); c != nil && c.HTML != nuevo {
		e.cambiarLigero(func() { c.HTML = nuevo })
	}
}
