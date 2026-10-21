// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// LAS TABLAS DEL CUERPO de una nota, pintadas como las pinta la web. `.sytue-prose` no
// tiene reglas de tabla, así que lo que se ve allí es la tabla del navegador con el
// `preflight` (`border-collapse: collapse`):
//
//   - `table-layout: auto`, con el ancho de lo que lleva y no el del cuerpo;
//   - `<th>` en negrita y centrada, y cada celda centrada en vertical en su caja;
//   - sin margen propio: el `mb-2` del párrafo de antes es lo único que la separa;
//   - `colspan` y `rowspan` ocupan sus ranuras, y el fondo, el color y la alineación de
//     la celda, la fila, la sección o la tabla se heredan en ese orden (el saneador
//     deja pasar los tres).
//
// Y dos estados, porque la web también los tiene. La tabla GUARDADA no lleva bordes ni
// relleno: el saneador del servicio los quita del `style` (medido: celdas de 19,9, justo
// la línea). La RECIÉN INSERTADA, mientras se escribe, sí: la barra la mete con
// `width: 100%` y cada celda con `border: 1px solid #d4d0c8; padding: 6px 8px`, y
// Chromium la pinta así hasta que se guarda y se vuelve a abrir. Aquí igual: se pinta lo
// que dice su `style`, sea cual sea.

// estiloCelda es lo que una celda pinta además de su texto.
type estiloCelda struct {
	fondo, tinta string // hexadecimales heredados; vacío, ninguno
	alinear      string
	cabecera     bool
	borde        float32
	colorBorde   color.NRGBA
	relleno      [4]float32 // arriba, derecha, abajo, izquierda
}

// celdaPuesta es una celda ya colocada en la rejilla.
type celdaPuesta struct {
	fila, col, filas, cols int
	desde, hasta           int // sus bloques en la lista que se maqueta
	estilo                 estiloCelda
	doc                    *Documento
}

// heredado busca una propiedad en el `style` de la celda y, si no la trae, en los de su
// fila, su sección y la tabla, que es como se heredan en CSS.
func heredado(t *Tabla, f, c int, prop string) string {
	fila := t.Filas[f]
	for _, as := range [][]html.Attribute{fila.Celdas[c].Atributos, fila.Atributos, fila.AtributosSeccion, t.Atributos} {
		if v := propiedad(attr(as, "style"), prop); v != "" {
			return v
		}
	}
	return ""
}

func attr(as []html.Attribute, k string) string {
	for _, a := range as {
		if a.Key == k {
			return a.Val
		}
	}
	return ""
}

// px lee una longitud en píxeles («6px»); cero si no lo es.
func px(s string) float32 {
	v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "px"), 32)
	if err != nil || v < 0 {
		return 0
	}
	return float32(v)
}

// estiloCeldaDe resuelve lo que pinta una celda.
func estiloCeldaDe(t *Tabla, f, c int) estiloCelda {
	celda := t.Filas[f].Celdas[c]
	e := estiloCelda{
		fondo: heredado(t, f, c, "background-color"), tinta: heredado(t, f, c, "color"),
		alinear: heredado(t, f, c, "text-align"), cabecera: celda.Cabecera,
	}
	if e.alinear == "" && e.cabecera {
		e.alinear = "center"
	}
	propio := attr(celda.Atributos, "style")
	// `border: 1px solid #d4d0c8`: el grueso y el color, que es lo que escribe la barra.
	for tok := range strings.FieldsSeq(propiedad(propio, "border")) {
		if strings.HasSuffix(tok, "px") {
			e.borde = px(tok)
		} else if col, ok := tokens.ParseColor(tok); ok {
			e.colorBorde = col
		}
	}
	// `padding` con uno, dos, tres o cuatro valores, como en CSS.
	if v := strings.Fields(propiedad(propio, "padding")); len(v) > 0 {
		r := [4]float32{}
		for i := range r {
			switch len(v) {
			case 1:
				r[i] = px(v[0])
			case 2:
				r[i] = px(v[i%2])
			case 3:
				r[i] = px(v[[]int{0, 1, 2, 1}[i]])
			default:
				r[i] = px(v[i])
			}
		}
		e.relleno = r
	}
	return e
}

