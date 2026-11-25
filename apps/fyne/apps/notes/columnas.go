// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// LAS COLUMNAS DE UNA NOTA: dos o tres, una al lado de la otra, como las del editor de
// la web (`plantillaColumnas` de bloques.ts):
//
//	<div data-columnas="2"><div data-columna="">…</div><div data-columna="">…</div></div>
//
// En el modelo son una TABLA de una sola fila sin rejilla (`Tabla.Columnas`): cada
// columna es una celda, y sus bloques llevan su `Celda`. Así el cursor, las flechas, la
// selección, escribir y el deshacer funcionan dentro de una columna sin una línea más,
// igual que en una celda. Lo que no es de tabla —la rejilla, la barra de tabla, las
// filas— no se aplica a unas columnas.
//
// Dentro de una columna sí puede haber paneles y desplegables (en una celda no: la web
// no los mete ahí). Unas columnas dentro de una celda o de una caja no se leen: la nota
// se abre en solo lectura, que es lo honrado con lo que esta vista no sabe escribir.
//
// Cómo se ven, de app.css: `my-3 grid gap-4`, las columnas a partes iguales y, en el
// editor, una guía punteada a 4 px de cada una (`outline-offset: 4px`). En el teléfono
// (`max-width: 639px`) van una debajo de otra.

// ColumnasPermitidas son los valores de `data-columnas`.
var ColumnasPermitidas = []string{"2", "3"}

// Medidas de las columnas: `gap-4`, `my-3` y el `outline-offset` de la guía.
var (
	huecoColumnas  = tema.Espacio(4)
	margenColumnas = tema.Espacio(3)
	desfaseGuia    = float32(4)
	// anchoApilar es el ancho por debajo del cual, en el teléfono, las columnas van una
	// debajo de otra: el `max-width: 639px` de la web, que mira la PANTALLA y no la nota
	// (en el escritorio, una tarjeta estrecha sigue con sus columnas lado a lado).
	anchoApilar = float32(600)
)

// esColumnas dice si un elemento abre unas columnas: `div[data-columnas]` con un valor
// de la lista.
func esColumnas(n *xhtml.Node) bool {
	return n.DataAtom == atom.Div && slices.Contains(ColumnasPermitidas, atributo(n, "data-columnas"))
}

// admiteCaja dice si lo que se está leyendo puede abrir un panel o un desplegable: fuera
// de toda tabla o dentro de una columna.
func (d *Documento) admiteCaja() bool {
	return !d.celda.EnTabla() || d.celda.Tabla.EsColumnas()
}

// leerColumnas aplana unas columnas: una «tabla» de una fila con una celda por columna.
// Lo que no sea un `div[data-columna]` (texto suelto entre ellas) no se lee, así que no
// vuelve y la nota se abre en solo lectura.
func (d *Documento) leerColumnas(n *xhtml.Node) {
	t := &Tabla{Columnas: atributo(n, "data-columnas"), Atributos: n.Attr}
	fila := FilaTabla{Seccion: "tbody", Nueva: true}
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if h.Type != xhtml.ElementNode || h.DataAtom != atom.Div || !tiene(h, "data-columna") {
			continue
		}
		d.celda = Celda{Tabla: t, Fila: 0, Col: len(fila.Celdas)}
		fila.Celdas = append(fila.Celdas, CeldaTabla{Atributos: h.Attr})
		d.abrir(Bloque{Tipo: Parrafo})
		for c := h.FirstChild; c != nil; c = c.NextSibling {
			d.recorrer(c, Tramo{}, 0)
		}
		d.cerrarSuelto(Celda{})
	}
	t.Filas = []FilaTabla{fila}
	d.celda = Celda{}
	if len(fila.Celdas) == 0 {
		// Sin columnas dentro no hay dónde escribir: un párrafo vacío en su sitio, que al
		// reescribirse no sale igual (no es fiel).
		d.abrir(Bloque{Tipo: Parrafo, Etiqueta: "div"})
	}
}

// escribirColumnas escribe los bloques de unas columnas con su forma: el `data-columnas`
// y un `div[data-columna]` por columna, con los atributos tal como venían (la web solo
// pone esos dos).
func escribirColumnas(b *strings.Builder, t *Tabla, porCelda map[[2]int][]Bloque) {
	b.WriteString("<div" + atributos(t.Atributos) + ">")
	if len(t.Filas) > 0 {
		for c, celda := range t.Filas[0].Celdas {
			b.WriteString("<div" + atributos(celda.Atributos) + ">")
			escribirBloques(b, porCelda[[2]int{0, c}])
			b.WriteString("</div>")
		}
	}
	b.WriteString("</div>")
}

