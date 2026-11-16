// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Las MEDIDAS PROPIAS de filas y columnas: los anchos y altos de un .xlsx o un .ods, y los
// que se cambian a mano. Es el puerto de la segunda mitad de `sheets/rejilla.ts`, con sus
// mismos nombres: `colWidths` y `rowHeights` guardan en px de maqueta solo lo que se aparta
// de la medida de serie, y la geometría deja de ser «puesto × paso» para ser una lista de
// posiciones acumuladas que se busca por bisección. Sin medidas propias nada de esto se
// ejecuta y la rejilla es la de siempre.
//
// Los vectores de `packages/office/test/vectores/calcus.json` («medidas») exigen que estas
// funciones den lo mismo que las de la web con las mismas entradas.

// Los límites de una medida propia, en px de maqueta: lo que se ve y lo que cabe en Excel
// (255 caracteres de ancho y 409 pt de alto). Son `MEDIDA_MIN`, `ANCHO_MAX` y `ALTO_MAX`.
const (
	MinMedida float32 = 4
	AnchoMax  float32 = 1790
	AltoMax   float32 = 545
)

// AireAjuste es `AIRE_AJUSTE`: el aire a cada lado del texto al autoajustar una columna.
const AireAjuste float32 = 10

// medidaDe es la medida de la unidad i (columna o fila, 0-based): la propia si es válida y,
// si no, la de serie. La propia se acota a `tope`.
func medidaDe(medidas map[string]float64, i int, serie, tope float32) float32 {
	m, ok := medidas[strconv.Itoa(i)]
	if !ok || math.IsNaN(m) || math.IsInf(m, 0) || float32(m) < MinMedida {
		return serie
	}
	return min(float32(m), tope)
}

// acumulado son las posiciones de las unidades `visibles`: pos[k] es el borde inicial de la
// k-ésima (el origen más lo que miden las anteriores) y pos[n], el final de la última.
func acumulado(origen float32, visibles []int, medida func(int) float32) []float32 {
	pos := make([]float32, len(visibles)+1)
	x := origen
	for k, i := range visibles {
		pos[k] = x
		x += medida(i)
	}
	pos[len(visibles)] = x
	return pos
}

