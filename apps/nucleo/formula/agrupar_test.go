// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"fmt"
	"reflect"
	"testing"
)

// Espejo de packages/formula/src/agrupar.test.ts: GROUPBY, PIVOTBY, PERCENTOF, TRIMRANGE,
// ISOMITTED y los LAMBDA con nombre llamados como función.

type datosHoja map[string]any

var ventas = datosHoja{
	"A1": "Región", "B1": "Producto", "C1": "Importe",
	"A2": "Norte", "B2": "Pan", "C2": 10.0,
	"A3": "Sur", "B3": "Pan", "C3": 20.0,
	"A4": "Norte", "B4": "Leche", "C4": 30.0,
	"A5": "Sur", "B5": "Leche", "C5": 40.0,
	"A6": "Norte", "B6": "Pan", "C6": 50.0,
}

func motorCon(t *testing.T, datos datosHoja, nombres map[string]string) *Engine {
	t.Helper()
	e := New(Options{})
	for n, d := range nombres {
		if !e.DefineName(n, d, "") {
			t.Fatalf("no se pudo definir %s", n)
		}
	}
	for ref, v := range datos {
		escribe(t, e, ref, fmt.Sprint(v))
	}
	return e
}

// rect evalúa en P1 y devuelve el rectángulo alto×ancho derramado; un error, como su código.
func rect(t *testing.T, formula string, alto, ancho int, datos datosHoja, nombres map[string]string) [][]any {
	t.Helper()
	e := motorCon(t, datos, nombres)
	escribe(t, e, "P1", formula)
	out := make([][]any, alto)
	for r := range out {
		out[r] = make([]any, ancho)
		for c := range out[r] {
			v := valor(t, e, fmt.Sprintf("%c%d", 'P'+c, r+1))
			if err, ok := v.(Error); ok {
				v = string(err)
			}
			out[r][c] = v
		}
	}
	return out
}

func uno1(t *testing.T, formula string, datos datosHoja, nombres map[string]string) any {
	t.Helper()
	return rect(t, formula, 1, 1, datos, nombres)[0][0]
}

