// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"strconv"
)

// COMBINAR Y SEPARAR CELDAS de una tabla: `colspan` y `rowspan`, los atributos de HTML,
// con los valores que deja el saneador del servicio (de 2 en adelante, hasta su tope; un
// 1 no se escribe). La web hace lo mismo con las mismas reglas (`tabla-notas.ts`):
//
//   - «Combinar con la de la derecha»: la celda se queda con la de su derecha si ocupan
//     las mismas filas; el texto de la otra pasa detrás del suyo.
//   - «Combinar con la de abajo»: igual con la que empieza justo debajo y ocupa las mismas
//     columnas.
//   - «Separar celdas»: la celda vuelve a ocupar una ranura, y las que tapaba vuelven,
//     vacías, a su sitio.
//
// Cada orden es una `Tabla` nueva, como las demás de la barra de tabla (las fotos de
// deshacer comparten la vieja), y su paso de deshacer.

// Los topes de `colspan` y `rowspan`: los del saneador (`apps/nucleo/notes/model/html.go`).
const (
	MaxColspan = 20
	MaxRowspan = 100
)

// OrdenesCombinar son las órdenes de combinar de la barra de tabla, con su rótulo.
var OrdenesCombinar = []struct{ Orden, Clave string }{
	{"combinarDerecha", "editor.mergeRight"},
	{"combinarAbajo", "editor.mergeDown"},
	{"separarCelda", "editor.splitCell"},
}

// ranuras es dónde empieza cada celda de la tabla en la rejilla: la columna de la
// rejilla de cada `[fila][celda]`, con las ranuras que tapan las combinadas de arriba ya
// saltadas (lo mismo que `colocarCeldas`, sin bloques).
func ranuras(t *Tabla) [][]int {
	ocupada := map[[2]int]bool{}
	out := make([][]int, len(t.Filas))
	for f, fila := range t.Filas {
		col := 0
		for _, celda := range fila.Celdas {
			for ocupada[[2]int{f, col}] {
				col++
			}
			out[f] = append(out[f], col)
			for df := range min(tramo(celda, "rowspan"), len(t.Filas)-f) {
				for dc := range tramo(celda, "colspan") {
					ocupada[[2]int{f + df, col + dc}] = true
				}
			}
			col += tramo(celda, "colspan")
		}
	}
	return out
}

// conTramo pone `colspan` o `rowspan` a `n` (quitándolo con 1).
func conTramo(c CeldaTabla, k string, n int) CeldaTabla {
	v := ""
	if n > 1 {
		v = strconv.Itoa(n)
	}
	c.Atributos = conAtributo(c.Atributos, k, v)
	return c
}

// vecinaDerecha es la celda que se combinaría con la (f, c) a la derecha: la siguiente
// de su fila, si ocupa las mismas filas y el total no pasa del tope. -1 si no hay.
func vecinaDerecha(t *Tabla, f, c int) int {
	fila := t.Filas[f].Celdas
	if c+1 >= len(fila) {
		return -1
	}
	a, b := fila[c], fila[c+1]
	if tramo(a, "rowspan") != tramo(b, "rowspan") || tramo(a, "colspan")+tramo(b, "colspan") > MaxColspan {
		return -1
	}
	if r := ranuras(t)[f]; r[c+1] != r[c]+tramo(a, "colspan") {
		return -1 // entre las dos asoma una de arriba: no son vecinas
	}
	return c + 1
}

// vecinaAbajo es la celda que se combinaría con la (f, c) por abajo: en la fila donde
// acaba, la que empieza en su misma columna y ocupa las mismas. (-1, -1) si no hay.
func vecinaAbajo(t *Tabla, f, c int) (int, int) {
	a := t.Filas[f].Celdas[c]
	g := f + tramo(a, "rowspan")
	if g >= len(t.Filas) {
		return -1, -1
	}
	r := ranuras(t)
	k := slices.Index(r[g], r[f][c])
	if k < 0 {
		return -1, -1
	}
	b := t.Filas[g].Celdas[k]
	if tramo(b, "colspan") != tramo(a, "colspan") || tramo(a, "rowspan")+tramo(b, "rowspan") > MaxRowspan ||
		t.Filas[g].Seccion != t.Filas[f].Seccion {
		return -1, -1
	}
	return g, k
}

