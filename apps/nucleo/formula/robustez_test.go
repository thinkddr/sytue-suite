// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"strings"
	"testing"
	"time"
)

// Ninguna función puede tirar la app ni colgarla con una entrada que nadie previó. Las
// trazas solo traen lo que los tests de la web pensaron; aquí se le da a CADA función
// registrada toda combinación de hasta tres argumentos de una lista de valores molestos
// (vacío, texto, error, rango, número enorme, negativo) con la red de evalCell QUITADA,
// para que un pánico se vea. El tope de tiempo caza los bucles sin acotar.
func TestNingunaFuncionEntraEnPanicoNiSeCuelga(t *testing.T) {
	sinRed = true
	defer func() { sinRed = false }()
	molestos := []string{`1`, `-1.5`, `0`, `"x"`, `""`, `TRUE`, `1E+300`, `A1:B2`, `1/0`, `C9`}
	combos := make([]string, 0, 1+(1+len(molestos)*7)*len(molestos))
	combos = append(combos, "")
	for _, a := range molestos {
		combos = append(combos, a)
		for _, b := range molestos {
			combos = append(combos, a+","+b)
			for _, c := range molestos[:6] {
				combos = append(combos, a+","+b+","+c)
			}
		}
	}
	e := New(Options{Now: func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, Random: func() float64 { return 0.5 }})
	for celda, raw := range map[string]string{"A1": "3", "B1": "hola", "A2": "=1/0", "B2": "TRUE"} {
		if err := e.SetCell("Hoja1", celda, raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, nombre := range FunctionNames() {
		inicio := time.Now()
		for _, args := range combos {
			formula := "=" + nombre + "(" + args + ")"
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s entra en pánico: %v", formula, r)
					}
				}()
				e.SetCellAt("Hoja1", 25, 0, formula)
				e.ValueAt("Hoja1", 25, 0)
			}()
		}
		if d := time.Since(inicio); d > 20*time.Second {
			t.Errorf("%s tarda %s con %d entradas: hay un bucle sin acotar", nombre, d.Round(time.Second), len(combos))
		}
	}
}

// Las que el parser o el motor resuelven sin pasar por el registro.
func TestLetLambdaYNombres(t *testing.T) {
	e := New(Options{})
	casos := map[string]Value{
		`=LET(x, 2, y, x*3, x+y)`:                    8.0,
		`=SUM(MAP(SEQUENCE(3), LAMBDA(v, v*v)))`:     14.0,
		`=REDUCE(0, SEQUENCE(4), LAMBDA(a, v, a+v))`: 10.0,
		`=LAMBDA(x, x)`:                              ErrCalc,
		`=NOEXISTE(1)`:                               ErrName,
		`=1+`:                                        ErrName,
		`=A1`:                                        ErrCirc,
		`="a"&1&TRUE`:                                "a1TRUE",
		`=-2^2`:                                      4.0,
		`=50%*2`:                                     1.0,
	}
	for formula, quiere := range casos {
		e.SetCellAt("Hoja1", 0, 0, formula)
		if got := e.ValueAt("Hoja1", 0, 0); got != quiere {
			t.Errorf("%s: quiere %v, da %v", formula, quiere, got)
		}
	}
	if !e.DefineName("IVA", "0.21", "") || e.DefineName("A1", "1", "") || e.DefineName("x y", "1", "") {
		t.Error("DefineName acepta o rechaza lo que no debe")
	}
	e.SetCellAt("Hoja1", 1, 0, "=100*IVA")
	if got := e.ValueAt("Hoja1", 1, 0); got != 21.0 {
		t.Errorf("nombre definido: %v", got)
	}
	if nombres := strings.Join(e.DefinedNameList(), ","); nombres != "IVA" {
		t.Errorf("nombres: %s", nombres)
	}
}

// Un nombre de hoja solo existe en su hoja y allí tapa al del libro (`defineName(…, true)`).
func TestNombreDeHoja(t *testing.T) {
	e := New(Options{Sheets: []string{"Hoja1", "Hoja2"}})
	if !e.DefineName("Tasa", "0.21", "") || !e.DefineSheetName("Tasa", "0.1", "Hoja2") || e.DefineSheetName("Tasa", "1", "NoExiste") {
		t.Fatal("DefineSheetName acepta o rechaza lo que no debe")
	}
	e.SetCellAt("Hoja1", 0, 0, "=Tasa")
	e.SetCellAt("Hoja2", 0, 0, "=Tasa")
	if a, b := e.ValueAt("Hoja1", 0, 0), e.ValueAt("Hoja2", 0, 0); a != 0.21 || b != 0.1 {
		t.Errorf("Hoja1 da %v y Hoja2 da %v; quiere 0.21 y 0.1", a, b)
	}
	if n := strings.Join(e.DefinedNameList(), ","); n != "TASA" {
		t.Errorf("nombres: %s", n)
	}
	e.DeleteName("Tasa")
	if v := e.ValueAt("Hoja1", 0, 0); v != ErrName {
		t.Errorf("sin el del libro, Hoja1 debería dar #NAME?, da %v", v)
	}
}
