// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"strings"
	"testing"
	"time"
)

// TestUnaCeldaHostilNoTumbaNiCongelaElMotor (FYE-02): cien mil paréntesis no
// desbordan la pila al cargar, dieciséis mil «-» no se evalúan en minutos, y un
// rango de miles de millones de filas no se materializa.
func TestUnaCeldaHostilNoTumbaNiCongelaElMotor(t *testing.T) {
	inicio := time.Now()
	e := New(Options{})
	e.SetCellAt("Hoja1", 0, 0, "="+strings.Repeat("(", 100_000)+"1"+strings.Repeat(")", 100_000))
	e.SetCellAt("Hoja1", 0, 1, "="+strings.Repeat("-", 16_000)+"1")
	e.SetCellAt("Hoja1", 0, 2, "=SUM(B1:B99999999999)")
	for fila := 0; fila < 3; fila++ {
		_ = e.ValueAt("Hoja1", 0, fila)
	}
	if v := e.ValueAt("Hoja1", 0, 2); v != ErrRef {
		t.Errorf("el rango gigante: %v, quiero #REF!", v)
	}
	if d := time.Since(inicio); d > 5*time.Second {
		t.Fatalf("tres celdas hostiles han tardado %v", d)
	}
	// Lo normal sigue igual.
	e.SetCellAt("Hoja1", 0, 3, "=((1+2))*--3")
	if v := e.ValueAt("Hoja1", 0, 3); v != float64(9) {
		t.Fatalf("=((1+2))*--3 = %v", v)
	}
}