// PuedeCombinar dice si la orden de combinar vale con el cursor en `p`.
func (d *Documento) PuedeCombinar(orden string, p Pos) bool {
	if len(d.Bloques) == 0 || p.B >= len(d.Bloques) {
		return false
	}
	en := d.Bloques[p.B].Celda
	if !en.EnTabla() || en.Tabla.EsColumnas() || en.Fila >= len(en.Tabla.Filas) || en.Col >= len(en.Tabla.Filas[en.Fila].Celdas) {
		return false
	}
	t, f, c := en.Tabla, en.Fila, en.Col
	switch orden {
	case "combinarDerecha":
		return vecinaDerecha(t, f, c) >= 0
	case "combinarAbajo":
		g, _ := vecinaAbajo(t, f, c)
		return g >= 0
	case "separarCelda":
		celda := t.Filas[f].Celdas[c]
		return tramo(celda, "colspan") > 1 || tramo(celda, "rowspan") > 1
	}
	return false
}

// CombinarCeldas aplica una de OrdenesCombinar a la celda del cursor y devuelve dónde
// queda: al principio de la celda combinada o de la separada.
func (d *Documento) CombinarCeldas(orden string, p Pos) Pos {
	if !d.PuedeCombinar(orden, p) {
		return p
	}
	en := d.Bloques[p.B].Celda
	i, j := d.rangoTabla(p.B)
	t, f, c := en.Tabla, en.Fila, en.Col
	filas := make([]FilaTabla, len(t.Filas))
	for k, fila := range t.Filas {
		fila.Celdas = slices.Clone(fila.Celdas)
		filas[k] = fila
	}
	celdas := d.desplegar(i, j)
	// juntar pone el texto de la celda absorbida detrás del de la que se queda, sin las
	// líneas vacías de la otra.
	juntar := func(g, k int) {
		for _, b := range celdas[g][k] {
			if !vacio(b) {
				celdas[f][c] = append(slices.DeleteFunc(celdas[f][c], vacio), b)
			}
		}
	}
	switch orden {
	case "combinarDerecha":
		k := vecinaDerecha(t, f, c)
		juntar(f, k)
		filas[f].Celdas[c] = conTramo(filas[f].Celdas[c], "colspan", tramo(t.Filas[f].Celdas[c], "colspan")+tramo(t.Filas[f].Celdas[k], "colspan"))
		filas[f].Celdas = slices.Delete(filas[f].Celdas, k, k+1)
		celdas[f] = slices.Delete(celdas[f], k, k+1)
	case "combinarAbajo":
		g, k := vecinaAbajo(t, f, c)
		juntar(g, k)
		filas[f].Celdas[c] = conTramo(filas[f].Celdas[c], "rowspan", tramo(t.Filas[f].Celdas[c], "rowspan")+tramo(t.Filas[g].Celdas[k], "rowspan"))
		filas[g].Celdas = slices.Delete(filas[g].Celdas, k, k+1)
		celdas[g] = slices.Delete(celdas[g], k, k+1)
	case "separarCelda":
		separar(t, filas, celdas, f, c)
	}
	nueva := &Tabla{Atributos: t.Atributos, Filas: filas}
	nuevos := aplanar(nueva, celdas)
	d.Bloques = slices.Concat(d.Bloques[:i], nuevos, d.Bloques[j:])
	for k := i; k < i+len(nuevos); k++ {
		if b := d.Bloques[k]; b.Celda.Fila == f && b.Celda.Col == c {
			return Pos{k, inicioEditable(b)}
		}
	}
	return Pos{i, 0}
}

// separar devuelve la celda (f, c) a una ranura y pone celdas vacías en las que tapaba:
// detrás de ella en su fila y, en las de debajo que ocupaba, en el sitio de su columna.
// Las nuevas son de cabecera si ella lo era, con su `scope`, para que la fila no cambie.
func separar(t *Tabla, filas []FilaTabla, celdas [][][]Bloque, f, c int) {
	r := ranuras(t)
	vieja := t.Filas[f].Celdas[c]
	cs, rs := tramo(vieja, "colspan"), min(tramo(vieja, "rowspan"), len(t.Filas)-f)
	filas[f].Celdas[c] = conTramo(conTramo(vieja, "colspan", 1), "rowspan", 1)
	hueca := func() CeldaTabla {
		n := CeldaTabla{Cabecera: vieja.Cabecera}
		if v := attr(vieja.Atributos, "scope"); v != "" {
			n.Atributos = conAtributo(nil, "scope", v)
		}
		return n
	}
	for g := f; g < f+rs; g++ {
		donde := c + 1 // en su fila, detrás de ella
		if g > f {
			// En las de debajo, delante de la primera celda que empiece más a la derecha.
			donde = len(r[g])
			for k, col := range r[g] {
				if col > r[f][c] {
					donde = k
					break
				}
			}
		}
		n := cs
		if g == f {
			n = cs - 1
		}
		for range n {
			filas[g].Celdas = slices.Insert(filas[g].Celdas, donde, hueca())
			celdas[g] = slices.Insert(celdas[g], donde, []Bloque{{Tipo: Parrafo}})
		}
	}
}
