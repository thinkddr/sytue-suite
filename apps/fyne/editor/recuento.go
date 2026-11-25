// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Recuento son las cifras del «Recuento de palabras» de Docus (`CifrasTexto` de
// `editor/recuento.ts`).
type Recuento struct {
	Palabras int
	// Caracteres son puntos de código, sin los saltos de párrafo.
	Caracteres            int
	CaracteresSinEspacios int
	// Parrafos son las líneas con algo que no sea blanco.
	Parrafos int
}

// LienzoConRecuento es el lienzo que sabe contar el documento y la selección: el folio
// de Docus. Es aparte de [Lienzo] porque solo Docus tiene recuento, como en la web.
type LienzoConRecuento interface {
	// Recuento son las cifras del documento —con las palabras de la barra, las de
	// [Lienzo.Estadisticas], para que barra y diálogo no se contradigan— y las de la
	// selección, o nil si no hay.
	Recuento() (documento Recuento, seleccion *Recuento)
	// PalabrasSeleccionadas son las de la selección, y si la hay. Va aparte porque la
	// barra la pide en cada repintado y contar el documento entero ahí sobraría.
	PalabrasSeleccionadas() (palabras int, hay bool)
}

// RecuentoDelTexto cuenta un texto plano con los párrafos separados por saltos de
// línea. Es `cifrasDelTexto` de la web, con sus mismos vectores
// (`packages/office/test/vectores/docus-recuento.json`): la misma selección no puede
// dar dos números según dónde se mire.
func RecuentoDelTexto(texto string) Recuento {
	var r Recuento
	for _, linea := range strings.FieldsFunc(texto, func(c rune) bool { return c == '\n' || c == '\r' }) {
		conTinta := false
		for _, c := range linea {
			if !EsBlancoJS(c) {
				r.CaracteresSinEspacios++
				conTinta = true
			}
		}
		r.Caracteres += utf8.RuneCountInString(linea)
		if conTinta {
			r.Parrafos++
			r.Palabras += len(strings.FieldsFunc(linea, EsBlancoJS))
		}
	}
	return r
}

// MinutosDeLectura es `minutosDeLectura`: a 200 palabras por minuto, hacia arriba, el
// ritmo con el que Notes dice su «x min». Sin palabras, cero.
func MinutosDeLectura(palabras int) int {
	return (max(0, palabras) + 199) / 200
}

// TextoPlanoDeDescarga es `textoPlanoDeDescarga`: una línea por párrafo, sin las vacías
// ni el blanco del final de cada una, y un salto al final. Sale del mismo texto que el
// recuento, con los mismos vectores (`docus-herramientas.json`).
func TextoPlanoDeDescarga(texto string) string {
	var lineas []string
	for _, l := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(texto, "\r\n", "\n"), "\r", "\n"), "\n") {
		l = strings.TrimRightFunc(l, EsBlancoJS)
		if strings.TrimFunc(l, EsBlancoJS) != "" {
			lineas = append(lineas, l)
		}
	}
	if len(lineas) == 0 {
		return ""
	}
	return strings.Join(lineas, "\n") + "\n"
}

