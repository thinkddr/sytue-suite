// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"reflect"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLosValoresVanYVuelven: lo escrito en una celda sale como el valor de su tipo, y lo
// que no encaja no se envía (espejo de lib/valores.ts).
func TestLosValoresVanYVuelven(t *testing.T) {
	sel := api.BasesField{Kind: api.BasesKindSelect, Choices: []string{"Alta", "Baja"}}
	casos := []struct {
		campo  api.BasesField
		texto  string
		quiero any
		ok     bool
	}{
		{api.BasesField{Kind: api.BasesKindText}, " hola ", " hola ", true},
		{api.BasesField{Kind: api.BasesKindText}, "   ", nil, true},
		{api.BasesField{Kind: api.BasesKindNumber}, "3,5", 3.5, true},
		{api.BasesField{Kind: api.BasesKindNumber}, "-12", -12.0, true},
		{api.BasesField{Kind: api.BasesKindNumber}, "doce", nil, false},
		{api.BasesField{Kind: api.BasesKindNumber}, "1e16", nil, false},
		{api.BasesField{Kind: api.BasesKindDate}, "2026-10-05", "2026-10-05", true},
		{api.BasesField{Kind: api.BasesKindDate}, "05/10/2026", nil, false},
		{api.BasesField{Kind: api.BasesKindDate}, "2026-13-01", nil, false},
		{sel, "Alta", "Alta", true},
		{sel, "Media", "Media", false},
		{api.BasesField{Kind: api.BasesKindLink}, "x", nil, false},
	}
	for _, c := range casos {
		got, ok := valorDeTexto(c.campo, c.texto)
		if ok != c.ok || (ok && got != c.quiero) {
			t.Errorf("valorDeTexto(%s, %q) = %v, %v; quiero %v, %v", c.campo.Kind, c.texto, got, ok, c.quiero, c.ok)
		}
	}
	for valor, quiero := range map[any]string{"a": "a", 3.0: "3", 0.25: "0.25", true: "✓", false: "", nil: ""} {
		if got := textoDeValor(valor); got != quiero {
			t.Errorf("textoDeValor(%v) = %q, quiero %q", valor, got, quiero)
		}
	}
	if got := textoDeValor([]any{"a", "b"}); got != "a, b" {
		t.Errorf("una selección múltiple se lee %q", got)
	}
	campos := []api.BasesField{{ID: "c", Kind: api.BasesKindCheckbox}, {ID: "n", Kind: api.BasesKindText}}
	r := api.BasesRecord{ID: "0123456789ab", Data: map[string]any{"c": true, "n": "Ana"}}
	if got := etiquetaDeRegistro(campos, r); got != "Ana" {
		t.Errorf("la etiqueta es el primer campo con valor que no sea casilla: %q", got)
	}
	r.Data["n"] = nil
	if got := etiquetaDeRegistro(campos, r); got != "01234567" {
		t.Errorf("sin valores, la etiqueta es el principio del id: %q", got)
	}
}

func pag(cursor string, ids ...string) api.BasesRecordPage {
	p := api.BasesRecordPage{NextCursor: cursor}
	for _, id := range ids {
		p.Records = append(p.Records, api.BasesRecord{ID: id})
	}
	return p
}

func ids(v ventanas) []string {
	out := make([]string, 0, len(v.registros()))
	for _, r := range v.registros() {
		out = append(out, r.ID)
	}
	return out
}

// TestLasVentanasNoAcumulanLaTabla: como mucho tres páginas en memoria, y se vuelve atrás
// con el cursor con el que se pidió cada una (espejo de lib/ventanas.test.ts).
func TestLasVentanasNoAcumulanLaTabla(t *testing.T) {
	v := empezar(pag("c1", "a"))
	if _, ok := cursorAnterior(v); ok {
		t.Fatal("en la primera página no hay anterior")
	}
	v = avanzar(v, pag("c2", "b"))
	v = avanzar(v, pag("c3", "c"))
	if got := ids(v); !slices.Equal(got, []string{"a", "b", "c"}) || v.primera != 0 {
		t.Fatalf("tres páginas caben: %v desde %d", got, v.primera)
	}
	v = avanzar(v, pag("", "d"))
	if got := ids(v); !slices.Equal(got, []string{"b", "c", "d"}) || v.primera != 1 || v.siguiente != "" {
		t.Fatalf("la cuarta suelta la primera: %v desde %d, siguiente %q", got, v.primera, v.siguiente)
	}
	cursor, ok := cursorAnterior(v)
	if !ok || cursor != "" {
		t.Fatalf("la página 0 se pidió sin cursor: %q, %v", cursor, ok)
	}
	v = retroceder(v, pag("c1", "a"))
	if got := ids(v); !slices.Equal(got, []string{"a", "b", "c"}) || v.primera != 0 || v.siguiente != "c3" {
		t.Fatalf("al retroceder se suelta la última y se recuerda su cursor: %v, siguiente %q", got, v.siguiente)
	}
	v.editar(func(r api.BasesRecord) (api.BasesRecord, bool) { return r, r.ID != "b" })
	v.anadir(api.BasesRecord{ID: "z"})
	if got := ids(v); !slices.Equal(got, []string{"a", "c", "z"}) {
		t.Fatalf("borrar y añadir en lo que hay en pantalla: %v", got)
	}
	vacia := sinVentanas()
	vacia.anadir(api.BasesRecord{ID: "n"})
	if got := ids(vacia); !slices.Equal(got, []string{"n"}) {
		t.Fatalf("añadir a una tabla vacía: %v", got)
	}
}

