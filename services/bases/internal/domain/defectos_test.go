// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestValidarDefecto(t *testing.T) {
	estado := Campo{Name: "Estado", Kind: TipoSeleccion, Choices: []string{"Nuevo", "Hecho"}}
	// want vacío es que falla; «null», que lo quita.
	casos := []struct {
		c    Campo
		raw  string
		want string
	}{
		{estado, `"Nuevo"`, `"Nuevo"`},
		{estado, `"Inventado"`, ``},
		{estado, `null`, `null`},
		{Campo{Kind: TipoFecha}, `"today"`, `"today"`},
		{Campo{Kind: TipoFecha}, `"2026-11-25"`, `"2026-11-25"`},
		{Campo{Kind: TipoFecha}, `"mañana"`, ``},
		// «today» solo es especial en una fecha: en un texto es un texto más.
		{Campo{Kind: TipoTexto}, `"today"`, `"today"`},
		{Campo{Kind: TipoCasilla}, `true`, `true`},
		{Campo{Kind: TipoMoneda}, `9.999`, `10`},
		{Campo{Kind: TipoRelacion}, `"x"`, ``},
		{Campo{Kind: TipoFormula}, `1`, ``},
		{Campo{Kind: TipoAdjunto}, `"x"`, ``},
	}
	for _, c := range casos {
		v, err := ValidarDefecto(c.c, json.RawMessage(c.raw))
		switch {
		case c.want == "":
			if !errors.Is(err, ErrInvalido) {
				t.Errorf("%s %s: quería error, dio %s (%v)", c.c.Kind, c.raw, v, err)
			}
		case c.want == "null":
			if err != nil || v != nil {
				t.Errorf("%s %s: quería quitarlo, dio %s (%v)", c.c.Kind, c.raw, v, err)
			}
		case err != nil || string(v) != c.want:
			t.Errorf("%s %s: %s (%v), quería %s", c.c.Kind, c.raw, v, err, c.want)
		}
	}
}

// El defecto rellena lo que el alta NO trae; lo que trae, aunque sea null, manda. «Hoy»
// es el día que se le pasa, y un defecto que ya no vale (una opción retirada) se salta
// en vez de tumbar el alta.
func TestConDefectos(t *testing.T) {
	hoy := time.Date(2026, 11, 25, 23, 30, 0, 0, time.UTC)
	campos := []Campo{
		{ID: "estado", Kind: TipoSeleccion, Choices: []string{"Nuevo"}, Default: json.RawMessage(`"Nuevo"`)},
		{ID: "alta", Kind: TipoFecha, Default: json.RawMessage(`"today"`)},
		{ID: "notas", Kind: TipoTexto, Default: json.RawMessage(`"Sin revisar"`)},
		{ID: "retirada", Kind: TipoSeleccion, Choices: []string{"B"}, Default: json.RawMessage(`"A"`)},
		{ID: "sin", Kind: TipoNumero},
	}
	entrada := map[string]json.RawMessage{"notas": json.RawMessage(`null`)}
	got := ConDefectos(campos, entrada, nil, hoy)
	want := map[string]string{"estado": `"Nuevo"`, "alta": `"2026-11-25"`}
	if len(got) != len(want) {
		t.Fatalf("datos: %v, quería %v", got, want)
	}
	for k, v := range want {
		if string(got[k]) != v {
			t.Errorf("%s = %s, quería %s", k, got[k], v)
		}
	}
	// Lo escrito no se pisa.
	got = ConDefectos(campos, map[string]json.RawMessage{"estado": json.RawMessage(`"Nuevo"`)},
		map[string]json.RawMessage{"estado": json.RawMessage(`"Nuevo"`)}, hoy)
	if string(got["estado"]) != `"Nuevo"` {
		t.Errorf("estado: %s", got["estado"])
	}
}

func TestValidarConfigObligatoriosYFechasRelativas(t *testing.T) {
	campos := []Campo{
		{ID: "nombre", Kind: TipoTexto},
		{ID: "alta", Kind: TipoFecha},
		{ID: "total", Kind: TipoFormula, Result: TipoNumero},
		{ID: "notas", Kind: TipoTexto},
	}
	n := func(x float64) json.RawMessage { b, _ := json.Marshal(x); return b }
	casos := []struct {
		nombre string
		kind   TipoVista
		cfg    ConfigVista
		vale   bool
	}{
		{"obligatorio del formulario", VistaFormulario, ConfigVista{Fields: []string{"nombre"}, Required: []string{"nombre"}}, true},
		{"sin lista, todos los admitidos", VistaFormulario, ConfigVista{Required: []string{"notas"}}, true},
		{"obligatorio que no sale", VistaFormulario, ConfigVista{Fields: []string{"nombre"}, Required: []string{"notas"}}, false},
		{"una fórmula no se rellena", VistaFormulario, ConfigVista{Required: []string{"total"}}, false},
		{"repetido", VistaFormulario, ConfigVista{Required: []string{"nombre", "nombre"}}, false},
		{"campo inventado", VistaFormulario, ConfigVista{Required: []string{"x"}}, false},
		{"hoy", VistaRejilla, ConfigVista{Filters: []Filtro{{Field: "alta", Op: OpHoy}}}, true},
		{"este mes", VistaRejilla, ConfigVista{Filters: []Filtro{{Field: "alta", Op: OpEsteMes}}}, true},
		{"últimos 7 días", VistaRejilla, ConfigVista{Filters: []Filtro{{Field: "alta", Op: OpUltimosDias, Value: n(7)}}}, true},
		{"próximos 0 días", VistaRejilla, ConfigVista{Filters: []Filtro{{Field: "alta", Op: OpProximosDia, Value: n(0)}}}, false},
		{"días con decimales", VistaRejilla, ConfigVista{Filters: []Filtro{{Field: "alta", Op: OpUltimosDias, Value: n(1.5)}}}, false},
		{"días de más", VistaRejilla, ConfigVista{Filters: []Filtro{{Field: "alta", Op: OpUltimosDias, Value: n(MaxDiasRelativos + 1)}}}, false},
		{"relativo en un texto", VistaRejilla, ConfigVista{Filters: []Filtro{{Field: "nombre", Op: OpHoy}}}, false},
	}
	for _, c := range casos {
		out, err := ValidarConfig(c.kind, c.cfg, campos)
		if (err == nil) != c.vale {
			t.Errorf("%s: err = %v, quería válido = %v", c.nombre, err, c.vale)
		}
		if err == nil && c.kind != VistaFormulario && out.Required != nil {
			t.Errorf("%s: solo un formulario guarda obligatorios", c.nombre)
		}
	}
	// Una vista que no es formulario no guarda obligatorios aunque se los manden.
	out, err := ValidarConfig(VistaRejilla, ConfigVista{Required: []string{"nombre"}}, campos)
	if err != nil || out.Required != nil {
		t.Errorf("rejilla con obligatorios: %+v %v", out, err)
	}
}
