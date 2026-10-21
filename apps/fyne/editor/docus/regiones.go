// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// El ENCABEZADO y el PIE en el folio: se pintan en el margen, con su raya, y se escriben
// como cualquier línea. El HTML es de `rico` (regiones.go).
//
// ponytail: como en la web, el folio es continuo y hay UN encabezado —arriba del primer
// folio— y UN pie —abajo del último—, no uno por página.

// pintarRegion pinta la raya de la región —debajo del encabezado, encima del pie— y, vacía,
// su nombre a media tinta (`:empty::before { content: …; opacity: .4 }`).
func (f *Folio) pintarRegion(objs []fyne.CanvasObject, c caja, ox, oy, z float32) []fyne.CanvasObject {
	y := c.y
	if c.b.Region == rico.Encabezado {
		y = c.y + c.alto - 1
	}
	raya := canvas.NewRectangle(rico.TintaLinea)
	raya.Move(fyne.NewPos(ox+c.x*z, oy+y*z))
	raya.Resize(fyne.NewSize(c.ancho*z, max(z, 1)))
	objs = append(objs, raya)
	if !c.b.Vacio() || f.ctx == nil || len(c.lineas) == 0 {
		return objs
	}
	clave := map[string]string{rico.Encabezado: "insert.header", rico.Pie: "insert.footer"}[c.b.Region]
	e := c.b.Base
	e.Color.A = uint8(float32(e.Color.A)*0.4 + 0.5)
	t := textoContenido(f.ctx.Traducir(clave), e, z)
	base := c.y + c.arribaTexto + c.lineas[0].base
	t.Move(fyne.NewPos(ox+c.izq*z, oy+(base-ascFuente*e.Tamano)*z))
	return append(objs, t)
}

// enRegion dice si el cursor está en el encabezado o en el pie.
func (f *Folio) enRegion() bool {
	return f.cur.b >= 0 && f.cur.b < len(f.bloques) && f.bloques[f.cur.b].Region != ""
}

// regionEn es la región visible que cubre la altura my (px de maqueta), o -1.
func (f *Folio) regionEn(my float32) int {
	for i, c := range f.cajas {
		if c.b.Region != "" && !c.b.Oculta && my >= c.y && my < c.y+c.alto {
			return i
		}
	}
	return -1
}

// region es «Encabezado» o «Pie de página» (`ensureRegion`): la crea si no hay y deja el
// cursor al final de lo que tenga.
func (f *Folio) region(cual string) {
	foto := paso{f.doc.HTML(), f.cur, f.anc}
	antes := len(f.doc.Bloques())
	l, ok := f.doc.Region(cual)
	if !ok {
		return
	}
	creada := len(f.doc.Bloques()) != antes
	f.bloques = f.doc.Bloques()
	i := rico.Indice(f.bloques, l)
	if i < 0 {
		return
	}
	if creada {
		f.apuntar(foto, false)
		f.ed = nil
		f.cambiado()
	}
	f.mover(punto{i, f.largo(i)}, false)
}

// primeraDistinta es «Primera página distinta»: esconde (o enseña) el encabezado.
func (f *Folio) primeraDistinta() {
	foto := paso{f.doc.HTML(), f.cur, f.anc}
	if !f.doc.AlternarPrimeraDistinta() {
		return
	}
	f.apuntar(foto, false)
	if f.enRegion() && f.bloques[f.cur.b].Region == rico.Encabezado {
		f.cur, f.anc = punto{f.cur.b + 1, 0}, punto{f.cur.b + 1, 0} // el cursor no se queda en lo que no se ve
	}
	f.reestructurado()
}

// campoPagina es «Número de página» (`insertPageField`), en el cursor.
func (f *Folio) campoPagina(texto string) {
	f.antesDeCambiar(false)
	f.borrarSeleccion()
	e := f.edicion(f.cur.b)
	e.InsertarCampoPagina(f.cur.p, texto)
	f.irA(e.Lugar, f.cur.p+1)
	f.reestructurado()
}

// regiones ejecuta las órdenes de «Encabezado y pie». Devuelve false si no es una.
func (f *Folio) regiones(c editor.Comando, valor string) bool {
	switch c {
	case editor.InsertarEncabezado:
		f.region(rico.Encabezado)
	case editor.InsertarPie:
		f.region(rico.Pie)
	case editor.InsertarNumeroPagina:
		f.campoPagina(valor)
	case editor.PrimeraPaginaDistinta:
		f.primeraDistinta()
	default:
		return false
	}
	return true
}

// soloEnLinea son las órdenes que caben en una región: lo que va dentro de su línea. Las
// demás meten bloques, y en una región partirían su `div`.
var soloEnLinea = map[editor.Comando]bool{
	editor.InsertarTexto: true, editor.InsertarImagen: true, editor.InsertarEnlace: true,
	editor.InsertarNumeroPagina: true, editor.InsertarEncabezado: true, editor.InsertarPie: true,
	editor.PrimeraPaginaDistinta: true,
	// Las de la imagen seleccionada: no meten nada nuevo.
	cmdAjusteImagen: true, cmdAlinearImagen: true, cmdAltImagen: true, cmdSinRecorte: true, cmdEliminarImagen: true,
	// Una cita va en la línea; las fuentes son datos del folio.
	cmdCitar: true, cmdFuentes: true,
	// La decoración es de la página, no de la región.
	cmdMarcaAgua: true, cmdBorde: true,
}
