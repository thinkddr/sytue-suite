// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdfdoc

import (
	"fmt"
	"strings"
)

// Bloque es el contenido de una celda: un párrafo o una imagen.
type Bloque struct {
	Parrafo *Parrafo
	Imagen  *Imagen
}

// Celda de una tabla. Una Cubierta es una posición ocupada por la combinación de otra celda
// (ColSpan/RowSpan): no se pinta ni lleva etiqueta, como en HTML. Encabezado la hace TH.
type Celda struct {
	Bloques          []Bloque
	Encabezado       bool
	ColSpan, RowSpan int
	Cubierta         bool
}

// Tabla es una rejilla rectangular de celdas (todas las filas con las mismas columnas, con
// las cubiertas ya marcadas). No se aplana: sale como Table › TR › TH/TD.
type Tabla struct{ Filas [][]Celda }

const padCelda = 4.0

// item es una línea de texto o una imagen ya maquetadas, listas para pintar.
type item struct {
	l      linea
	tam    float64
	alto   float64 // incluye el interlineado y, en el último de un párrafo, el espacio posterior
	alinea Alineacion
	img    *imagenLay
}

type celdaLay struct {
	e          *Elem
	col, cs    int
	rs         int
	x, ancho   float64
	items      []item
	encabezado bool
}

func (c *celdaLay) alto() float64 {
	h := 2 * padCelda
	for _, it := range c.items {
		h += it.alto
	}
	return h
}

// maquetarCelda reparte los bloques de una celda en items, creando su etiqueta P/Figure.
func (d *Doc) maquetarCelda(c *celdaLay, bloques []Bloque, ancho, alto float64) {
	for _, b := range bloques {
		switch {
		case b.Parrafo != nil:
			p := *b.Parrafo
			tam := p.Tam
			if tam == 0 {
				tam = 11
			}
			if !tieneTexto(p.Fragmentos) {
				continue
			}
			tipo := "P"
			if strings.HasPrefix(p.Rol, "H") {
				tipo = d.tipoCabecera(p.Rol)
			}
			pe := d.nuevo(tipo, c.e, true)
			ls := d.romper(d.piezas(p.Fragmentos, tam, pe), tam, ancho-p.Sangria)
			for i, l := range ls {
				it := item{l: l, tam: tam, alto: tam * interlineado, alinea: p.Alinear}
				if i == len(ls)-1 {
					it.alto += p.Despues
				}
				c.items = append(c.items, it)
			}
		case b.Imagen != nil:
			if l, err := d.maquetarImagen(c.e, *b.Imagen, ancho, alto); err == nil {
				c.items = append(c.items, item{img: l, alto: l.h + 4})
			}
		}
	}
}

// anchosColumnas reparte el ancho útil en proporción al texto de cada columna (el de una línea
// sin partir, con un mínimo): las columnas de cifras no se llevan el mismo ancho que una de
// comentarios. ponytail: solo cuentan las celdas sin combinar; con tablas muy desiguales un
// algoritmo de mínimos/máximos como el de CSS sería más fino.
func (d *Doc) anchosColumnas(t Tabla, n int) []float64 {
	avail := PageW - 2*Margin
	nat := make([]float64, n)
	for j := range nat {
		nat[j] = 36
	}
	for _, fila := range t.Filas {
		for j, c := range fila {
			if c.Cubierta || max(c.ColSpan, 1) > 1 {
				continue
			}
			for _, b := range c.Bloques {
				switch {
				case b.Parrafo != nil:
					tam := b.Parrafo.Tam
					if tam == 0 {
						tam = 11
					}
					w := 0.0
					for _, p := range d.piezas(b.Parrafo.Fragmentos, tam, nil) {
						w += p.ancho
					}
					nat[j] = max(nat[j], min(w+2*padCelda, avail))
				case b.Imagen != nil:
					nat[j] = max(nat[j], min(b.Imagen.Ancho+2*padCelda, avail/2))
				}
			}
		}
	}
	return repartir(nat, avail)
}

