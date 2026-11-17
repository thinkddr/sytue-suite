// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analisis

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// TestOcultasComoLaWeb recorre los vectores que comparte con sheets/ocultas.ts.
func TestOcultasComoLaWeb(t *testing.T) {
	datos, err := os.ReadFile("testdata/ocultas.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Casos []struct {
			Nombre   string          `json:"nombre"`
			Op       string          `json:"op"`
			Max      int             `json:"max"`
			Crudo    any             `json:"crudo"`
			Tramos   []Tramo         `json:"tramos"`
			A        int             `json:"a"`
			B        int             `json:"b"`
			Desde    int             `json:"desde"`
			Cantidad int             `json:"cantidad"`
			Esperado json.RawMessage `json:"esperado"`
		} `json:"casos"`
	}
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) == 0 {
		t.Fatal("sin vectores")
	}
	for _, c := range v.Casos {
		var got []Tramo
		ok := true
		switch c.Op {
		case "sanear":
			got = SanearTramos(c.Crudo, c.Max)
		case "ocultar":
			got, ok = OcultarTramo(c.Tramos, c.A, c.B, c.Max)
		case "mostrar":
			got = MostrarTramo(c.Tramos, c.A, c.B)
		case "mover":
			got = MoverTramos(c.Tramos, c.Desde, c.Cantidad, c.Max)
		default:
			t.Fatalf("%s: op desconocida %q", c.Nombre, c.Op)
		}
		if string(c.Esperado) == "null" {
			if ok {
				t.Errorf("%s: se esperaba que no se pudiera, y dio %v", c.Nombre, got)
			}
			continue
		}
		var want []Tramo
		if err := json.Unmarshal(c.Esperado, &want); err != nil {
			t.Fatalf("%s: %v", c.Nombre, err)
		}
		if !ok {
			t.Errorf("%s: se negó y se esperaba %v", c.Nombre, want)
			continue
		}
		if len(got) == 0 && len(want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v, se esperaba %v", c.Nombre, got, want)
		}
	}
}

// TestUnidadesDeTramosNoRecorreHastaElFinal: un tramo hasta el final de la rejilla (el
// millón de filas de «ocultar desde aquí») se corta en el límite que se pide.
func TestUnidadesDeTramosNoRecorreHastaElFinal(t *testing.T) {
	u := UnidadesDeTramos([]Tramo{{Start: 2, End: 3}, {Start: 8, End: 1 << 20}}, 10)
	want := map[int]bool{2: true, 3: true, 8: true, 9: true}
	if !reflect.DeepEqual(u, want) {
		t.Errorf("%v, se esperaba %v", u, want)
	}
	if !TramoOculto([]Tramo{{Start: 2, End: 3}}, 3) || TramoOculto([]Tramo{{Start: 2, End: 3}}, 4) {
		t.Error("TramoOculto no ve los extremos")
	}
}
