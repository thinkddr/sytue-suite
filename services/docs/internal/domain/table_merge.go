// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

const (
	// maxTableCols acota el ancho de una tabla importada: Word llega a 63 columnas.
	maxTableCols = 256
	// maxCoveredCells acota las posiciones cubiertas por combinaciones de una tabla,
	// para que un fichero hostil con rowspan enormes no reserve memoria sin medida.
	maxCoveredCells = 1 << 20
)

type tablePos struct{ r, c int }

// rejilla es el estado de normalize: qué celda está en cada posición y qué posiciones
// cubre ya una combinación (con su origen).
type rejilla struct {
	nr, cols, budget int
	cells            map[tablePos]xcell
	owner            map[tablePos]tablePos
}

func (g *rejilla) cubierta(r, c int) bool { _, ok := g.owner[tablePos{r, c}]; return ok }

// recortar limita la combinación de x en (r, c) a lo que cabe sin pisar lo ya cubierto.
func (g *rejilla) recortar(x xcell, r, c int) (cs, rs int) {
	cs, rs = max(x.colspan, 1), max(x.rowspan, 1)
	cs, rs = min(cs, maxTableCols-c), min(rs, g.nr-r)
	for k := 1; k < cs; k++ {
		if g.cubierta(r, c+k) {
			cs = k
			break
		}
	}
	for k := 1; k < rs; k++ {
		for j := 0; j < cs; j++ {
			if g.cubierta(r+k, c+j) {
				rs = k
			}
		}
		if rs == k {
			break
		}
	}
	if cs*rs-1 > g.budget {
		return 1, 1
	}
	return cs, rs
}

// colocar pone x en (r, c) y marca lo que cubre; devuelve las columnas que ocupa.
func (g *rejilla) colocar(x xcell, r, c int) int {
	cs, rs := g.recortar(x, r, c)
	g.budget -= cs*rs - 1
	x.colspan, x.rowspan = cs, rs
	g.cells[tablePos{r, c}] = x
	for i := 0; i < rs; i++ {
		for j := 0; j < cs; j++ {
			if i+j > 0 {
				g.owner[tablePos{r + i, c + j}] = tablePos{r, c}
			}
		}
	}
	return cs
}

// normalize deja la tabla como rejilla rectangular a partir de filas «de marcado»
// (solo las celdas que existen, como en HTML y en OOXML, o como en ODF sin sus
// covered-table-cell): coloca cada celda en la primera posición libre de su fila,
// recorta colspan/rowspan a lo que cabe sin solaparse y rellena con celdas `covered` lo
// que cubren, y con celdas vacías lo que falta para completar las filas.
func (t *xtable) normalize() {
	g := &rejilla{nr: len(t.rows), budget: maxCoveredCells, cells: map[tablePos]xcell{}, owner: map[tablePos]tablePos{}}
	for r, row := range t.rows {
		c := 0
		for _, x := range row {
			for g.cubierta(r, c) {
				c++
			}
			if c >= maxTableCols {
				break
			}
			c += g.colocar(x, r, c)
			g.cols = max(g.cols, c)
		}
	}
	rows := make([][]xcell, g.nr)
	for r := range rows {
		rows[r] = make([]xcell, g.cols)
		for c := range rows[r] {
			p := tablePos{r, c}
			if o, ok := g.owner[p]; ok {
				rows[r][c] = xcell{covered: true, orow: o.r, ocol: o.c}
			} else if x, ok := g.cells[p]; ok {
				rows[r][c] = x
			} else {
				rows[r][c] = xcell{paras: []xpara{{}}}
			}
		}
	}
	t.rows, t.cols = rows, g.cols
}

// span devuelve las columnas y filas que ocupa la celda (siempre ≥ 1).
func (c xcell) span() (cols, rows int) { return max(c.colspan, 1), max(c.rowspan, 1) }