// repartir da a cada columna su ancho natural si cabe todo; si no, las columnas cortas (por
// debajo de la media) conservan el suyo y las largas se reparten el resto en proporción.
func repartir(nat []float64, avail float64) []float64 {
	suma := 0.0
	for _, w := range nat {
		suma += w
	}
	out := make([]float64, len(nat))
	if suma <= avail {
		for j, w := range nat {
			out[j] = avail * w / suma
		}
		return out
	}
	media := avail / float64(len(nat))
	resto, largas := avail, 0.0
	for _, w := range nat {
		if w <= media {
			resto -= w
		} else {
			largas += w
		}
	}
	for j, w := range nat {
		if w <= media {
			out[j] = w
		} else {
			out[j] = resto * w / largas
		}
	}
	return out
}

// Tabla maqueta y pinta una tabla etiquetada. Cada fila (o grupo de filas unidas por una
// combinación vertical) cabe en una página; si no cabe en lo que queda, pasa a la
// siguiente. Una fila más alta que una página entera se parte por líneas.
// ponytail: las filas de cabecera no se repiten en cada página; la combinación vertical que
// abarca más que una página se pinta sin combinar.
func (d *Doc) Tabla(t Tabla) {
	d.listas = nil
	if len(t.Filas) == 0 || len(t.Filas[0]) == 0 {
		return
	}
	tabla := d.Abrir("Table")
	defer d.Cerrar()
	filas := d.maquetarFilas(t, tabla)
	cuerpo := PageH - 2*Margin
	for i := 0; i < len(filas); {
		fin := i
		for k := i; k <= fin; k++ {
			for _, c := range filas[k] {
				fin = max(fin, k+c.rs-1)
			}
		}
		h := alturas(filas, i, fin, false)
		plano := false
		switch total := sumar(h); {
		case total <= d.y-Margin:
		case total <= cuerpo:
			d.NuevaPagina()
		default:
			plano = true
			h = alturas(filas, i, fin, true)
		}
		for k := i; k <= fin; k++ {
			cajas := make([]float64, len(filas[k]))
			for m, c := range filas[k] {
				cajas[m] = h[k-i]
				if !plano && c.rs > 1 {
					cajas[m] = sumar(h[k-i : k-i+c.rs])
				}
			}
			d.dibujarFila(filas[k], h[k-i], cajas)
		}
		i = fin + 1
	}
	d.y -= 6
}

// maquetarFilas crea la estructura (TR, TH/TD) y reparte el contenido de cada celda en items.
func (d *Doc) maquetarFilas(t Tabla, tabla *Elem) [][]*celdaLay {
	n := len(t.Filas[0])
	anchos := d.anchosColumnas(t, n)
	cuerpo := PageH - 2*Margin
	filas := make([][]*celdaLay, len(t.Filas))
	for i, fila := range t.Filas {
		tr := d.nuevo("TR", tabla, true)
		x := Margin
		for j, c := range fila {
			if j >= n {
				break
			}
			if !c.Cubierta {
				cs := max(1, min(c.ColSpan, n-j))
				rs := max(1, min(c.RowSpan, len(t.Filas)-i))
				w := sumar(anchos[j : j+cs])
				tipo := "TD"
				if c.Encabezado {
					tipo = "TH"
				}
				cl := &celdaLay{e: d.nuevo(tipo, tr, true), col: j, cs: cs, rs: rs, x: x, ancho: w, encabezado: c.Encabezado}
				cl.e.colSpan, cl.e.rowSpan = cs, rs
				if c.Encabezado {
					cl.e.scope = alcance(fila, j)
				}
				d.maquetarCelda(cl, c.Bloques, w-2*padCelda, cuerpo-2*padCelda)
				filas[i] = append(filas[i], cl)
			}
			x += anchos[j]
		}
	}
	return filas
}

// alcance dice a qué se aplica un encabezado: a la columna si toda su fila son encabezados
// (una fila de títulos), a la fila si no (la primera columna de una tabla de datos).
func alcance(fila []Celda, j int) string {
	todas := true
	for _, c := range fila {
		if !c.Cubierta && !c.Encabezado {
			todas = false
		}
	}
	switch {
	case todas:
		return "Column"
	case j == 0:
		return "Row"
	default:
		return "Column"
	}
}

