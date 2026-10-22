// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analisis

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type vectoresEsquema struct {
	Grupos []struct {
		Nombre   string          `json:"nombre"`
		Op       string          `json:"op"`
		Grupos   []Grupo         `json:"grupos"`
		Crudo    any             `json:"crudo"`
		A        int             `json:"a"`
		B        int             `json:"b"`
		Max      int             `json:"max"`
		Nivel    int             `json:"nivel"`
		Esperado json.RawMessage `json:"esperado"`
	} `json:"grupos"`
	Subtotales []struct {
		Nombre   string            `json:"nombre"`
		Celdas   map[string]string `json:"celdas"`
		Peticion struct {
			C1       int        `json:"c1"`
			R1       int        `json:"r1"`
			C2       int        `json:"c2"`
			R2       int        `json:"r2"`
			GroupCol int        `json:"groupCol"` //nolint:tagliatelle // lo escribe la web
			Fn       Agregacion `json:"fn"`
			Cols     []int      `json:"cols"`
			Remove   bool       `json:"remove"`
			Rows     int        `json:"rows"`
		} `json:"peticion"`
		Esperado struct {
			Error      string      `json:"error"`
			Desde      []int       `json:"desde"`
			Escrituras []Escritura `json:"escrituras"`
			Grupos     []Grupo     `json:"grupos"`
			UltimaFila int         `json:"ultimaFila"` //nolint:tagliatelle // el nombre lo fija el vector
			Bloques    int         `json:"bloques"`
		} `json:"esperado"`
	} `json:"subtotales"`
}

func leerVectores(t *testing.T) vectoresEsquema {
	t.Helper()
	datos, err := os.ReadFile("testdata/esquema.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectoresEsquema
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestEsquemaComoLaWeb recorre los vectores de grupos que comparte con outline.ts.
func TestEsquemaComoLaWeb(t *testing.T) {
	for _, c := range leerVectores(t).Grupos {
		var got any
		switch c.Op {
		case "agrupar":
			got = Agrupar(c.Grupos, c.A, c.B, c.Max)
		case "desagrupar":
			got = Desagrupar(c.Grupos, c.A, c.B)
		case "nivel":
			got = HastaNivel(c.Grupos, c.Nivel)
		case "conmutar":
			got = Conmutar(c.Grupos, c.A, c.B, c.Nivel)
		case "ocultas":
			oc := Ocultas(c.Grupos)
			us := make([]int, 0, len(oc))
			for u := range oc {
				us = append(us, u)
			}
			slices.Sort(us)
			got = us
		case "niveles":
			got = Niveles(c.Grupos)
		case "sanear":
			got = SanearGrupos(c.Crudo, c.Max)
		default:
			t.Fatalf("%s: operación %q desconocida", c.Nombre, c.Op)
		}
		mismo(t, c.Nombre, got, c.Esperado)
	}
}

// mismo compara por su JSON: es la forma en que la web los escribe.
func mismo(t *testing.T, nombre string, got any, esperado json.RawMessage) {
	t.Helper()
	var a, b any
	g, _ := json.Marshal(got)
	_ = json.Unmarshal(g, &a)
	_ = json.Unmarshal(esperado, &b)
	if a == nil { // una lista vacía de Go sale null
		a = []any{}
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("%s: sale %s, y la web da %s", nombre, g, esperado)
	}
}

// TestSubtotalesComoLaWeb recorre los vectores del asistente de Subtotales.
func TestSubtotalesComoLaWeb(t *testing.T) {
	for _, c := range leerVectores(t).Subtotales {
		crudo := func(col, f int) string { return c.Celdas[string(rune('A'+col))+strconv.Itoa(f+1)] }
		ultima := -1
		for k := range c.Celdas {
			n, _ := strconv.Atoi(k[1:])
			ultima = max(ultima, n-1)
		}
		p := c.Peticion
		res, err := AplicarSubtotales(PeticionSubtotal{
			C1: p.C1, F1: p.R1, C2: p.C2, F2: p.R2, ColGrupo: p.GroupCol, Fn: p.Fn, Cols: p.Cols,
			Quitar: p.Remove, Filas: p.Rows, UltimaUsada: ultima,
			RotuloTotal: "Total", RotuloGrande: "Total general", RotuloDe: func(k string) string { return "Total " + k },
		}, Lector{
			Crudo: crudo,
			Clave: crudo,
			EsNumero: func(col, f int) bool {
				_, e := strconv.ParseFloat(strings.TrimSpace(crudo(col, f)), 64)
				return e == nil
			},
			TieneAlgo: func(col, f int) bool { return crudo(col, f) != "" },
		})
		e := c.Esperado
		if err != "" || e.Error != "" {
			if err != e.Error {
				t.Errorf("%s: error %q, y la web da %q", c.Nombre, err, e.Error)
			}
			continue
		}
		if res.Escrituras == nil {
			res.Escrituras = []Escritura{}
		}
		if res.Grupos == nil {
			res.Grupos = []Grupo{}
		}
		if !slices.Equal(res.Desde, e.Desde) || !reflect.DeepEqual(res.Escrituras, e.Escrituras) ||
			!reflect.DeepEqual(res.Grupos, e.Grupos) || res.UltimaFila != e.UltimaFila || res.Bloques != e.Bloques {
			t.Errorf("%s:\n sale   %+v\n espera %+v", c.Nombre, res, e)
		}
	}
}
