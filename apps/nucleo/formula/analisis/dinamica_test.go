// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analisis

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// rotulosVector son los rótulos de los vectores: los mismos que el test de la web.
var rotulosVector = RotulosDinamica{
	Blank: "(en blanco)", Grand: "Total general", All: "(Todos)", Multiple: "(Varios)",
	TotalOf:   func(k string) string { return "Total " + k },
	ValueName: func(fn, f string) string { return fn + " de " + f },
	Quarter:   func(q int) string { return "T" + formula.FormatNumber(float64(q)) },
	Month:     func(m int) string { return "M" + formula.FormatNumber(float64(m)) },
	Day:       func(d float64) string { return "D" + formula.FormatNumber(d) },
	Show: func(v formula.Value, f int) string {
		t := formula.Display(v)
		if f >= 0 {
			return t + "@" + formula.FormatNumber(float64(f))
		}
		return t
	},
}

// valorVector es una casilla del origen: {e} es un error.
func valorVector(x any) formula.Value {
	if m, ok := x.(map[string]any); ok {
		e, _ := m["e"].(string)
		return formula.Error(e)
	}
	return x
}

// casillaVector es la casilla como la escribe el vector.
func casillaVector(c *CasillaDinamica) map[string]any {
	if c == nil {
		return nil
	}
	o := map[string]any{"v": c.V}
	if c.Negrita {
		o["b"] = true
	}
	switch {
	case c.Fmt == nil:
	case c.Fmt.Pct:
		o["f"] = "p"
	case c.Fmt.Campo >= 0:
		o["f"] = "c" + formula.FormatNumber(float64(c.Fmt.Campo))
	default:
		o["f"] = "v" + formula.FormatNumber(float64(c.Fmt.Valor))
	}
	return o
}

// TestLaDinamicaComoLaWeb recorre los vectores que grabó la web.
func TestLaDinamicaComoLaWeb(t *testing.T) {
	datos, err := os.ReadFile("testdata/dinamica.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Casos []struct {
			Nombre   string             `json:"nombre"`
			Origen   [][]any            `json:"origen"`
			Def      DefDinamica        `json:"def"`
			Esperado [][]map[string]any `json:"esperado"`
		} `json:"casos"`
	}
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) < 30 {
		t.Fatalf("solo %d vectores", len(v.Casos))
	}
	for _, c := range v.Casos {
		origen := make([][]formula.Value, len(c.Origen))
		for i, f := range c.Origen {
			origen[i] = make([]formula.Value, len(f))
			for j, x := range f {
				origen[i][j] = valorVector(x)
			}
		}
		got := CalcularDinamica(origen, c.Def, rotulosVector)
		if len(got) != len(c.Esperado) {
			t.Errorf("%s: %d filas, y la web da %d", c.Nombre, len(got), len(c.Esperado))
			continue
		}
		for i, fila := range got {
			if len(fila) != len(c.Esperado[i]) {
				t.Errorf("%s: la fila %d tiene %d casillas, y la web %d", c.Nombre, i, len(fila), len(c.Esperado[i]))
				continue
			}
			for j, celda := range fila {
				if a, b := casillaVector(celda), c.Esperado[i][j]; !igualCasilla(a, b) {
					t.Errorf("%s: (%d, %d) sale %v, y la web da %v", c.Nombre, i, j, a, b)
				}
			}
		}
	}
}

// igualCasilla compara dos casillas; los números, con la tolerancia relativa de 1e-12.
func igualCasilla(a, b map[string]any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	x, xok := a["v"].(float64)
	y, yok := b["v"].(float64)
	if xok && yok {
		if math.Abs(x-y) > 1e-12*math.Max(1, math.Abs(y)) {
			return false
		}
		a, b = copia(a), copia(b)
		a["v"], b["v"] = 0.0, 0.0
	}
	return reflect.DeepEqual(a, b)
}

func copia(m map[string]any) map[string]any {
	o := make(map[string]any, len(m))
	for k, v := range m {
		o[k] = v
	}
	return o
}

// TestElSaneadoComoLaWeb: el mismo documento se lee igual que en la web.
func TestElSaneadoComoLaWeb(t *testing.T) {
	datos, err := os.ReadFile("testdata/dinamica.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Saneado []struct {
			Nombre   string `json:"nombre"`
			Crudo    any    `json:"crudo"`
			Esperado any    `json:"esperado"`
		} `json:"saneado"`
	}
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Saneado) < 5 {
		t.Fatalf("solo %d vectores de saneado", len(v.Saneado))
	}
	rango := func(s string) bool { _, ok := formula.ParseRangeRef(s); return ok }
	tal := func(x any) (any, bool) { return x, x != nil }
	for _, c := range v.Saneado {
		d, ok := SanearDinamica(c.Crudo, tal, rango)
		var got any
		if ok {
			b, _ := json.Marshal(d)
			_ = json.Unmarshal(b, &got)
		}
		if !reflect.DeepEqual(got, c.Esperado) {
			b, _ := json.Marshal(got)
			w, _ := json.Marshal(c.Esperado)
			t.Errorf("%s: sale %s, y la web da %s", c.Nombre, b, w)
		}
	}
}