func sumar(v []float64) float64 {
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s
}

// alturas calcula la altura de las filas a..b: la de su celda más alta, y las celdas con
// combinación vertical estiran su última fila si no les basta. plano ignora la combinación.
func alturas(filas [][]*celdaLay, a, b int, plano bool) []float64 {
	h := make([]float64, b-a+1)
	for k := a; k <= b; k++ {
		h[k-a] = 2*padCelda + 11*interlineado
		for _, c := range filas[k] {
			if c.rs == 1 || plano {
				h[k-a] = max(h[k-a], c.alto())
			}
		}
	}
	if !plano {
		for k := a; k <= b; k++ {
			for _, c := range filas[k] {
				if c.rs > 1 {
					falta := c.alto() - sumar(h[k-a:k-a+c.rs])
					if falta > 0 {
						h[k-a+c.rs-1] += falta
					}
				}
			}
		}
	}
	return h
}

// dibujarFila pinta una fila en d.y. alto es la de la fila y cajas la de la caja de cada celda
// (más alta que la fila si se combina con las de abajo, que ya están en la misma página porque
// la unidad cabía). Una fila más alta que lo que queda se parte por líneas, página a página.
func (d *Doc) dibujarFila(fila []*celdaLay, alto float64, cajas []float64) {
	hecho := make([]int, len(fila))
	entera := true
	for {
		disp := d.y - Margin
		if entera && alto <= disp {
			for k, c := range fila {
				d.dibujarCelda(c, 0, len(c.items), d.y, cajas[k])
			}
			d.y -= alto
			return
		}
		fin := make([]int, len(fila))
		trozo := 2 * padCelda
		paginaNueva := d.y > PageH-Margin-0.5
		for k, c := range fila {
			fin[k] = hecho[k]
			usado := 2 * padCelda
			for fin[k] < len(c.items) && (usado+c.items[fin[k]].alto <= disp || (paginaNueva && fin[k] == hecho[k])) {
				usado += c.items[fin[k]].alto
				fin[k]++
			}
			trozo = max(trozo, usado)
		}
		if trozo <= 2*padCelda { // no cupo ni una línea: página nueva
			d.NuevaPagina()
			continue
		}
		todo := true
		for k, c := range fila {
			d.dibujarCelda(c, hecho[k], fin[k], d.y, trozo)
			hecho[k] = fin[k]
			todo = todo && hecho[k] >= len(c.items)
		}
		d.y -= trozo
		if todo {
			return
		}
		entera = false
		d.NuevaPagina()
	}
}

func (d *Doc) dibujarCelda(c *celdaLay, desde, hasta int, ysup, alto float64) {
	d.artefacto(func(b *strings.Builder) {
		if c.encabezado {
			fmt.Fprintf(b, "q 0.92 g %s %s %s %s re f Q\n", ftoa(c.x), ftoa(ysup-alto), ftoa(c.ancho), ftoa(alto))
		}
		fmt.Fprintf(b, "q 0.55 G 0.5 w %s %s %s %s re S Q\n", ftoa(c.x), ftoa(ysup-alto), ftoa(c.ancho), ftoa(alto))
	})
	y := ysup - padCelda
	inner := c.ancho - 2*padCelda
	for _, it := range c.items[desde:hasta] {
		x := c.x + padCelda
		if it.img != nil {
			switch it.img.alinea {
			case Izquierda:
			case Centro:
				x += (inner - it.img.w) / 2
			case Derecha:
				x += inner - it.img.w
			}
			d.dibujarImagen(it.img, x, y)
		} else {
			switch it.alinea {
			case Izquierda:
			case Centro:
				x += (inner - it.l.ancho) / 2
			case Derecha:
				x += inner - it.l.ancho
			}
			d.pintar(it.l, x, y-it.tam*1.05, it.tam)
		}
		y -= it.alto
	}
}
