// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidarValor(t *testing.T) {
	sel := Campo{Name: "Estado", Kind: TipoSeleccion, Choices: []string{"nuevo", "hecho"}}
	mul := Campo{Name: "Etiquetas", Kind: TipoMultiple, Choices: []string{"a", "b", "c"}}
	casos := []struct {
		campo Campo
		raw   string
		want  string // cadena vacía = error; "null" = vacío
	}{
		{Campo{Kind: TipoTexto}, `"hola"`, `"hola"`},
		{Campo{Kind: TipoTexto}, `""`, `null`},
		{Campo{Kind: TipoTexto}, `42`, ``},
		{Campo{Kind: TipoTexto}, `"a\u0000b"`, ``},
		{Campo{Kind: TipoTexto}, `"` + strings.Repeat("x", MaxLongTexto+1) + `"`, ``},
		{Campo{Kind: TipoNumero}, `12.5`, `12.5`},
		{Campo{Kind: TipoNumero}, `"12"`, ``},
		{Campo{Kind: TipoNumero}, `1e300`, ``},
		{Campo{Kind: TipoFecha}, `"2026-10-04"`, `"2026-10-04"`},
		{Campo{Kind: TipoFecha}, `"2026-02-30"`, ``},
		{Campo{Kind: TipoFecha}, `"04/10/2026"`, ``},
		{Campo{Kind: TipoCasilla}, `true`, `true`},
		{Campo{Kind: TipoCasilla}, `"true"`, ``},
		{sel, `"hecho"`, `"hecho"`},
		{sel, `"otro"`, ``},
		{mul, `["c","a","a"]`, `["a","c"]`},
		{mul, `["z"]`, ``},
		{mul, `[]`, `null`},
		{Campo{Kind: TipoTexto}, `null`, `null`},
		{Campo{Kind: TipoRelacion}, `"x"`, ``},
	}
	for _, c := range casos {
		got, err := ValidarValor(c.campo, json.RawMessage(c.raw))
		switch {
		case c.want == "" && err == nil:
			t.Errorf("%s %s: debía fallar y dio %s", c.campo.Kind, c.raw, got)
		case c.want == "null" && (err != nil || got != nil):
			t.Errorf("%s %s: esperaba vacío, dio %s, %v", c.campo.Kind, c.raw, got, err)
		case c.want != "" && c.want != "null" && (err != nil || string(got) != c.want):
			t.Errorf("%s %s: %s, %v; esperado %s", c.campo.Kind, c.raw, got, err, c.want)
		}
	}
}

func TestValidarDatosRechazaCampoDesconocidoYEnlaces(t *testing.T) {
	campos := []Campo{{ID: "f1", Kind: TipoTexto}, {ID: "f2", Kind: TipoRelacion}}
	if _, err := ValidarDatos(campos, map[string]json.RawMessage{"nope": json.RawMessage(`"x"`)}); err == nil {
		t.Error("un campo desconocido debe rechazarse")
	}
	if _, err := ValidarDatos(campos, map[string]json.RawMessage{"f2": json.RawMessage(`"x"`)}); err == nil {
		t.Error("un enlace no se escribe como dato")
	}
	got, err := ValidarDatos(campos, map[string]json.RawMessage{"f1": json.RawMessage(`""`)})
	if err != nil || len(got) != 0 {
		t.Errorf("un vacío se omite: %v %v", got, err)
	}
	grande := map[string]json.RawMessage{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		grande[id] = json.RawMessage(`"` + strings.Repeat("x", 9999) + `"`)
	}
	if err := TamanoDatos(grande); err == nil {
		t.Error("un registro de más de 60 KB debe rechazarse")
	}
}

func TestValidarEntradaCampo(t *testing.T) {
	malos := []EntradaCampo{
		{Name: "", Kind: TipoTexto},
		{Name: "x", Kind: "formula"},
		{Name: "x", Kind: TipoSeleccion},
		{Name: "x", Kind: TipoSeleccion, Choices: []string{"a", "a"}},
		{Name: "x", Kind: TipoRelacion, LinkTableID: "no-uuid"},
		{Name: "con\x01control", Kind: TipoTexto},
	}
	for i, m := range malos {
		if _, err := ValidarEntradaCampo(m); err == nil {
			t.Errorf("caso %d debía fallar: %+v", i, m)
		}
	}
	ok, err := ValidarEntradaCampo(EntradaCampo{Name: " Estado ", Kind: TipoSeleccion, Choices: []string{" nuevo "}, LinkTableID: "x"})
	if err != nil || ok.Name != "Estado" || ok.Choices[0] != "nuevo" || ok.LinkTableID != "" {
		t.Errorf("normalización: %+v %v", ok, err)
	}
}

