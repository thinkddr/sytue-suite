// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"testing"
)

// Constantes matriciales ({1,2;3,4}), espejo de arrayliteral.test.ts.

func TestConstantesMatriciales(t *testing.T) {
	val := func(f string) Value {
		e := New(Options{})
		if err := e.SetCell("Hoja1", "A1", f); err != nil {
			t.Fatal(err)
		}
		v, _ := e.GetValue("Hoja1", "A1")
		return v
	}
	for f, want := range map[string]float64{
		"=SUM({1,2;3,4})":                        10,
		"=SUM({1,2,3}*2)":                        12,
		"=INDEX({10,20,30},2)":                   20,
		"=ROWS({1;2;3})":                         3,
		"=COLUMNS({1,2,3})":                      3,
		"=SUMPRODUCT({1,2,3},{4,5,6})":           32,
		"=MATCH(2,{1,2,3},0)":                    2,
		"=INDEX(FREQUENCY({1,2,3,4,5},{2,4}),2)": 2,
		"=LEN(\"{1,2}\")":                        5,
		"=SUM({1,2},{3})":                        6,
	} {
		if got, ok := val(f).(float64); !ok || got != want {
			t.Errorf("%s = %v, quería %v", f, val(f), want)
		}
	}
	if got, _ := val("=FVSCHEDULE(1000,{0.09,0.11,0.1})").(float64); math.Abs(got-1330.89) > 1e-8 {
		t.Errorf("FVSCHEDULE con constante: %v", got)
	}
	e := New(Options{})
	_ = e.SetCell("Hoja1", "A1", "={1,2;3,4}")
	if v, _ := e.GetValue("Hoja1", "B2"); v != 4.0 {
		t.Errorf("no derrama: B2 = %v", v)
	}
	_ = e.SetCell("Hoja1", "D1", `={"x","y"}`)
	if v, _ := e.GetValue("Hoja1", "E1"); v != "y" {
		t.Errorf("texto: E1 = %v", v)
	}
	for _, malo := range []string{"1,2;3", "A1", "1+1", `"abierto`, "1;;2,3"} {
		if _, err := parseArrayLiteral(malo); err == nil {
			t.Errorf("%q debería no parsear", malo)
		}
	}
	m, err := parseArrayLiteral(`"a;b","c,d";TRUE,-1.5e2`)
	if err != nil || m[0][0] != "a;b" || m[0][1] != "c,d" || m[1][0] != true || m[1][1] != -150.0 {
		t.Errorf("lectura: %v %v", m, err)
	}
}

// Una constante vuelve a escribirse como se leyó (desplazar y el editor la re-serializan).
func TestConstanteMatricialSeSerializa(t *testing.T) {
	n, err := parseFormula(`={1,"a ""b""";TRUE,#N/A}`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := serializar(n), `{1,"a ""b""";TRUE,#N/A}`; got != want {
		t.Errorf("serializar = %s", got)
	}
}

// Aritmética y comparación como Excel (comparacion.test.ts): igualdad a 15 cifras y
// desbordamiento = #NUM!.
func TestComparacionYDesbordamiento(t *testing.T) {
	val := func(f string) Value {
		e := New(Options{})
		_ = e.SetCell("Hoja1", "A1", f)
		v, _ := e.GetValue("Hoja1", "A1")
		return v
	}
	for f, want := range map[string]Value{
		"=0.1+0.2=0.3":      true,
		"=0.1+0.2<>0.3":     false,
		"=0.1+0.2<0.3":      false,
		"=0.1+0.2>=0.3":     true,
		"=(1/3)*3=1":        true,
		"=1=1.000000000001": false,
		"=0.3<0.30000001":   true,
		"=1E-20=2E-20":      false,
		"=2>1":              true,
		"=1E+308*10":        ErrNum,
		"=1E+308+1E+308":    ErrNum,
		"=-1E+308-1E+308":   ErrNum,
		"=1/0":              ErrDiv0,
		"=2^1024":           ErrNum,
	} {
		if got := val(f); got != want {
			t.Errorf("%s = %v, quería %v", f, got, want)
		}
	}
}
