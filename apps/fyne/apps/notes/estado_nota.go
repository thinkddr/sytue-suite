// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// EL PIE DEL EDITOR: cuántas palabras y letras tiene la nota y cuánto se tarda en leerla
// («128 palabras · 734 caracteres · 1 min de lectura»), en `text-xs text-subtle` al pie
// del cuerpo y a la derecha. Con algo seleccionado cuenta lo seleccionado, que es lo que
// se pregunta al seleccionar un párrafo.
//
// Las cuentas se rehacen SOLO cuando el documento o la selección cambian, no en cada
// repintado: mover el cursor sin seleccionar no las toca.

// PalabrasPorMinuto es el ritmo de lectura del pie: el de lectura en pantalla de un
// adulto, el que usan los editores para su «min de lectura».
const PalabrasPorMinuto = 200

// Medidas del pie: `px-3 py-1`, la caja de `text-xs`.
var (
	pyPieEstado = tema.Espacio(1)
	altoPie     = tema.TextoXS.Linea + 2*pyPieEstado
)

// cuentaTexto son las palabras y los caracteres de un texto: una palabra es un tramo sin
// blancos con al menos una letra o una cifra (un «—» suelto no lo es), y un carácter,
// cualquier letra que no sea un salto de línea ni el hueco de una imagen.
func cuentaTexto(s string) (palabras, caracteres int) {
	for _, r := range s {
		if r != '\n' && r != Objeto {
			caracteres++
		}
	}
	for _, f := range strings.Fields(s) {
		if strings.IndexFunc(f, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) >= 0 {
			palabras++
		}
	}
	return palabras, caracteres
}

// cuentaDocumento cuenta lo escrito en el documento: no el hueco que la casilla de una
// tarea deja delante de su texto, ni las reglas.
func cuentaDocumento(d *Documento) (palabras, caracteres int) {
	for _, b := range d.Bloques {
		if !editable(b) {
			continue
		}
		r := []rune(b.Texto())
		p, c := cuentaTexto(string(r[min(inicioEditable(b), len(r)):]))
		palabras, caracteres = palabras+p, caracteres+c
	}
	return palabras, caracteres
}

// minutosLectura es lo que se tarda en leer tantas palabras, redondeado hacia arriba:
// una nota de diez palabras es «1 min», y una vacía, cero.
func minutosLectura(palabras int) int {
	return (palabras + PalabrasPorMinuto - 1) / PalabrasPorMinuto
}

// textoEstado es la línea del pie para estas cuentas.
func textoEstado(tr *i18n.Translator, palabras, caracteres int) string {
	return strings.Join([]string{
		tr.T("editor.words", i18n.Vars{"count": palabras}),
		tr.T("editor.chars", i18n.Vars{"count": caracteres}),
		tr.T("editor.readingTime", i18n.Vars{"minutes": minutosLectura(palabras)}),
	}, " · ")
}

// pieEstado es el pie del editor.
type pieEstado struct {
	widget.BaseWidget
	e     *EditorNota
	texto string
	// visto es lo último que se contó: el documento y la selección. Si no cambian, no se
	// vuelve a contar.
	visto      *Documento
	vistoA     Pos
	vistoB     Pos
	conCuentas bool
	// cambiosVistos es el `EditorNota.cambios` de la última cuenta.
	cambiosVistos int
	linea         *canvas.Text
}

var _ fyne.Accessible = (*pieEstado)(nil)

// conPie le pone el pie al editor. Es de Notepads: el redactor de Mail no lo lleva.
func (e *EditorNota) conPie() {
	if e.estado != nil {
		return
	}
	p := &pieEstado{e: e}
	p.ExtendBaseWidget(p)
	e.estado = p
	e.refrescarEstado()
}

// refrescarEstado rehace las cuentas si el documento o la selección han cambiado.
func (e *EditorNota) refrescarEstado() {
	p := e.estado
	if p == nil {
		return
	}
	a, b := e.seleccion()
	if p.conCuentas && p.visto == e.doc && p.vistoA == a && p.vistoB == b && !p.sucio() {
		return
	}
	var palabras, caracteres int
	if a != b {
		palabras, caracteres = cuentaTexto(e.doc.TextoEntre(a, b))
	} else {
		palabras, caracteres = cuentaDocumento(e.doc)
	}
	p.visto, p.vistoA, p.vistoB, p.conCuentas = e.doc, a, b, true
	p.limpio()
	if t := textoEstado(e.tr, palabras, caracteres); t != p.texto {
		p.texto = t
		p.Refresh()
	}
}

// sucio y limpio llevan la cuenta de los cambios en el MISMO documento: el editor
// escribe sobre el modelo en su sitio, así que el puntero no basta.
func (p *pieEstado) sucio() bool { return p.e.cambios != p.cambiosVistos }
func (p *pieEstado) limpio()     { p.cambiosVistos = p.e.cambios }

// AccessibilityLabel es la línea entera.
func (p *pieEstado) AccessibilityLabel() string { return p.texto }

// AccessibilityRole: es texto que se lee.
func (p *pieEstado) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

func (p *pieEstado) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.linea = ui.Texto(p.texto, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	return &pieEstadoRenderer{p: p}
}

type pieEstadoRenderer struct{ p *pieEstado }

func (r *pieEstadoRenderer) MinSize() fyne.Size { return fyne.NewSize(0, altoPie) }
func (r *pieEstadoRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.p.linea}
}
func (r *pieEstadoRenderer) Destroy() {}

func (r *pieEstadoRenderer) Layout(t fyne.Size) {
	l := r.p.linea
	ancho := ui.Truncar(l, r.p.texto, tema.TextoXS, ui.Normal, max(t.Width-2*PXZonaEd, 0))
	ui.ColocarTexto(l, tema.TextoXS, max(t.Width-PXZonaEd-ancho, PXZonaEd), pyPieEstado)
}

func (r *pieEstadoRenderer) Refresh() {
	r.p.linea.Color = tema.Color("text-subtle", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.Layout(r.p.Size())
	r.p.linea.Refresh()
}