// indiceEn es el índice visible (0..n-1) de la unidad que cubre la coordenada x; se acota a
// los extremos.
func indiceEn(pos []float32, x float32) int {
	n := len(pos) - 1
	if n <= 0 {
		return 0
	}
	lo, hi := 0, n-1
	for lo < hi {
		mid := (lo + hi + 1) >> 1
		if pos[mid] <= x {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// reMedida es lo que se teclea en «Ancho de columna…»: «120», «120px» o «120,5».
var reMedida = regexp.MustCompile(`^(\d+(?:[.,]\d+)?)\s*(?:px)?$`)

// medidaPedida es lo que pide `colwidth`/`rowheight`: unos px o el autoajuste.
type medidaPedida struct {
	auto bool
	px   float64
}

// leerMedida es `leerMedida`: un número de px de maqueta, «auto» o nada válido (false).
func leerMedida(valor string) (medidaPedida, bool) {
	v := strings.ToLower(strings.TrimSpace(valor))
	if v == "auto" {
		return medidaPedida{auto: true}, true
	}
	m := reMedida.FindStringSubmatch(v)
	if m == nil {
		return medidaPedida{}, false
	}
	n, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
	if err != nil || math.IsInf(n, 0) {
		return medidaPedida{}, false
	}
	return medidaPedida{px: n}, true
}

// conMedidas es `conMedidas`: el mapa con `indices` puestos a px (nil = volver a la de
// serie). La medida se redondea y se acota a MinMedida..tope; la que coincide con la de
// serie BORRA su clave, para que el modelo siga guardando solo lo que se aparta (y la
// coedición, que fusiona por clave, no arrastre ruido). nil si no queda ninguna.
//
// No toca el mapa de entrada: la foto del historial lo comparte.
func conMedidas(medidas map[string]float64, indices []int, px *float64, serie, tope float32) map[string]float64 {
	out := make(map[string]float64, len(medidas)+len(indices))
	for k, v := range medidas {
		out[k] = v
	}
	m := float64(serie)
	if px != nil {
		m = math.Min(float64(tope), math.Max(float64(MinMedida), redondeoJS(*px)))
	}
	for _, i := range indices {
		if m == float64(serie) {
			delete(out, strconv.Itoa(i))
		} else {
			out[strconv.Itoa(i)] = m
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// redondeoJS es `Math.round`: la mitad va hacia +∞, no hacia fuera del cero.
func redondeoJS(x float64) float64 { return math.Floor(x + 0.5) }

// anchoAjustado es el ancho de autoajuste de una columna a partir de lo que miden sus
// textos: el más ancho más el aire, acotado. Una columna vacía vuelve a la de serie, como
// el doble clic de Excel sobre una columna sin datos.
func anchoAjustado(anchos []float32, serie float32) float32 {
	var mayor float32
	for _, a := range anchos {
		if !math.IsNaN(float64(a)) && !math.IsInf(float64(a), 0) && a > mayor {
			mayor = a
		}
	}
	if mayor <= 0 {
		return serie
	}
	return min(AnchoMax, max(MinMedida, float32(math.Ceil(float64(mayor+AireAjuste)))))
}

// altoAjustado es el alto de autoajuste de una fila a partir de los tamaños de letra (pt)
// de sus celdas: la línea de la letra mayor (×4/3 a px, interlineado 1,3) más el aire;
// nunca por debajo de la de serie. Sin tamaños propios vuelve a la de serie.
func altoAjustado(tamanosPt []float64, serie float32) float32 {
	var mayor float64
	for _, t := range tamanosPt {
		if !math.IsNaN(t) && !math.IsInf(t, 0) && t > mayor {
			mayor = t
		}
	}
	if mayor <= 0 {
		return serie
	}
	return min(AltoMax, max(serie, float32(math.Ceil(mayor*4/3*1.3+6))))
}

// eje es la geometría de un eje de la rejilla en px de la HOJA (sin zoom), contada desde
// el origen de las celdas: los puestos que se ven (el mapa de lo oculto) y, con medidas
// propias, sus posiciones acumuladas.
//
// `pos` solo llega hasta la última unidad con medida propia: de ahí en adelante todas son
// de serie y la posición se extrapola. Así una hoja de un millón de filas con el alto de
// la cabecera cambiado no acumula un millón de posiciones en cada pintado.
type eje struct {
	mapaFilas
	serie float32
	pos   []float32 // nil: paso fijo
}

// nuevoEje monta el eje con lo que esconde el mapa y las medidas propias de la hoja.
func nuevoEje(m mapaFilas, medidas map[string]float64, serie, tope float32) eje {
	e := eje{mapaFilas: m, serie: serie}
	ultima := -1
	for k := range medidas {
		// Con la de serie a -1, medidaDe dice si la propia es válida.
		if i, err := strconv.Atoi(k); err == nil && i > ultima && medidaDe(medidas, i, -1, tope) >= 0 {
			ultima = i
		}
	}
	if ultima < 0 {
		return e
	}
	n := m.puesto(ultima + 1)
	vis := make([]int, n)
	for k := range vis {
		vis[k] = m.fila(k)
	}
	e.pos = acumulado(0, vis, func(i int) float32 { return medidaDe(medidas, i, serie, tope) })
	return e
}

// inicio es dónde empieza el puesto k.
func (e eje) inicio(k int) float32 {
	if e.pos == nil || k < 0 {
		return float32(k) * e.serie
	}
	if n := len(e.pos) - 1; k > n {
		return e.pos[n] + float32(k-n)*e.serie
	}
	return e.pos[k]
}

// borde es dónde empieza la unidad i, oculta o no (una oculta mide cero y empieza donde
// la siguiente que se ve).
func (e eje) borde(i int) float32 { return e.inicio(e.puesto(i)) }

// medida es lo que mide la unidad i en pantalla: cero si está oculta.
func (e eje) medida(i int) float32 {
	if e.oculta(i) {
		return 0
	}
	k := e.puesto(i)
	return e.inicio(k+1) - e.inicio(k)
}

// puestoEn es el puesto que cae en la coordenada x (desde el origen). Con paso fijo trunca
// hacia el cero, como siempre ha hecho la rejilla: un poco antes del origen es el puesto 0.
func (e eje) puestoEn(x float32) int {
	if e.pos == nil || x < 0 {
		return int(x / e.serie)
	}
	if n := len(e.pos) - 1; x >= e.pos[n] {
		return n + int((x-e.pos[n])/e.serie)
	}
	return indiceEn(e.pos, x)
}

// puestosHasta es cuántos puestos hacen falta para cubrir hasta x: el techo de puestoEn.
func (e eje) puestosHasta(x float32) int {
	k := e.puestoEn(x)
	if e.inicio(k) < x {
		k++
	}
	return k
}