// TestLaConfiguracionDeUnaVista: el borrador del diálogo sale como lo espera el servicio,
// y uno a medias no se envía (espejo de lib/vistas.test.ts).
func TestLaConfiguracionDeUnaVista(t *testing.T) {
	campos := []api.BasesField{
		{ID: "n", Kind: api.BasesKindText},
		{ID: "p", Kind: api.BasesKindNumber},
		{ID: "e", Kind: api.BasesKindSelect, Choices: []string{"Alta"}},
		{ID: "c", Kind: api.BasesKindCheckbox},
		{ID: "f", Kind: api.BasesKindFormula, Result: "number"},
		{ID: "l", Kind: api.BasesKindLink},
		{ID: "a", Kind: api.BasesKindAttachment},
	}
	casos := []struct {
		que    string
		b      borradorVista
		quiero api.BasesViewConfig
		ok     bool
	}{
		{"rejilla sin nada", borradorVista{tipo: api.BasesGrid}, api.BasesViewConfig{}, true},
		{
			"filtro numérico con coma y orden",
			borradorVista{tipo: api.BasesGrid, filtroCampo: "p", filtroOp: "gt", filtroValor: "1,5", ordenCampo: "n", ordenDir: "desc"},
			api.BasesViewConfig{Filters: []api.BasesFilter{{Field: "p", Op: "gt", Value: 1.5}}, Sort: []api.BasesSort{{Field: "n", Dir: "desc"}}},
			true,
		},
		{
			"una fórmula se filtra como su resultado",
			borradorVista{tipo: api.BasesGrid, filtroCampo: "f", filtroOp: "lte", filtroValor: "3"},
			api.BasesViewConfig{Filters: []api.BasesFilter{{Field: "f", Op: "lte", Value: 3.0}}},
			true,
		},
		{
			"vacío no lleva valor",
			borradorVista{tipo: api.BasesGrid, filtroCampo: "n", filtroOp: "empty", filtroValor: "x"},
			api.BasesViewConfig{Filters: []api.BasesFilter{{Field: "n", Op: "empty"}}},
			true,
		},
		{
			"casilla",
			borradorVista{tipo: api.BasesGrid, filtroCampo: "c", filtroOp: "eq", filtroValor: "false"},
			api.BasesViewConfig{Filters: []api.BasesFilter{{Field: "c", Op: "eq", Value: false}}},
			true,
		},
		{"filtro sin valor", borradorVista{tipo: api.BasesGrid, filtroCampo: "n", filtroOp: "contains"}, api.BasesViewConfig{}, false},
		{"operador que no es de ese tipo", borradorVista{tipo: api.BasesGrid, filtroCampo: "c", filtroOp: "gt", filtroValor: "1"}, api.BasesViewConfig{}, false},
		{"número que no lo es", borradorVista{tipo: api.BasesGrid, filtroCampo: "p", filtroOp: "eq", filtroValor: "mucho"}, api.BasesViewConfig{}, false},
		{"tablero", borradorVista{tipo: api.BasesKanban, agrupar: "e"}, api.BasesViewConfig{GroupBy: "e"}, true},
		{"tablero sin campo", borradorVista{tipo: api.BasesKanban}, api.BasesViewConfig{}, false},
		{"formulario", borradorVista{tipo: api.BasesForm, campos: []string{"n", "p"}}, api.BasesViewConfig{Fields: []string{"n", "p"}}, true},
		{"formulario sin campos", borradorVista{tipo: api.BasesForm}, api.BasesViewConfig{}, false},
	}
	for _, c := range casos {
		got, ok := configDeBorrador(c.b, campos)
		if ok != c.ok || (ok && !reflect.DeepEqual(got, c.quiero)) {
			t.Errorf("%s: %+v, %v; quiero %+v, %v", c.que, got, ok, c.quiero, c.ok)
		}
	}
	if len(operadoresDe(campos[6])) != 0 {
		t.Error("los adjuntos no se filtran")
	}
	formulario := make([]string, 0, len(campos))
	for _, c := range camposDeFormulario(campos) {
		formulario = append(formulario, c.ID)
	}
	if !slices.Equal(formulario, []string{"n", "p", "e", "c"}) {
		t.Errorf("un formulario no lleva enlaces, fórmulas ni adjuntos: %v", formulario)
	}
}

