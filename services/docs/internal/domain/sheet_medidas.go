// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"math"
	"slices"
	"strconv"
)

// Los ANCHOS DE COLUMNA y los ALTOS DE FILA de una hoja de Calcus (`SheetData.colWidths` y
// `rowHeights`: px de maqueta a zoom 100 %, con el índice 0-based de la columna o la fila
// como clave). La rejilla de serie es de 96 × 24 px; solo se guarda lo que se aparta de
// ella. Excel mide el ancho en caracteres del dígito más ancho de la fuente base (7 px en
// Calibri 11, más 5 px de margen) y el alto en puntos; ODF, en unidades de longitud.

const (
	// anchoSeriePx y altoSeriePx son la rejilla de serie del editor (`CELL_W`, `CELL_H`).
	anchoSeriePx = 96.0
	altoSeriePx  = 24.0
	// Excel: ancho de un carácter y margen de la celda, en px.
	pxPorCaracter = 7.0
	pxMargen      = 5.0
	// Topes: el de Excel (255 caracteres y 409 pt) y un mínimo que se ve.
	maxAnchoPx = 1790.0
	maxAltoPx  = 545.0
	minMedida  = 4.0
	// maxColumnasConAncho acota las columnas a las que se expande un `<col min max>` enorme.
	maxColumnasConAncho = 1024
)

// medidasHoja son las medidas personalizadas; van embebidas en sheetData.
type medidasHoja struct {
	ColWidths  map[string]float64 `json:"colWidths,omitempty"`  //nolint:tagliatelle // el nombre lo fija el editor
	RowHeights map[string]float64 `json:"rowHeights,omitempty"` //nolint:tagliatelle // el nombre lo fija el editor
}

func redondeaPx(px float64) float64 { return math.Round(px*100) / 100 }

// medidaValida acota una medida de una entrada no confiable.
func medidaValida(px, tope float64) (float64, bool) {
	if math.IsNaN(px) || math.IsInf(px, 0) || px < minMedida {
		return 0, false
	}
	return redondeaPx(math.Min(px, tope)), true
}

// anchoExcel es el ancho de columna de Excel (caracteres) de un ancho en px.
func anchoExcel(px float64) float64 { return math.Round((px-pxMargen)/pxPorCaracter*100) / 100 }

// pxDeAnchoExcel es el ancho en px de un ancho de columna de Excel.
func pxDeAnchoExcel(w float64) float64 { return redondeaPx(w*pxPorCaracter + pxMargen) }

// medidasOrdenadas son las claves 0-based de un mapa de medidas, ordenadas y válidas.
func medidasOrdenadas(m map[string]float64, limite int) []int {
	var idx []int
	for k, v := range m {
		if i, err := strconv.Atoi(k); err == nil && i >= 0 && i < limite {
			if _, ok := medidaValida(v, maxAnchoPx); ok {
				idx = append(idx, i)
			}
		}
	}
	slices.Sort(idx)
	return idx
}

// altoFilaXlsx es el ` ht=".." customHeight="1"` de una fila (1-based); "" si no tiene.
func altoFilaXlsx(sd sheetData, fila int) string {
	px, ok := medidaValida(sd.RowHeights[strconv.Itoa(fila-1)], maxAltoPx)
	if !ok {
		return ""
	}
	return ` ht="` + strconv.FormatFloat(math.Round(px*0.75*100)/100, 'f', -1, 64) + `" customHeight="1"`
}

// filasConAlto son las filas (1-based) con alto propio.
func filasConAlto(sd sheetData) []int {
	var out []int
	for _, i := range medidasOrdenadas(sd.RowHeights, maxFilasHoja) {
		if _, ok := medidaValida(sd.RowHeights[strconv.Itoa(i)], maxAltoPx); ok {
			out = append(out, i+1)
		}
	}
	return out
}

// fijarAncho y fijarAlto guardan una medida si es válida y se aparta de la de serie.
func (m *medidasHoja) fijarAncho(col int, px float64) {
	if v, ok := medidaValida(px, maxAnchoPx); ok && math.Abs(v-anchoSeriePx) >= 1 {
		if m.ColWidths == nil {
			m.ColWidths = map[string]float64{}
		}
		m.ColWidths[strconv.Itoa(col)] = v
	}
}

func (m *medidasHoja) fijarAlto(fila int, px float64) {
	if v, ok := medidaValida(px, maxAltoPx); ok && math.Abs(v-altoSeriePx) >= 1 {
		if m.RowHeights == nil {
			m.RowHeights = map[string]float64{}
		}
		m.RowHeights[strconv.Itoa(fila)] = v
	}
}
