// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominio

import (
	"encoding/json"
	"testing"
)

// La búsqueda y el filtro «contiene» de una vista en memoria siguen la regla de
// apps/nucleo/buscar: «reunion» encuentra «Reunión» y «MULLER» encuentra «Müller».
func TestLaVistaBuscaSinAcentosNiMayusculas(t *testing.T) {
	campos := []Campo{{ID: "t", Kind: TipoTexto}}
	regs := []Registro{
		{ID: "1", Data: map[string]json.RawMessage{"t": json.RawMessage(`"Reunión de equipo"`)}},
		{ID: "2", Data: map[string]json.RawMessage{"t": json.RawMessage(`"Frau Müller"`)}},
		{ID: "3", Data: map[string]json.RawMessage{"t": json.RawMessage(`"Otra cosa"`)}},
	}
	if got := FiltrarYOrdenar(campos, regs, ConfigVista{Search: "reunion"}); len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("búsqueda «reunion»: %+v", got)
	}
	filtro := Filtro{Field: "t", Op: "contains", Value: json.RawMessage(`"MULLER"`)}
	if got := FiltrarYOrdenar(campos, regs, ConfigVista{Filters: []Filtro{filtro}}); len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("filtro contiene «MULLER»: %+v", got)
	}
}
