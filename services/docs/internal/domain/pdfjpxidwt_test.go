// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de la wavelet inversa.
//
// El check que vale es la IDA Y VUELTA con la 5/3: esa transformada es
// REVERSIBLE por definición —es la del modo sin pérdida— así que descomponer y
// recomponer tiene que devolver los mismos enteros, bit a bit. Cualquier fallo
// de redondeo, de borde o de paridad rompe esa igualdad, y ninguno de ellos da
// un error: dan una imagen casi bien.
package domain

import "testing"

// dwt53Linea es la transformada DIRECTA (§F.4.8.1), escrita aquí solo para poder
// comprobar la vuelta. No hace falta en producción: un PDF trae la imagen ya
// descompuesta.
func dwt53Linea(x []int32, i0 int) {
	n := len(x)
	if n <= 1 {
		if n == 1 && i0%2 != 0 {
			x[0] *= 2
		}
		return
	}
	en := func(i int) int32 { return x[reflejar(i, n)] }
	y := make([]int32, n)
	copy(y, x)
	// Primero las impares (alta frecuencia), a partir de las pares originales.
	for i := 0; i < n; i++ {
		if (i+i0)%2 == 0 {
			continue
		}
		y[i] = x[i] - pisoDiv(en(i-1)+en(i+1), 2)
	}
	// Después las pares, a partir de las impares YA transformadas.
	enY := func(i int) int32 { return y[reflejar(i, n)] }
	for i := 0; i < n; i++ {
		if (i+i0)%2 != 0 {
			continue
		}
		y[i] = x[i] + pisoDiv(enY(i-1)+enY(i+1)+2, 4)
	}
	copy(x, y)
}

func TestLa53EsReversibleBitABit(t *testing.T) {
	// Con NEGATIVOS y con longitud impar: los dos casos donde se rompe. Los
	// negativos por el redondeo (Go trunca hacia cero, la norma va al piso) y la
	// longitud impar porque deja un extremo sin pareja.
	casos := [][]int32{
		{5, -3, 200, -128, 0, 77, -1},
		{-1, -1, -1, -1},
		{0},
		{1000, -1000},
		{7, 7, 7, 7, 7, 7, 7, 7, 7},
	}
	for _, inicial := range casos {
		for _, i0 := range []int{0, 1} {
			x := append([]int32(nil), inicial...)
			dwt53Linea(x, i0)
			idwt53Linea(x, i0)
			for i := range x {
				if x[i] != inicial[i] {
					t.Errorf("i0=%d, len=%d: la muestra %d volvió como %d y era %d (%v → %v)",
						i0, len(inicial), i, x[i], inicial[i], inicial, x)
					break
				}
			}
		}
	}
}

// TestElPisoNoEsElTruncado fija el redondeo. Es UNA línea de código y es la
// diferencia entre que el modo sin pérdida lo sea o no.
func TestElPisoNoEsElTruncado(t *testing.T) {
	casos := [][3]int32{
		{-5, 4, -2}, // Go daría -1
		{-1, 2, -1}, // Go daría 0
		{-4, 4, -1},
		{5, 4, 1},
		{0, 4, 0},
		{-8, 4, -2},
	}
	for _, c := range casos {
		if got := pisoDiv(c[0], c[1]); got != c[2] {
			t.Errorf("pisoDiv(%d, %d) = %d, quería %d", c[0], c[1], got, c[2])
		}
	}
}

// TestElBordeSeRefleja: extender con ceros deja una raya oscura en cada borde
// —y una cuadrícula si la imagen va en tiles—, que es un defecto muy visible y
// que no da ningún error.
func TestElBordeSeRefleja(t *testing.T) {
	// Con n=5 el periodo es 8: los índices salen espejados, no recortados.
	quiero := map[int]int{-1: 1, -2: 2, -3: 3, 5: 3, 6: 2, 7: 1, 8: 0, 0: 0, 4: 4}
	for i, esperado := range quiero {
		if got := reflejar(i, 5); got != esperado {
			t.Errorf("reflejar(%d, 5) = %d, quería %d", i, got, esperado)
		}
	}
	// Con una sola muestra no hay a dónde reflejar y todo cae en ella.
	for _, i := range []int{-3, 0, 7} {
		if got := reflejar(i, 1); got != 0 {
			t.Errorf("reflejar(%d, 1) = %d, quería 0", i, got)
		}
	}
}

// TestLaRCTEsExacta: es la transformada de color del modo sin pérdida, así que
// tiene que devolver los mismos enteros. Con la división de Go no lo hace.
func TestLaRCTEsExacta(t *testing.T) {
	r := []int32{0, 255, 128, 7, 200}
	g := []int32{0, 255, 64, 9, 10}
	b := []int32{0, 255, 32, 11, 250}

	// Transformada directa (§G.2), la que hace el codificador.
	y := make([]int32, len(r))
	u := make([]int32, len(r))
	v := make([]int32, len(r))
	for i := range r {
		y[i] = pisoDiv(r[i]+2*g[i]+b[i], 4)
		u[i] = b[i] - g[i]
		v[i] = r[i] - g[i]
	}
	rctInversa(y, u, v)
	for i := range r {
		if y[i] != r[i] || u[i] != g[i] || v[i] != b[i] {
			t.Errorf("píxel %d volvió como (%d,%d,%d) y era (%d,%d,%d)",
				i, y[i], u[i], v[i], r[i], g[i], b[i])
		}
	}
}
