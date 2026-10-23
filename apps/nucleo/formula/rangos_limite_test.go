// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"testing"
)

func TestDimensionRangoLimites(t *testing.T) {
	for _, caso := range []struct {
		nombre            string
		inicio, fin, tope int
		want              int
		ok                bool
	}{
		{"una celda mínima", math.MinInt, math.MinInt, 1, 1, true},
		{"una celda máxima", math.MaxInt, math.MaxInt, 1, 1, true},
		{"cerca del mínimo", math.MinInt, math.MinInt + 2, 3, 3, true},
		{"cerca del máximo", math.MaxInt - 2, math.MaxInt, 3, 3, true},
		{"cruza cero", -2, 2, 5, 5, true},
		{"tope inclusivo", 0, 4, 5, 5, true},
		{"excede tope", 0, 5, 5, 0, false},
		{"tope máximo", math.MinInt, -2, math.MaxInt, math.MaxInt, true},
		{"suma final desbordaría", 0, math.MaxInt, math.MaxInt, 0, false},
		{"diferencia máxima", math.MinInt, math.MaxInt, math.MaxInt, 0, false},
		{"diferencia desborda desde mínimo", math.MinInt, 0, math.MaxInt, 0, false},
		{"diferencia desborda hasta máximo", -1, math.MaxInt, math.MaxInt, 0, false},
		{"invertido", 1, 0, 1, 0, false},
		{"tope cero", 0, 0, 0, 0, false},
		{"tope negativo", 0, 0, -1, 0, false},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			got, ok := dimensionRango(caso.inicio, caso.fin, caso.tope)
			if got != caso.want || ok != caso.ok {
				t.Fatalf("dimensionRango(%d, %d, %d) = (%d, %v), quiero (%d, %v)",
					caso.inicio, caso.fin, caso.tope, got, ok, caso.want, caso.ok)
			}
		})
	}
}

func TestDimensionesRangoAcotaProducto(t *testing.T) {
	if filas, cols, ok := dimensionesRango(RefRange{R1: 0, R2: 1, C1: 0, C2: 2}, 6); !ok || filas != 2 || cols != 3 {
		t.Fatalf("rango en el tope: (%d, %d, %v)", filas, cols, ok)
	}
	// Cada dimensión cabe por separado, pero su producto no cabe en int.
	if _, _, ok := dimensionesRango(RefRange{R1: 0, R2: math.MaxInt - 1, C1: 0, C2: 1}, math.MaxInt); ok {
		t.Fatal("se aceptó un producto de dimensiones que desborda")
	}
}