// tramo es un atributo numérico de una celda (`colspan`, `rowspan`): 1 si no lo trae.
func tramo(c CeldaTabla, k string) int {
	n, err := strconv.Atoi(strings.TrimSpace(attr(c.Atributos, k)))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// colocarCeldas reparte las celdas en la rejilla —cada una en la primera ranura libre de
// su fila, ocupando las que pidan su `colspan` y su `rowspan`— y les asigna sus bloques.
// Devuelve las celdas y el número de columnas.
func colocarCeldas(bloques []Bloque) ([]celdaPuesta, int) {
	t := bloques[0].Celda.Tabla
	rango := map[[2]int][2]int{}
	for k, b := range bloques {
		clave := [2]int{b.Celda.Fila, b.Celda.Col}
		if r, ok := rango[clave]; ok {
			rango[clave] = [2]int{r[0], k + 1}
		} else {
			rango[clave] = [2]int{k, k + 1}
		}
	}
	ocupada := map[[2]int]bool{}
	var puestas []celdaPuesta
	columnas := 0
	for f, fila := range t.Filas {
		col := 0
		for c, celda := range fila.Celdas {
			for ocupada[[2]int{f, col}] {
				col++
			}
			p := celdaPuesta{
				fila: f, col: col, cols: tramo(celda, "colspan"),
				filas:  min(tramo(celda, "rowspan"), len(t.Filas)-f),
				estilo: estiloCeldaDe(t, f, c),
			}
			for df := range p.filas {
				for dc := range p.cols {
					ocupada[[2]int{f + df, col + dc}] = true
				}
			}
			r, ok := rango[[2]int{f, c}]
			if !ok {
				r = [2]int{-1, -1}
			}
			p.desde, p.hasta = r[0], r[1]
			p.doc = docDeCelda(bloques, p)
			puestas = append(puestas, p)
			col += p.cols
			columnas = max(columnas, col)
		}
	}
	return puestas, columnas
}

// docDeCelda son los bloques de una celda como un documento aparte, con lo que heredan
// ya puesto: la alineación, el color y la negrita de la cabecera. Son COPIAS —el
// documento no cambia— y sin su `Celda`, para que el maquetador no vuelva a ver una
// tabla dentro.
func docDeCelda(bloques []Bloque, p celdaPuesta) *Documento {
	d := &Documento{}
	if p.desde < 0 {
		return d
	}
	for _, b := range bloques[p.desde:p.hasta] {
		b.Celda = Celda{}
		if b.Alineacion == "" {
			b.Alineacion = p.estilo.alinear
		}
		b.Tramos = slices.Clone(b.Tramos)
		for k := range b.Tramos {
			tr := &b.Tramos[k]
			if p.estilo.cabecera {
				tr.Marcas = slices.Concat([]string{"b"}, tr.Marcas)
			}
			if tr.Color == "" && tr.Enlace == "" {
				tr.Color = p.estilo.tinta
			}
		}
		d.Bloques = append(d.Bloques, b)
	}
	return d
}

// anchoMaquetado es lo que ocupa de ancho lo maquetado: su línea más larga con la
// sangría de su bloque, sin lo que la corre una alineación (una cabecera centrada a un
// ancho sin límite empezaría en medio millón).
func anchoMaquetado(m *Maquetado) float32 {
	var w float32
	for _, l := range m.Lineas {
		w = max(w, l.X1-l.X0+m.Bloques[l.Bloque].X)
	}
	for _, a := range m.Adornos {
		if a.Tipo != adornoFondo && a.Tipo != adornoRegla {
			w = max(w, a.X+a.An)
		}
	}
	return w
}

// anchoMaximo es «sin límite» para medir el ancho de contenido de una celda.
const anchoMaximo = 1e6

// horizontal es lo que el borde y el relleno de una celda le quitan de ancho.
func (e estiloCelda) horizontal() float32 { return e.relleno[1] + e.relleno[3] }

// vertical es lo que le quitan de alto.
func (e estiloCelda) vertical() float32 { return e.relleno[0] + e.relleno[2] }

// anchosDeColumna es el `table-layout: auto` a ese ancho: cada columna, lo que pide su
// celda más ancha sin partir (y una que ocupa varias reparte lo que les falte a partes
// iguales); si no caben todas, se reparte lo que sobra del ancho mínimo (el de la
// palabra más larga) en proporción a lo que a cada una le falta. Con `llenar` (el
// `width: 100%`), lo que sobra del ancho se reparte en proporción a lo que pide cada una.
func anchosDeColumna(celdas []celdaPuesta, columnas int, ancho, lineas float32, llenar bool, v tema.Variante) []float32 {
	minimos, maximos := make([]float32, columnas), make([]float32, columnas)
	medir := func(p celdaPuesta) (float32, float32) {
		h := p.estilo.horizontal()
		return anchoMaquetado(Maquetar(p.doc, 1, v)) + h, anchoMaquetado(Maquetar(p.doc, anchoMaximo, v)) + h
	}
	// Primero las de una columna, y luego las que ocupan varias sobre lo ya repartido.
	for _, varias := range []bool{false, true} {
		for _, p := range celdas {
			if (p.cols > 1) != varias {
				continue
			}
			mn, mx := medir(p)
			var sumMn, sumMx float32
			for c := p.col; c < p.col+p.cols; c++ {
				sumMn, sumMx = sumMn+minimos[c], sumMx+maximos[c]
			}
			hueco := float32(p.cols-1) * lineas
			for c := p.col; c < p.col+p.cols; c++ {
				minimos[c] += max(mn-hueco-sumMn, 0) / float32(p.cols)
				maximos[c] += max(mx-hueco-sumMx, 0) / float32(p.cols)
			}
		}
	}
	var sumaMin, sumaMax float32
	for j := range maximos {
		maximos[j] = max(maximos[j], minimos[j])
		sumaMin, sumaMax = sumaMin+minimos[j], sumaMax+maximos[j]
	}
	disponible := ancho - float32(columnas+1)*lineas
	switch {
	case sumaMax <= disponible && !llenar:
		return maximos
	case sumaMax <= disponible:
		out := make([]float32, columnas)
		for j := range maximos {
			parte := 1 / float32(columnas)
			if sumaMax > 0 {
				parte = maximos[j] / sumaMax
			}
			out[j] = maximos[j] + (disponible-sumaMax)*parte
		}
		return out
	case sumaMin >= disponible || sumaMax == sumaMin:
		return minimos
	}
	out := make([]float32, columnas)
	for j := range maximos {
		out[j] = minimos[j] + (disponible-sumaMin)*(maximos[j]-minimos[j])/(sumaMax-sumaMin)
	}
	return out
}

// tabla maqueta los bloques de UNA tabla desde `y` y devuelve su alto y la caja de cada
// bloque. Las líneas y los adornos de cada celda entran en el maquetado de la nota,
// movidos a su sitio y con el índice de bloque que diga `indice` (el suyo, o el del
// bloque opaco que la contiene).
//
//nolint:gocognit,gocyclo // Las reglas de rejilla y celdas deben mantenerse juntas para conservar geometría.
func (m *Maquetado) tabla(bloques []Bloque, indice func(int) int, y, ancho float32, v tema.Variante) (float32, []CajaBloque) {
	t := bloques[0].Celda.Tabla
	celdas, columnas := colocarCeldas(bloques)
	for k := range celdas {
		celdas[k].doc.Imagenes = m.imagenes // las imágenes de una celda, como las de fuera
	}
	// Con bordes, la rejilla es de líneas de ese grueso (`border-collapse`): una entre
	// cada dos columnas y una a cada lado.
	var lineas float32
	for _, p := range celdas {
		lineas = max(lineas, p.estilo.borde)
	}
	llenar := strings.TrimSpace(propiedad(attr(t.Atributos, "style"), "width")) == "100%"
	anchos := anchosDeColumna(celdas, columnas, ancho, lineas, llenar, v)
	xs := make([]float32, columnas+1)
	xs[0] = lineas
	for c := range columnas {
		xs[c+1] = xs[c] + anchos[c] + lineas
	}
	caja := func(p celdaPuesta) float32 { return xs[p.col+p.cols] - lineas - xs[p.col] }

	// Los altos: las de una fila deciden la suya; una que ocupa varias alarga la última.
	maq := make([]*Maquetado, len(celdas))
	altos := make([]float32, len(t.Filas))
	for _, varias := range []bool{false, true} {
		for k, p := range celdas {
			if (p.filas > 1) != varias {
				continue
			}
			maq[k] = Maquetar(p.doc, max(caja(p)-p.estilo.horizontal(), 1), v)
			necesita := maq[k].Alto + p.estilo.vertical()
			var tiene float32
			for f := p.fila; f < p.fila+p.filas; f++ {
				tiene += altos[f]
			}
			tiene += float32(p.filas-1) * lineas
			if necesita > tiene {
				altos[p.fila+p.filas-1] += necesita - tiene
			}
		}
	}
	ys := make([]float32, len(t.Filas)+1)
	ys[0] = y + lineas
	for f := range altos {
		ys[f+1] = ys[f] + altos[f] + lineas
	}

	cajas := make([]CajaBloque, len(bloques))
	fondos := len(m.Adornos)
	for k, p := range celdas {
		x0, y0 := xs[p.col], ys[p.fila]
		w, h := caja(p), ys[p.fila+p.filas]-lineas-y0
		if c, ok := tokens.ParseColor(p.estilo.fondo); ok && p.estilo.fondo != "" {
			m.Adornos = slices.Insert(m.Adornos, fondos, Adorno{Tipo: adornoCaja, Bloque: indice(max(p.desde, 0)), X: x0, Y: y0, An: w, Al: h, color: c})
			fondos++
		}
		cm := maq[k]
		dx := x0 + p.estilo.relleno[3]
		dy := y0 + p.estilo.relleno[0] + (h-p.estilo.vertical()-cm.Alto)/2 // `vertical-align: middle`
		for _, l := range cm.Lineas {
			l.Bloque = indice(p.desde + l.Bloque)
			l.Y, l.Base, l.X0, l.X1 = l.Y+dy, l.Base+dy, l.X0+dx, l.X1+dx
			l.ZonaX0, l.ZonaX1 = x0, x0+w
			l.Trozos = slices.Clone(l.Trozos)
			for i := range l.Trozos {
				l.Trozos[i].X += dx
			}
			m.Lineas = append(m.Lineas, l)
		}
		for _, a := range cm.Adornos {
			a.Bloque, a.X, a.Y = indice(p.desde+a.Bloque), a.X+dx, a.Y+dy
			m.Adornos = append(m.Adornos, a)
		}
		for i, cb := range cm.Bloques {
			cb.Y, cb.X = cb.Y+dy, cb.X+dx
			cajas[p.desde+i] = cb
		}
	}
	alto := ys[len(t.Filas)] - y
	if lineas > 0 {
		// La rejilla, por encima de los fondos y por debajo del texto: los cuatro bordes
		// de CADA celda con borde, y no rayas de lado a lado, para que una celda que
		// ocupa varias no salga cruzada por la raya de las que junta. Las celdas vecinas
		// comparten raya (`border-collapse`).
		rejilla := make([]Adorno, 0, 4*len(celdas))
		for _, p := range celdas {
			if p.estilo.borde <= 0 {
				continue
			}
			x0, y0 := xs[p.col]-lineas, ys[p.fila]-lineas
			an, al := xs[p.col+p.cols]-x0, ys[p.fila+p.filas]-y0
			c := p.estilo.colorBorde
			rejilla = append(rejilla,
				Adorno{Tipo: adornoCaja, X: x0, Y: y0, An: an, Al: lineas, color: c},
				Adorno{Tipo: adornoCaja, X: x0, Y: y0 + al - lineas, An: an, Al: lineas, color: c},
				Adorno{Tipo: adornoCaja, X: x0, Y: y0, An: lineas, Al: al, color: c},
				Adorno{Tipo: adornoCaja, X: x0 + an - lineas, Y: y0, An: lineas, Al: al, color: c})
		}
		m.Adornos = slices.Insert(m.Adornos, fondos, rejilla...)
	}
	return alto, cajas
}

// tablaOpaca maqueta una tabla guardada entera en un bloque (la de dentro de una celda):
// se lee aparte y todo lo suyo cuenta como del bloque `i`.
func (m *Maquetado) tablaOpaca(i int, b Bloque, y, ancho float32, v tema.Variante) float32 {
	d := Leer(b.Crudo)
	if len(d.Bloques) == 0 || !d.Bloques[0].Celda.EnTabla() {
		return 0
	}
	j := 1
	for j < len(d.Bloques) && d.Bloques[j].Celda.Tabla == d.Bloques[0].Celda.Tabla {
		j++
	}
	alto, _ := m.tabla(d.Bloques[:j], func(int) int { return i }, y, ancho, v)
	return alto
}