func igual(t *testing.T, got, want [][]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestPercentOf(t *testing.T) {
	d := datosHoja{"A1": 10.0, "A2": 20.0, "A3": 30.0, "A4": 40.0}
	if v, _ := uno1(t, "=PERCENTOF(A1:A2,A1:A4)", d, nil).(float64); v < 0.2999999 || v > 0.3000001 {
		t.Errorf("PERCENTOF = %v", v)
	}
	if v := uno1(t, "=PERCENTOF(A1,A2)", datosHoja{"A1": 5.0, "A2": 0.0}, nil); v != "#DIV/0!" {
		t.Errorf("sobre cero: %v", v)
	}
	if v := uno1(t, "=PERCENTOF(A1)", datosHoja{"A1": 5.0}, nil); v != "#VALUE!" {
		t.Errorf("una sola: %v", v)
	}
}

func TestTrimRange(t *testing.T) {
	d := datosHoja{"B2": 1.0, "C2": 2.0, "B4": 3.0, "C4": 4.0}
	igual(t, rect(t, "=TRIMRANGE(A1:D5)", 3, 2, d, nil), [][]any{{1.0, 2.0}, {"", ""}, {3.0, 4.0}})
	d2 := datosHoja{"B2": 1.0, "B4": 2.0}
	igual(t, rect(t, "=TRIMRANGE(A1:B5,1,0)", 4, 2, d2, nil)[:1], [][]any{{"", 1.0}})
	igual(t, rect(t, "=TRIMRANGE(A1:B5,0,3)", 5, 1, d2, nil)[1:2], [][]any{{1.0}})
	if v := uno1(t, "=TRIMRANGE(A1:B2)", nil, nil); v != "#CALC!" {
		t.Errorf("todo en blanco: %v", v)
	}
	if v := uno1(t, "=TRIMRANGE(A1:B2,4)", datosHoja{"A1": 1.0}, nil); v != "#VALUE!" {
		t.Errorf("modo 4: %v", v)
	}
}

func TestArgumentoOmitido(t *testing.T) {
	if v := uno1(t, "=SUM(1,,3)", nil, nil); v != 4.0 {
		t.Errorf("SUM(1,,3) = %v", v)
	}
}

func TestLambdaConNombre(t *testing.T) {
	if v := uno1(t, "=DOBLE(21)", nil, map[string]string{"DOBLE": "LAMBDA(x,x*2)"}); v != 42.0 {
		t.Errorf("DOBLE = %v", v)
	}
	suma := map[string]string{"SUMA": "LAMBDA(a,b,a+b)"}
	for _, f := range []string{"=SUMA(1)", "=SUMA(1,2,3)"} {
		if v := uno1(t, f, nil, suma); v != "#VALUE!" {
			t.Errorf("%s = %v, quiero #VALUE!", f, v)
		}
	}
	mas := map[string]string{"MAS": "LAMBDA(a,[b],IF(ISOMITTED(b),a,a+b))"}
	for f, want := range map[string]float64{"=MAS(5)": 5, "=MAS(5,2)": 7, "=MAS(5,)": 5} {
		if v := uno1(t, f, nil, mas); v != want {
			t.Errorf("%s = %v, quiero %v", f, v, want)
		}
	}
	if v := uno1(t, "=F(1,2)", nil, map[string]string{"F": "LAMBDA([a],b,a+b)"}); v != "#VALUE!" {
		t.Errorf("opcional antes de obligatorio: %v", v)
	}
	if v := uno1(t, "=ISOMITTED(x)", nil, nil); v != "#NAME?" {
		t.Errorf("ISOMITTED fuera de un LAMBDA: %v", v)
	}
	if v := uno1(t, "=ISOMITTED(1)", nil, nil); v != "#VALUE!" {
		t.Errorf("ISOMITTED(1): %v", v)
	}
}

func TestLambdaConNombreEnOrdenSuperiorYRecursion(t *testing.T) {
	d := datosHoja{"A1": 1.0, "A2": 2.0, "A3": 3.0}
	got := rect(t, "=MAP(A1:A3,DOBLE)", 3, 1, d, map[string]string{"DOBLE": "LAMBDA(x,x*2)"})
	igual(t, got, [][]any{{2.0}, {4.0}, {6.0}})
	got = rect(t, "=BYROW(A1:B2,SUM)", 2, 1, datosHoja{"A1": 1.0, "B1": 2.0, "A2": 3.0, "B2": 4.0}, nil)
	igual(t, got, [][]any{{3.0}, {7.0}})
	if v := uno1(t, "=FACT2(5)", nil, map[string]string{"FACT2": "LAMBDA(n,IF(n<=1,1,n*FACT2(n-1)))"}); v != 120.0 {
		t.Errorf("FACT2(5) = %v", v)
	}
	if v := uno1(t, "=INF(1)", nil, map[string]string{"INF": "LAMBDA(n,INF(n+1))"}); v != "#NUM!" {
		t.Errorf("recursión sin fin: %v", v)
	}
}

func TestLambdaConNombreDependenciasYVolatilidad(t *testing.T) {
	e := motorCon(t, datosHoja{"A1": 10.0}, map[string]string{"SUMAA1": "LAMBDA(x,x+A1)"})
	escribe(t, e, "B1", "=SUMAA1(1)")
	if v := valor(t, e, "B1"); v != 11.0 {
		t.Fatalf("B1 = %v", v)
	}
	escribe(t, e, "A1", "20")
	if v := valor(t, e, "B1"); v != 21.0 {
		t.Errorf("tras editar A1: B1 = %v", v)
	}

	n := 0.0
	v := New(Options{Random: func() float64 { n++; return n / 100 }})
	v.DefineName("AZAR", "LAMBDA(x,x+RAND())", "")
	escribe(t, v, "B1", "=AZAR(1)")
	antes := valor(t, v, "B1")
	escribe(t, v, "H9", "nota")
	if despues := valor(t, v, "B1"); despues == antes {
		t.Errorf("una LAMBDA volátil con nombre no se reevaluó: %v", despues)
	}
}

func TestGroupBy(t *testing.T) {
	igual(t, rect(t, "=GROUPBY(A1:A6,C1:C6,SUM)", 4, 2, ventas, nil),
		[][]any{{"Región", "Importe"}, {"Norte", 90.0}, {"Sur", 60.0}, {"Total", 150.0}})
	d := datosHoja{"A1": "a", "A2": "b", "A3": "a", "B1": 1.0, "B2": 5.0, "B3": 2.0}
	igual(t, rect(t, "=GROUPBY(A1:A3,B1:B3,SUM,0,0,-2)", 2, 2, d, nil), [][]any{{"b", 5.0}, {"a", 3.0}})
	if got := rect(t, "=GROUPBY(A2:A6,C2:C6,SUM,0,-1)", 3, 2, ventas, nil)[0]; !reflect.DeepEqual(got, []any{"Total", 150.0}) {
		t.Errorf("total arriba: %v", got)
	}
	igual(t, rect(t, "=GROUPBY(A2:B6,C2:C6,SUM,0,0)", 4, 3, ventas, nil), [][]any{
		{"Norte", "Leche", 30.0}, {"Norte", "Pan", 60.0}, {"Sur", "Leche", 40.0}, {"Sur", "Pan", 20.0},
	})
	igual(t, rect(t, "=GROUPBY(A2:B6,C2:C6,SUM,0,2)", 7, 3, ventas, nil), [][]any{
		{"Norte", "Leche", 30.0},
		{"Norte", "Pan", 60.0},
		{"Norte", "Total", 90.0},
		{"Sur", "Leche", 40.0},
		{"Sur", "Pan", 20.0},
		{"Sur", "Total", 60.0},
		{"Total", "", 150.0},
	})
	p := rect(t, "=GROUPBY(A2:A6,C2:C6,PERCENTOF,0,0)", 2, 2, ventas, nil)
	if p[0][0] != "Norte" || p[0][1].(float64) < 0.5999999 || p[0][1].(float64) > 0.6000001 {
		t.Errorf("PERCENTOF: %v", p)
	}
	igual(t, rect(t, `=GROUPBY(A2:A6,C2:C6,LAMBDA(g,MAX(g)),0,0,,B2:B6<>"Leche")`, 2, 2, ventas, nil),
		[][]any{{"Norte", 50.0}, {"Sur", 20.0}})
	m := datosHoja{"A1": "x", "A2": "x", "A3": "y", "B1": 1.0, "B2": 2.0, "B3": 3.0, "C1": 10.0, "C2": 20.0, "C3": 30.0}
	igual(t, rect(t, "=GROUPBY(A1:A3,B1:C3,SUM,0,0)", 2, 3, m, nil), [][]any{{"x", 3.0, 30.0}, {"y", 3.0, 30.0}})
	igual(t, rect(t, "=GROUPBY(A2:A3,C2:C3,LAMBDA(g,1/0),0,0)", 2, 2, ventas, nil),
		[][]any{{"Norte", "#DIV/0!"}, {"Sur", "#DIV/0!"}})
}

func TestGroupByErrores(t *testing.T) {
	casos := map[string]string{
		"=GROUPBY(A2:A6,C2:C5,SUM)":                "#VALUE!",
		"=GROUPBY(A2:A6,C2:C6,SUM,0,9)":            "#VALUE!",
		"=GROUPBY(A2:A6,C2:C6,SUM,0,1,7)":          "#VALUE!",
		"=GROUPBY(A2:A6,C2:C6,SUM,0,1,,C2:C6>999)": "#CALC!",
		"=GROUPBY(A2:A6,C2:C6)":                    "#VALUE!",
	}
	for f, want := range casos {
		if v := uno1(t, f, ventas, nil); v != want {
			t.Errorf("%s = %v, quiero %s", f, v, want)
		}
	}
}

func TestPivotBy(t *testing.T) {
	igual(t, rect(t, "=PIVOTBY(A1:A6,B1:B6,C1:C6,SUM)", 4, 4, ventas, nil), [][]any{
		{"Región", "Leche", "Pan", "Total"},
		{"Norte", 30.0, 60.0, 90.0},
		{"Sur", 40.0, 20.0, 60.0},
		{"Total", 70.0, 80.0, 150.0},
	})
	d := datosHoja{"A1": "a", "A2": "b", "B1": "x", "B2": "y", "C1": 1.0, "C2": 2.0}
	igual(t, rect(t, "=PIVOTBY(A1:A2,B1:B2,C1:C2,SUM,0,0,,0)", 3, 3, d, nil), [][]any{
		{"", "x", "y"}, {"a", 1.0, ""}, {"b", "", 2.0},
	})
	m := rect(t, "=PIVOTBY(A2:A6,B2:B6,C2:C6,SUM,0,-1,,-1)", 4, 4, ventas, nil)
	if m[0][0] != "" || m[0][1] != "Total" || !reflect.DeepEqual(m[1], []any{"Total", 150.0, 70.0, 80.0}) {
		t.Errorf("totales arriba: %v", m)
	}
	for _, f := range []string{"=PIVOTBY(A2:A6,B2:B6,C2:D6,SUM)", "=PIVOTBY(A2:B6,B2:B6,C2:C6,SUM,0,2)"} {
		if v := uno1(t, f, ventas, nil); v != "#VALUE!" {
			t.Errorf("%s = %v, quiero #VALUE!", f, v)
		}
	}
}