// InsertarColumnas pone unas columnas (2 o 3) vacías en el cursor, con un párrafo vacío
// detrás, como la tabla. Devuelve el cursor en la primera. Dentro de una tabla o de unas
// columnas no hace nada: no se anidan.
func (d *Documento) InsertarColumnas(p Pos, n int) Pos {
	valor := strconv.Itoa(n)
	if !slices.Contains(ColumnasPermitidas, valor) || len(d.Bloques) > 0 && d.Bloques[p.B].Celda.EnTabla() {
		return p
	}
	t := &Tabla{Columnas: valor, Atributos: []xhtml.Attribute{{Key: "data-columnas", Val: valor}}}
	fila := FilaTabla{Seccion: "tbody", Nueva: true}
	celdas := make([][][]Bloque, 1)
	for range n {
		fila.Celdas = append(fila.Celdas, CeldaTabla{Atributos: []xhtml.Attribute{{Key: "data-columna"}}})
		celdas[0] = append(celdas[0], []Bloque{{Tipo: Parrafo, Etiqueta: "p"}})
	}
	t.Filas = []FilaTabla{fila}
	if len(d.Bloques) > 0 && d.Bloques[p.B].Caja != nil {
		return p // tampoco dentro de un panel: la web no las mete ahí y no se leerían
	}
	return Pos{d.ponerConParrafo(p, aplanar(t, celdas)), 0}
}

// enColumnas dice si el bloque `i` es de unas columnas.
func (d *Documento) enColumnas(i int) bool {
	return i >= 0 && i < len(d.Bloques) && d.Bloques[i].Celda.Tabla.EsColumnas()
}

// ponerColumna mete en lo maquetado las líneas y los adornos de una columna maquetada
// aparte, llevados a (x0, y0) y con el índice de bloque del documento.
func (m *Maquetado) ponerColumna(cm *Maquetado, indice func(int) int, x0, y0, w float32) {
	for _, l := range cm.Lineas {
		l.Bloque = indice(l.Bloque)
		l.Y, l.Base, l.X0, l.X1 = l.Y+y0, l.Base+y0, l.X0+x0, l.X1+x0
		l.ZonaX0, l.ZonaX1 = x0, x0+w
		l.Trozos = slices.Clone(l.Trozos)
		for i := range l.Trozos {
			l.Trozos[i].X += x0
		}
		m.Lineas = append(m.Lineas, l)
	}
	for _, a := range cm.Adornos {
		a.Bloque, a.X, a.Y = indice(a.Bloque), a.X+x0, a.Y+y0
		m.Adornos = append(m.Adornos, a)
	}
}

// columnas maqueta los bloques de UNAS columnas desde `y`: cada una a su ancho y todas a
// la misma altura de arriba. Devuelve el alto y la caja de cada bloque. En el editor, la
// guía punteada de cada columna va a 4 px por fuera de ella.
func (m *Maquetado) columnas(bloques []Bloque, indice func(int) int, y, ancho float32, v tema.Variante) (float32, []CajaBloque) {
	t := bloques[0].Celda.Tabla
	n := 0
	if len(t.Filas) > 0 {
		n = len(t.Filas[0].Celdas)
	}
	n = max(n, 1)
	apilar := ancho < anchoApilar && ui.EsTactil()
	w := (ancho - huecoColumnas*float32(n-1)) / float32(n)
	if apilar {
		w = ancho
	}
	w = max(w, 1)
	cajas := make([]CajaBloque, len(bloques))
	var alto, yCol float32
	for c := range n {
		var desde, hasta int
		desde = slices.IndexFunc(bloques, func(b Bloque) bool { return b.Celda.Col == c })
		if desde < 0 {
			continue
		}
		hasta = desde
		for hasta < len(bloques) && bloques[hasta].Celda.Col == c {
			hasta++
		}
		doc := &Documento{Imagenes: m.imagenes, desplegado: m.desplegado, rotuloExpandir: m.rotulo}
		for _, b := range bloques[desde:hasta] {
			b.Celda = Celda{}
			doc.Bloques = append(doc.Bloques, b)
		}
		cm := Maquetar(doc, w, v, m.letra)
		x0, y0 := (w+huecoColumnas)*float32(c), y
		if apilar {
			x0, y0 = 0, y+yCol
		}
		m.ponerColumna(cm, func(k int) int { return indice(desde + k) }, x0, y0, w)
		for i, cb := range cm.Bloques {
			cb.Y, cb.X = cb.Y+y0, cb.X+x0
			cajas[desde+i] = cb
		}
		if m.desplegado {
			m.Adornos = append(m.Adornos, Adorno{
				Tipo: adornoGuia, Bloque: indice(desde),
				X: x0 - desfaseGuia, Y: y0 - desfaseGuia, An: w + 2*desfaseGuia, Al: cm.Alto + 2*desfaseGuia,
			})
		}
		if apilar {
			yCol += cm.Alto + huecoColumnas
			alto = yCol - huecoColumnas
		} else {
			alto = max(alto, cm.Alto)
		}
	}
	if m.desplegado {
		// Las guías de una fila miden lo que la más alta: se ven como una rejilla.
		for k := range m.Adornos {
			if a := &m.Adornos[k]; a.Tipo == adornoGuia && !apilar && a.Y == y-desfaseGuia {
				a.Al = alto + 2*desfaseGuia
			}
		}
	}
	return alto, cajas
}
