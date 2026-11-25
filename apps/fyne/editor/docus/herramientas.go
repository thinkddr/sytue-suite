// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Lo de la tanda del 2026-11-25 en el folio: aumentar y reducir la letra, ordenar
// párrafos e insertar la fecha y la hora, como en la web (`canvas/exec.ts` e
// `Insertar.tsx`), con los mismos vectores (`docus-herramientas.json`).

// PasoDeTalla es `pasoDeTalla`: el peldaño de `fontSize` de al lado, sin pasar de 1 ni de
// 7. Indeterminado («») cuenta como la talla normal, «3».
func PasoDeTalla(actual string, dir int) string {
	n := 3
	if len(actual) == 1 && actual[0] >= '1' && actual[0] <= '7' {
		n = int(actual[0] - '0')
	}
	return string(rune('0' + min(7, max(1, n+dir))))
}

// herramienta son las órdenes de esta tanda sobre la selección [a, b]: un peldaño de letra
// más o menos y ordenar párrafos. Devuelve false si no es una de ellas.
func (f *Folio) herramienta(c editor.Comando, valor string, a, b punto) bool {
	switch c {
	case editor.AumentarLetra, editor.ReducirLetra:
		actual := f.Formato().Tamano
		dir := map[bool]int{true: 1, false: -1}[c == editor.AumentarLetra]
		if paso := PasoDeTalla(actual, dir); paso != actual {
			f.porTramos(func(e *rico.Edicion, d, h int) { e.PonerPropiedad(d, h, "font-size", tamanoCSS(paso)) })
		}
	case editor.OrdenarParrafos:
		f.ordenarParrafos(a.b, b.b, valor == "desc")
	default:
		return false
	}
	return true
}

// idiomaDeOrden es el del documento si lo tiene y si no el de la interfaz: el mismo
// `lang` que la web pone al folio.
func (f *Folio) idiomaDeOrden() i18n.Locale {
	if i18n.IsLocale(f.pagina.Idioma) {
		return i18n.Locale(f.pagina.Idioma)
	}
	if f.ctx != nil && i18n.IsLocale(string(f.ctx.Loc)) {
		return f.ctx.Loc
	}
	return i18n.Locale("es")
}

// ordenarParrafos es «Ordenar párrafos» sobre los bloques [desde, hasta] de la selección.
func (f *Folio) ordenarParrafos(desde, hasta int, desc bool) {
	f.antesDeCambiar(false)
	if l, ok := f.doc.OrdenarBloques(f.doc.Bloques(), desde, hasta, desc, f.idiomaDeOrden().Tag()); ok {
		f.irA(l, 0)
	}
	f.reestructurado()
}

// FormatosFechaHora es `formatosFechaHora`: el AHORA en los cuatro formatos que se
// ofrecen, ya escritos en el idioma de quien escribe.
func FormatosFechaHora(l i18n.Locale, ahora time.Time) []string {
	return []string{
		i18n.FormatDate(l, ahora),
		i18n.FormatDateLong(l, ahora),
		i18n.FormatFullDate(l, ahora),
		i18n.FormatTime(l, ahora),
	}
}

// botonFechaHora es «Insertar fecha y hora» de Elementos: un emergente con las cuatro
// formas de ahora, que se escriben como texto fijo en el cursor.
func botonFechaHora(c *editor.Contexto, inerte func(*editor.BotonCinta) *editor.BotonCinta) *editor.BotonCinta {
	return inerte(editor.NuevoBotonEmergente(iconos.EdHistorial, c.Traducir("insert.dateTime"), func(cerrar func()) fyne.CanvasObject {
		opciones := FormatosFechaHora(c.Loc, time.Now())
		botones := make([]fyne.CanvasObject, len(opciones))
		for i, texto := range opciones {
			botones[i] = editor.NuevoBotonPequeno(texto, func() {
				cerrar()
				c.Mandar(editor.InsertarTexto, texto)
			})
		}
		return container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, botones...)
	}))
}

// rotulosOrden son las dos órdenes de «Ordenar párrafos», con los rótulos de Datos.
var rotulosOrden = []struct {
	valor, clave string
	icono        iconos.Ed
}{
	{"asc", "data.sortAsc", iconos.EdOrdenAsc},
	{"desc", "data.sortDesc", iconos.EdOrdenDesc},
}

// botonOrdenar es «Ordenar párrafos» (`OrdenarParrafosMenu` de la web).
func botonOrdenar(c *editor.Contexto) *editor.BotonCinta {
	b := c.Boton(iconos.EdOrdenAsc, "paragraph.sort", false, editor.OrdenarParrafos)
	var menu *ui.Menu
	b.AlPulsar = func() {
		if menu != nil && menu.Abierto() {
			menu.Cerrar()
			return
		}
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		elems := make([]fyne.CanvasObject, 0, len(rotulosOrden))
		for _, o := range rotulosOrden {
			e := ui.NuevoElementoMenu(c.Traducir(o.clave), iconos.Ninguno, func() { c.Mandar(editor.OrdenarParrafos, o.valor) })
			e.Denso, e.Glifo, e.TamIcono = true, iconos.TrazoEd(o.icono, editor.IconoBotonCinta, "text", v), editor.IconoBotonCinta
			elems = append(elems, e)
		}
		menu = ui.NuevoMenu(elems...)
		menu.Rotulo = b.Rotulo
		menu.Mostrar(b)
	}
	return b
}
