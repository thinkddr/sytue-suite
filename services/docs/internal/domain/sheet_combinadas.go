// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "strconv"

// Las CELDAS COMBINADAS de una hoja de Calcus (`SheetData.merges`, rangos A1 de más de una
// celda que no se solapan): la celda de arriba a la izquierda es el ancla y las demás quedan
// cubiertas. En OOXML son `<mergeCells>`; en ODF, `number-columns-spanned` y
// `number-rows-spanned` en el ancla y `table:covered-table-cell` en el resto.

const maxCombinadas = 5000

// combinadasHoja son las celdas combinadas de una SheetData; van embebidas en sheetData.
type combinadasHoja struct {
	Merges []string `json:"merges,omitempty"`
}

// sanearCombinadas deja los rangos válidos de más de una celda y sin solaparse (el primero
// gana), acotados en número.
func sanearCombinadas(ms []string) []rango {
	var out []rango
	for _, m := range ms {
		r, ok := leerRango(m)
		if !ok || !r.valido() || (r.c1 == r.c2 && r.r1 == r.r2) || len(out) >= maxCombinadas {
			continue
		}
		solapa := false
		for _, o := range out {
			if r.c1 <= o.c2 && r.c2 >= o.c1 && r.r1 <= o.r2 && r.r2 >= o.r1 {
				solapa = true
				break
			}
		}
		if !solapa {
			out = append(out, r)
		}
	}
	return out
}

// combinadasDeRangos son los rangos como los guarda el editor.
func combinadasDeRangos(rs []rango) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.a1()
	}
	return out
}

// mergeCellsXlsx es el `<mergeCells>` de la hoja; "" si no hay.
func mergeCellsXlsx(sd sheetData) string {
	rs := sanearCombinadas(sd.Merges)
	if len(rs) == 0 {
		return ""
	}
	b := `<mergeCells count="` + strconv.Itoa(len(rs)) + `">`
	for _, r := range rs {
		b += `<mergeCell ref="` + r.a1() + `"/>`
	}
	return b + `</mergeCells>`
}

// combinarODS marca en las celdas del recorrido las combinadas de la hoja: el ancla lleva su
// rango y las demás quedan cubiertas (con las que faltan, vacías, añadidas). Una combinada que
// pasaría del tope de celdas vacías por hoja no se escribe.
func combinarODS(cells []cellVal, rs []rango) []cellVal {
	if len(rs) == 0 {
		return cells
	}
	pos := make(map[[2]int]int, len(cells)) // (fila, columna) -> índice
	for i, c := range cells {
		pos[[2]int{c.row, c.col}] = i
	}
	presupuesto := maxCeldasVacias
	for _, r := range rs {
		area := (r.r2 - r.r1 + 1) * (r.c2 - r.c1 + 1)
		if area > presupuesto {
			continue
		}
		presupuesto -= area
		rr := r
		for f := r.r1; f <= r.r2; f++ {
			for c := r.c1; c <= r.c2; c++ {
				i, hay := pos[[2]int{f, c}]
				if !hay {
					cells = append(cells, cellVal{ref: A1Ref(f, c), row: f, col: c, soloFormato: true})
					i = len(cells) - 1
					pos[[2]int{f, c}] = i
				}
				if f == r.r1 && c == r.c1 {
					cells[i].span = &rr
				} else {
					cells[i].cubierta = true
				}
			}
		}
	}
	return cells
}
