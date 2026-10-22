// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Check de la tabla D.3 (contexto del signo).
//
// La tabla era un `switch` de nueve ramas y pasó a ser un array indexado, que es
// lo que la norma dice que es. Ese cambio no puede fallar ruidosamente: un índice
// desplazado devuelve un contexto plausible y lo único que se ve es que la imagen
// sale con ruido en los bordes, mucho después y sin nada que lo relacione.
//
// Por eso las nueve casillas van escritas UNA A UNA con su (h, v), y no
// generadas: una tabla derivada de la misma fórmula que prueba no prueba nada.
package domain

import "testing"

func TestTablaDelSignoCubreLasNueveCasillas(t *testing.T) {
	t.Parallel()
	// (h, v) → contexto, XOR. Tabla D.3 de la ISO/IEC 15444-1.
	casos := []struct {
		h, v int
		ctx  int
		xor  uint8
	}{
		{1, 1, 13, 0},
		{1, 0, 12, 0},
		{1, -1, 11, 0},
		{0, 1, 10, 0},
		{0, 0, 9, 0},
		{0, -1, 10, 1},
		{-1, 1, 11, 1},
		{-1, 0, 12, 1},
		{-1, -1, 13, 1},
	}
	for _, c := range casos {
		got := tablaSigno[(c.h+1)*3+(c.v+1)]
		if got.ctx != c.ctx || got.xor != c.xor {
			t.Errorf("h=%d v=%d: (%d, %d), quiero (%d, %d)", c.h, c.v, got.ctx, got.xor, c.ctx, c.xor)
		}
	}
}

// El recorte al rango de la tabla es lo que impide que un índice se salga: la
// suma de dos vecinos llega hasta ±2 y el array tiene nueve posiciones.
func TestRecorteDelRangoDeLaTabla(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ dentro, fuera int }{{-1, -2}, {1, 2}} {
		if got := recorta1(c.fuera); got != c.dentro {
			t.Errorf("recorta1(%d) = %d, quiero %d", c.fuera, got, c.dentro)
		}
	}
	for _, n := range []int{-1, 0, 1} {
		if got := recorta1(n); got != n {
			t.Errorf("recorta1(%d) = %d: lo que ya está dentro no se toca", n, got)
		}
	}
}
