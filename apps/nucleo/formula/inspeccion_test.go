// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"errors"
	"slices"
	"testing"
)

func TestInspeccionar(t *testing.T) {
	u, err := Inspeccionar(`=IF(sum(A1:A3, 2) > Hoja2!B1, "sí", ROUND(tasa * 2, 1)) + {1,2}`)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(u.Funciones, []string{"IF", "ROUND", "SUM"}) {
		t.Errorf("funciones: %v", u.Funciones)
	}
	// A1:A3, Hoja2!B1 y el nombre «tasa».
	if u.Referencias != 3 || u.Matrices != 1 {
		t.Errorf("referencias %d, matrices %d", u.Referencias, u.Matrices)
	}
	if u.Nodos < 10 || u.Profundidad < 4 {
		t.Errorf("nodos %d, profundidad %d", u.Nodos, u.Profundidad)
	}
	if u, err := Inspeccionar(`1 + 2 * 3`); err != nil || u.Referencias != 0 || len(u.Funciones) != 0 {
		t.Errorf("sin referencias: %+v %v", u, err)
	}
	if _, err := Inspeccionar(`=SUM(`); !errors.Is(err, ErrSintaxis) {
		t.Errorf("sintaxis: %v", err)
	}
}
