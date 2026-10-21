// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// TestSubtotalYAggregateComoLaWeb recorre los vectores que comparte con el motor de la
// web (packages/formula/src/functions/aggregate.test.ts): el mismo caso, el mismo valor.
func TestSubtotalYAggregateComoLaWeb(t *testing.T) {
	datos, err := os.ReadFile("testdata/subtotal.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Casos []struct {
			Nombre    string            `json:"nombre"`
			Celdas    map[string]string `json:"celdas"`
			Filtradas []int             `json:"filtradas"`
			Manuales  []int             `json:"manuales"`
			Formula   string            `json:"formula"`
			Esperado  struct {
				N *float64 `json:"n"`
				E string   `json:"e"`
			} `json:"esperado"`
		} `json:"casos"`
	}
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) < 40 {
		t.Fatalf("solo %d vectores", len(v.Casos))
	}
	for _, c := range v.Casos {
		e := New(Options{})
		for ref, raw := range c.Celdas {
			if err := e.SetCell("Hoja1", ref, raw); err != nil {
				t.Fatal(err)
			}
		}
		e.SetHiddenRows("Hoja1", c.Filtradas, c.Manuales)
		_ = e.SetCell("Hoja1", "Z1", c.Formula)
		got, _ := e.GetValue("Hoja1", "Z1")
		if c.Esperado.E != "" {
			if got != Error(c.Esperado.E) {
				t.Errorf("%s: %s = %v, y la web da %s", c.Nombre, c.Formula, got, c.Esperado.E)
			}
			continue
		}
		n, ok := got.(float64)
		want := *c.Esperado.N
		if !ok || math.Abs(n-want) > 1e-12*math.Max(1, math.Abs(want)) {
			t.Errorf("%s: %s = %v, y la web da %v", c.Nombre, c.Formula, got, want)
		}
	}
}

// TestContraerUnGrupoRecalcula: el motor memoriza, así que cambiar las filas ocultas
// tiene que vaciar lo calculado o el total se queda con el valor de antes.
func TestContraerUnGrupoRecalcula(t *testing.T) {
	e := New(Options{})
	for ref, raw := range map[string]string{"A1": "1", "A2": "2", "A3": "3", "B1": "=SUBTOTAL(109,A1:A3)"} {
		_ = e.SetCell("Hoja1", ref, raw)
	}
	for _, paso := range []struct {
		manuales []int
		want     float64
	}{{nil, 6}, {[]int{1}, 4}, {nil, 6}} {
		e.SetHiddenRows("Hoja1", nil, paso.manuales)
		if got := e.ValueAt("Hoja1", 1, 0); got != paso.want {
			t.Errorf("con %v ocultas, B1 = %v; quería %v", paso.manuales, got, paso.want)
		}
	}
}
