// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"fmt"
	"slices"
	"sort"
	"testing"
	"time"
)

// Espejo de packages/formula/src/incremental.test.ts: una edición solo reevalúa a sus
// dependientes, y las volátiles se reevalúan en cada edición aunque no lean nada de lo
// editado.

func valor(t *testing.T, e *Engine, ref string) Value {
	t.Helper()
	v, err := e.GetValue("Hoja1", ref)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func escribe(t *testing.T, e *Engine, ref, raw string) {
	t.Helper()
	if err := e.SetCell("Hoja1", ref, raw); err != nil {
		t.Fatal(err)
	}
}

func TestUnaEdicionSoloReevaluaASusDependientes(t *testing.T) {
	e := New(Options{})
	var refs []string
	for i := 1; i <= 100; i++ {
		escribe(t, e, fmt.Sprintf("A%d", i), fmt.Sprint(i))
		escribe(t, e, fmt.Sprintf("B%d", i), fmt.Sprintf("=A%d*2", i))
		refs = append(refs, fmt.Sprintf("B%d", i))
	}
	escribe(t, e, "C1", "=SUM(B1:B100)")
	refs = append(refs, "C1")
	leer := func() {
		for _, r := range refs {
			valor(t, e, r)
		}
	}
	leer()
	base := e.Evaluaciones()
	if base != 101 {
		t.Fatalf("carga: %d evaluaciones, quiero 101", base)
	}
	leer()
	if e.Evaluaciones() != base {
		t.Errorf("sin cambios debía servir la caché: %d", e.Evaluaciones()-base)
	}
	escribe(t, e, "A5", "500") // toca B5 y, por él, C1: nada más
	leer()
	if got := e.Evaluaciones() - base; got != 2 {
		t.Errorf("tras editar A5 se reevaluaron %d fórmulas, quiero 2", got)
	}
	if v := valor(t, e, "C1"); v != float64(10_100-10+1000) {
		t.Errorf("C1 = %v", v)
	}
}

func TestRangoGrandeSeInvalidaAlEditar(t *testing.T) {
	// A1:QH450 son 450 × 450 = 202.500 celdas, más que topeDeps: antes no tenía ninguna
	// dependencia y su caché quedaba rancia para siempre.
	e := New(Options{})
	escribe(t, e, "A1", "5")
	escribe(t, e, "QJ1", "=SUM(A1:QH450)")
	if v := valor(t, e, "QJ1"); v != 5.0 {
		t.Fatalf("QJ1 = %v", v)
	}
	escribe(t, e, "A2", "7")
	if v := valor(t, e, "QJ1"); v != 12.0 {
		t.Errorf("tras A2: QJ1 = %v, quiero 12", v)
	}
	escribe(t, e, "A1", "")
	if v := valor(t, e, "QJ1"); v != 7.0 {
		t.Errorf("tras vaciar A1: QJ1 = %v, quiero 7", v)
	}
}

func TestVolatilesLasSieteDeExcel(t *testing.T) {
	got := make([]string, 0, len(volatiles))
	for n := range volatiles {
		got = append(got, n)
	}
	sort.Strings(got)
	want := []string{"INDIRECT", "NOW", "OFFSET", "RAND", "RANDARRAY", "RANDBETWEEN", "TODAY"}
	if !slices.Equal(got, want) {
		t.Errorf("volátiles = %v", got)
	}
	if !EsVolatil("now") || EsVolatil("SUM") {
		t.Error("EsVolatil distingue mal")
	}
}

func TestVolatilesSeReevaluanEnCadaEdicion(t *testing.T) {
	var seg, azar int
	e := New(Options{
		Now:    func() time.Time { seg++; return time.Date(2026, 10, 2, 12, 0, seg, 0, time.UTC) },
		Random: func() float64 { azar++; return float64(azar) / 100 },
	})
	escribe(t, e, "A1", "=NOW()")
	escribe(t, e, "A2", "=RAND()")
	escribe(t, e, "A3", "=TODAY()")
	escribe(t, e, "B1", "=A2*100") // depende de una volátil: también cambia
	escribe(t, e, "B2", "=1+1")    // ni volátil ni depende de una
	a1, a2, b1 := valor(t, e, "A1"), valor(t, e, "A2"), valor(t, e, "B1")
	valor(t, e, "A3")
	valor(t, e, "B2")
	base := e.Evaluaciones()

	escribe(t, e, "Z1", "cualquier edición")
	if valor(t, e, "A1") == a1 || valor(t, e, "A2") == a2 || valor(t, e, "B1") == b1 {
		t.Error("las volátiles no cambiaron tras una edición")
	}
	valor(t, e, "A3")
	valor(t, e, "B2")
	if got := e.Evaluaciones() - base; got != 4 {
		t.Errorf("se reevaluaron %d fórmulas, quiero 4 (A1, A2, A3 y B1; B2 no)", got)
	}
}

func TestOffsetEIndirectSeReevaluanSiempre(t *testing.T) {
	e := New(Options{})
	escribe(t, e, "A1", "1")
	escribe(t, e, "A2", "2")
	escribe(t, e, "B1", `=INDIRECT("A"&2)`)
	escribe(t, e, "B2", "=OFFSET(A1,1,0)")
	if valor(t, e, "B1") != 2.0 || valor(t, e, "B2") != 2.0 {
		t.Fatal("valores iniciales")
	}
	escribe(t, e, "A2", "20") // ninguna de las dos lo declaró como dependencia
	if valor(t, e, "B1") != 20.0 || valor(t, e, "B2") != 20.0 {
		t.Errorf("rancias: B1=%v B2=%v", valor(t, e, "B1"), valor(t, e, "B2"))
	}
}

func TestVolatilidadPorNombreDefinido(t *testing.T) {
	e := New(Options{Random: func() float64 { return 0.25 }})
	e.DefineName("Azar", "RAND()", "")
	escribe(t, e, "A1", "=Azar*4")
	valor(t, e, "A1")
	base := e.Evaluaciones()
	escribe(t, e, "Z1", "x")
	valor(t, e, "A1")
	if e.Evaluaciones()-base != 1 {
		t.Error("volátil por el nombre: debía reevaluarse")
	}
	e.DefineName("Azar", "7", "") // ya no
	valor(t, e, "A1")
	base2 := e.Evaluaciones()
	escribe(t, e, "Z2", "y")
	valor(t, e, "A1")
	if e.Evaluaciones() != base2 {
		t.Error("dejó de ser volátil y se reevaluó")
	}
	if v := valor(t, e, "A1"); v != 28.0 {
		t.Errorf("A1 = %v, quiero 28", v)
	}
}