// EsBlancoJS es el `\s` de JavaScript y no `unicode.IsSpace`: el de Go toma también
// U+0085 y deja fuera U+FEFF, y con eso una misma selección contaría distinto. Es
// exportado porque «Limpiar texto» de Calcus recorta con la misma regla que la web.
func EsBlancoJS(c rune) bool {
	switch c {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return c >= 0x2000 && c <= 0x200a
}

// AtajoRecuento es Ctrl/Cmd+Mayús+C, el de Docs de Google: abre el recuento. Solo en
// Docus; en Calcus y Diapos no es suyo y se deja pasar.
func (c *Contexto) AtajoRecuento(s *desktop.CustomShortcut) bool {
	if c == nil || s == nil || s.KeyName != fyne.KeyC ||
		s.Modifier != fyne.KeyModifierShortcutDefault|fyne.KeyModifierShift ||
		c.Tipo != Documento || c.AbrirRecuento == nil {
		return false
	}
	c.AbrirRecuento()
	return true
}

// abrirRecuento enseña el diálogo con las cifras de AHORA: es una foto, no se rehace
// mientras se lee (como en la web).
func (b *BarraEstado) abrirRecuento() {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(b)
	if lienzo == nil || b.ctx.Tipo != Documento {
		return
	}
	_, paginas, palabras := b.cifras()
	documento := Recuento{Palabras: palabras}
	var seleccion *Recuento
	if l, ok := b.ctx.Lienzo.(LienzoConRecuento); ok {
		documento, seleccion = l.Recuento()
	}
	tr := b.ctx.Traducir
	d := ui.NuevoDialogo(tr("wordcount.title"), CuerpoRecuento(b.ctx, paginas, documento, seleccion)).Con(ui.DialogoMD, nil)
	d.RotuloCerrar = tr("ui.close")
	d.Mostrar(lienzo)
}

// CuerpoRecuento es la tabla del diálogo: un dato por fila y, con selección, una
// columna para ella antes de la del documento (RecuentoDialog.tsx).
func CuerpoRecuento(ctx *Contexto, paginas int, documento Recuento, seleccion *Recuento) fyne.CanvasObject {
	tr := ctx.Traducir
	num := func(n int) string { return i18n.FormatNumber(ctx.Loc, float64(n)) }
	type fila struct {
		clave    string
		doc, sel int
		sinSel   bool
	}
	s := Recuento{}
	if seleccion != nil {
		s = *seleccion
	}
	filas := []fila{
		// Las páginas de una selección no se cuentan sin maquetarla aparte: celda vacía
		// antes que un número inventado.
		{"wordcount.pages", paginas, 0, true},
		{"wordcount.words", documento.Palabras, s.Palabras, false},
		{"wordcount.characters", documento.Caracteres, s.Caracteres, false},
		{"wordcount.charactersNoSpaces", documento.CaracteresSinEspacios, s.CaracteresSinEspacios, false},
		{"wordcount.paragraphs", documento.Parrafos, s.Parrafos, false},
		{"wordcount.readingTime", MinutosDeLectura(documento.Palabras), MinutosDeLectura(s.Palabras), false},
	}
	// La cabecera del rótulo va vacía: en la web es `sr-only`, y aquí el nombre ya lo da el
	// título del diálogo. Las cifras miden lo que su cabecera y el rótulo se queda el
	// resto, que «Caracteres sin espacios» no quepa partido entre tres.
	columnas := []ui.ColumnaTabla{{Clave: "dato"}}
	cifra := func(clave, rotulo string) ui.ColumnaTabla {
		versal := ui.Versalitas(rotulo)
		ancho := ui.Medir(versal, tema.TextoXS, ui.Negrita).Width +
			float32(utf8.RuneCountInString(versal))*ui.TrackingOverline*tema.TextoXS.Tam + 2*ui.MargenCelda + tema.Borde
		return ui.ColumnaTabla{Clave: clave, Rotulo: rotulo, Alineacion: ui.Fin, Ancho: max(ancho, AnchoCifraRecuento)}
	}
	if seleccion != nil {
		columnas = append(columnas, cifra("seleccion", tr("wordcount.selection")))
	}
	columnas = append(columnas, cifra("documento", tr("wordcount.document")))
	tabla := ui.NuevaTabla(columnas, func() int { return len(filas) }, func(i, col int) string {
		f := filas[i]
		switch columnas[col].Clave {
		case "dato":
			return tr(f.clave)
		case "seleccion":
			if f.sinSel {
				return ""
			}
			return num(f.sel)
		}
		return num(f.doc)
	})
	// La tabla de la suite virtualiza el cuerpo y pide de mínimo una fila: aquí son
	// cinco fijas y se ven enteras, sin desplazamiento dentro del diálogo.
	alto := ui.AltoCabeceraTabla + float32(len(filas))*ui.AltoFilaTabla() + 2*tema.Borde
	return container.New(altoFijo(alto), tabla)
}

// AnchoCifraRecuento es el mínimo de una columna de cifras: un número de siete cifras
// con sus separadores y el `px-3` de cada lado.
const AnchoCifraRecuento float32 = 96

// altoFijo pide ese alto y le da todo el sitio a lo que lleva dentro.
type altoFijo float32

func (a altoFijo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, float32(a)) }

func (a altoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}
