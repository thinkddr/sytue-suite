// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// # El folio sin página: el texto de una caja
//
// Una caja de texto de Diapos es, en la web, un `contenteditable` como el folio de Docus:
// el mismo HTML, el mismo `exec` de la cinta y el mismo navegador repartiendo líneas. Lo
// que cambia es la hoja (`BOX_CSS` de SlidesCanvas.tsx en vez de `editor.css`) y que no
// hay página: la caja mide lo que mide, a la escala del escenario, y lo que no cabe se
// recorta (`overflow: hidden`).
//
// Por eso no hay un segundo editor de texto enriquecido: es ESTE folio, con [Caja] en vez
// de [Pagina]. Escribir, el cursor, la selección, el portapapeles, deshacer y los
// comandos de la cinta son los mismos, y los píxeles de una caja que se escribe son los
// de la que no ([PintarCaja] pinta con las mismas piezas).

// Caja es la geometría y la hoja del folio de una caja.
type Caja struct {
	// Ancho y Alto son los de la caja en px de su lienzo, sin escalar.
	Ancho, Alto float32
	// Escala es la del lienzo que la aloja (el `transform: scale(k)` del escenario).
	Escala float32
	// Relleno es el `padding` de la izquierda y la derecha, en px del lienzo.
	Relleno float32
	// Hoja es la del contenedor (ver [rico.Hoja]).
	Hoja *rico.Hoja
	// Rotulo es el `aria-label` de la caja.
	Rotulo string
	// AlSalir es Escape; AlPerderFoco, cuando el foco se va (el `blur` de la web).
	AlSalir, AlPerderFoco func()
}

// NuevoFolioCaja crea el folio de una caja. El contexto es el del folio, no el del
// documento que aloja la caja: de él se leen el permiso y a quién avisar de que cambia el
// formato (`Refrescar`).
func NuevoFolioCaja(ctx *editor.Contexto, c Caja) *Folio {
	f := NuevoFolio(ctx)
	f.caja = &c
	f.scroll.Direction = container.ScrollNone // recorta, y no se desplaza
	return f
}

// PonerCaja cambia la geometría de la caja (el zoom del escenario, una caja que crece) y
// vuelve a repartir las líneas.
func (f *Folio) PonerCaja(c Caja) {
	anterior := f.caja
	f.caja = &c
	if anterior == nil || anterior.Ancho != c.Ancho || anterior.Relleno != c.Relleno {
		f.remaquetar()
	}
	f.pag.Refresh()
	f.scroll.Refresh()
}

// hoja es la del contenedor, o nil en el folio de Docus.
func (f *Folio) hoja() *rico.Hoja {
	if f.caja == nil {
		return nil
	}
	return f.caja.Hoja
}

// AlFinal pone el cursor al final del texto, sin selección: es donde lo deja la web al
// empezar a escribir en una caja (`range.collapse(false)`).
func (f *Folio) AlFinal() {
	n := len(f.bloques) - 1
	f.mover(f.acotar(punto{n, f.largo(n)}), false)
}

// Enfocar le da el foco del teclado.
func (f *Folio) Enfocar() { f.enfocar() }

// PintarCaja son los objetos del HTML maquetado en la caja `c`, con su esquina en
// `origen` (px de pantalla): lo mismo que pinta su folio mientras se escribe, sin cursor
// ni selección. Lo que la caja no puede enseñar (`overflow: hidden`) lo recorta quien la
// aloja; aquí se devuelven también las líneas que no caben.
func PintarCaja(html string, c Caja, origen fyne.Position) []fyne.CanvasObject {
	doc := rico.ParseCon(html, c.Hoja)
	cajas, _ := maquetar(doc.Bloques(), max(c.Ancho-2*c.Relleno, 1))
	f := &Folio{}
	usadas := map[string]*canvas.Image{}
	return f.pintarCajas(nil, cajas, origen.X+c.Relleno*c.Escala, origen.Y, c.Escala, usadas)
}

// TextoSeleccionado es la selección como texto plano: lo que el panel de «Enlace» ofrece
// como texto visible.
func (f *Folio) TextoSeleccionado() string { return f.textoSeleccionado() }