func TestValidarEnlaces(t *testing.T) {
	u := "11111111-1111-1111-1111-111111111111"
	got, err := ValidarEnlaces([]string{u, u})
	if err != nil || len(got) != 1 {
		t.Errorf("duplicados: %v %v", got, err)
	}
	if _, err := ValidarEnlaces([]string{"x"}); err == nil {
		t.Error("id inválido")
	}
	if _, err := ValidarEnlaces(make([]string, MaxEnlaces+1)); err == nil {
		t.Error("demasiados enlaces")
	}
}

func TestValidarConfig(t *testing.T) {
	campos := []Campo{
		{ID: "t", Kind: TipoTexto},
		{ID: "n", Kind: TipoNumero},
		{ID: "s", Kind: TipoSeleccion, Choices: []string{"a"}},
		{ID: "m", Kind: TipoMultiple, Choices: []string{"a"}},
		{ID: "l", Kind: TipoRelacion},
	}
	bien := []ConfigVista{
		{Filters: []Filtro{{Field: "n", Op: "gt", Value: json.RawMessage(`3`)}}, Sort: []Orden{{Field: "t", Dir: "desc"}}},
		{Filters: []Filtro{{Field: "m", Op: "contains", Value: json.RawMessage(`"a"`)}}},
		{Filters: []Filtro{{Field: "l", Op: "notempty"}}},
	}
	for i, c := range bien {
		if _, err := ValidarConfig(VistaRejilla, c, campos); err != nil {
			t.Errorf("bien %d: %v", i, err)
		}
	}
	malos := []ConfigVista{
		{Filters: []Filtro{{Field: "nope", Op: "eq", Value: json.RawMessage(`"x"`)}}},
		{Filters: []Filtro{{Field: "t", Op: "gt", Value: json.RawMessage(`"x"`)}}},                   // operador que el tipo no admite
		{Filters: []Filtro{{Field: "t", Op: "eq; DROP TABLE bases", Value: json.RawMessage(`"x"`)}}}, // inyección por operador
		{Filters: []Filtro{{Field: "n", Op: "eq", Value: json.RawMessage(`"no número"`)}}},
		{Filters: []Filtro{{Field: "s", Op: "eq", Value: json.RawMessage(`"fuera de la lista"`)}}},
		{Sort: []Orden{{Field: "t", Dir: "asc; --"}}},
		{Sort: []Orden{{Field: "l", Dir: "asc"}}},
		{GroupBy: "t"},
		{Fields: []string{"nope"}},
		{Filters: make([]Filtro, MaxFiltros+1)},
	}
	for i, c := range malos {
		if _, err := ValidarConfig(VistaRejilla, c, campos); err == nil {
			t.Errorf("malo %d debía fallar: %+v", i, c)
		}
	}
	if _, err := ValidarConfig(VistaKanban, ConfigVista{}, campos); err == nil {
		t.Error("el tablero sin campo de agrupación debe fallar")
	}
	if _, err := ValidarConfig(VistaKanban, ConfigVista{GroupBy: "s"}, campos); err != nil {
		t.Errorf("tablero por selección: %v", err)
	}
	if _, err := ValidarConfig("gantt", ConfigVista{}, campos); err == nil {
		t.Error("tipo de vista desconocido")
	}
}

func TestAcotar(t *testing.T) {
	if got := Acotar(json.RawMessage(`"corto"`)); string(got) != `"corto"` {
		t.Errorf("corto: %s", got)
	}
	grande := json.RawMessage(`"` + strings.Repeat("x", 5000) + `"`)
	got := Acotar(grande)
	if len(got) > 100 || !strings.Contains(string(got), `"truncated":true`) {
		t.Errorf("grande: %s", got)
	}
	if Acotar(nil) != nil {
		t.Error("nil")
	}
}