// TestElFocoDeLaRejillaSeMueveComoUnGrid (espejo de lib/rejilla.test.ts).
func TestElFocoDeLaRejillaSeMueveComoUnGrid(t *testing.T) {
	casos := []struct {
		tecla  string
		ctrl   bool
		desde  celda
		quiero celda
		mueve  bool
	}{
		{"Up", false, celda{0, 1}, celda{0, 1}, true},
		{"Down", false, celda{0, 1}, celda{1, 1}, true},
		{"Down", false, celda{29, 1}, celda{29, 1}, true},
		{"Right", false, celda{2, 3}, celda{2, 3}, true},
		{"Home", false, celda{5, 2}, celda{5, 0}, true},
		{"Home", true, celda{5, 2}, celda{0, 0}, true},
		{"End", true, celda{5, 2}, celda{29, 3}, true},
		{"Next", false, celda{5, 2}, celda{15, 2}, true},
		{"Prior", false, celda{5, 2}, celda{0, 2}, true},
		{"Return", false, celda{5, 2}, celda{5, 2}, false},
	}
	for _, c := range casos {
		got, mueve := moverFoco(c.desde, c.tecla, c.ctrl, 30, 4)
		if got != c.quiero || mueve != c.mueve {
			t.Errorf("%s desde %v = %v, %v; quiero %v, %v", c.tecla, c.desde, got, mueve, c.quiero, c.mueve)
		}
	}
	if _, mueve := moverFoco(celda{}, "Down", false, 0, 3); mueve {
		t.Error("sin filas no hay adónde ir")
	}
}

// TestLaFormulaSeGuardaPorIds (espejo de lib/formulas.test.ts).
func TestLaFormulaSeGuardaPorIds(t *testing.T) {
	lineas := api.BasesTable{ID: "t2", Fields: []api.BasesField{
		{ID: "i", Name: "Importe", Kind: api.BasesKindNumber}, {ID: "d", Name: "Detalle", Kind: api.BasesKindText},
	}}
	propios := []api.BasesField{
		{ID: "p", Name: "Precio", Kind: api.BasesKindNumber},
		{ID: "c", Name: "Cantidad", Kind: api.BasesKindNumber},
		{ID: "l", Name: "Líneas", Kind: api.BasesKindLink, LinkTableID: "t2"},
		{ID: "x", Name: "Ver. 2", Kind: api.BasesKindNumber},
	}
	tablas := []api.BasesTable{lineas}
	casos := map[string]string{
		"{Precio} * {cantidad}":            "{p} * {c}",
		"SUM({Líneas.Importe})":            "SUM({l.i})",
		`IF({Precio} > 3, "{caro}", "no")`: `IF({p} > 3, "{caro}", "no")`,
		"{Ver. 2} + 1":                     "{x} + 1",
	}
	for expr, quiero := range casos {
		if got, ok := formulaAIds(expr, propios, tablas); !ok || got != quiero {
			t.Errorf("formulaAIds(%q) = %q, %v; quiero %q", expr, got, ok, quiero)
		}
	}
	for _, mala := range []string{"{Coste}", "{Líneas.Nada}", "{Precio"} {
		if got, ok := formulaAIds(mala, propios, tablas); ok {
			t.Errorf("formulaAIds(%q) tendría que fallar y da %q", mala, got)
		}
	}
	quiero := []string{"{Precio}", "{Cantidad}", "{Líneas}", "{Líneas.Importe}", "{Ver. 2}"}
	if got := referenciasDeFormula(propios, tablas); !slices.Equal(got, quiero) {
		t.Errorf("referencias = %v, quiero %v", got, quiero)
	}
}
