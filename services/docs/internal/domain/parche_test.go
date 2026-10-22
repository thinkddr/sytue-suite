// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func ruta(t *testing.T, segs ...any) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, len(segs))
	for i, s := range segs {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = b
	}
	return out
}

func id(x string) map[string]string { return map[string]string{"id": x} }

func TestAplicarCambios(t *testing.T) {
	libro := `{"active":0,"sheets":[{"id":"h1","name":"A","cells":{"A1":"1"},"w":1.50}]}`
	casos := []struct {
		nombre string
		c      Cambio
		quiero string
	}{
		{
			"celda existente",
			Cambio{Path: ruta(t, "sheets", id("h1"), "cells", "A1"), Value: json.RawMessage(`"2"`)},
			`{"active":0,"sheets":[{"cells":{"A1":"2"},"id":"h1","name":"A","w":1.50}]}`,
		},
		{
			"celda nueva y contenedor nuevo",
			Cambio{Path: ruta(t, "sheets", id("h1"), "styles", "B2"), Value: json.RawMessage(`{"b":true}`)},
			`{"active":0,"sheets":[{"cells":{"A1":"1"},"id":"h1","name":"A","styles":{"B2":{"b":true}},"w":1.50}]}`,
		},
		{
			"borrar celda",
			Cambio{Path: ruta(t, "sheets", id("h1"), "cells", "A1"), Del: true},
			`{"active":0,"sheets":[{"cells":{},"id":"h1","name":"A","w":1.50}]}`,
		},
		{"hoja borrada por otro: no resucita", Cambio{Path: ruta(t, "sheets", id("h9"), "cells", "A1"), Value: json.RawMessage(`"x"`)}, libro2()},
		{
			"hoja nueva al final con la identidad de la ruta",
			Cambio{Path: ruta(t, "sheets", id("h2")), Value: json.RawMessage(`{"name":"B","cells":{}}`)},
			`{"active":0,"sheets":[{"cells":{"A1":"1"},"id":"h1","name":"A","w":1.50},{"cells":{},"id":"h2","name":"B"}]}`,
		},
		{"quitar hoja", Cambio{Path: ruta(t, "sheets", id("h1")), Del: true}, `{"active":0,"sheets":[]}`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := AplicarCambios(libro, []Cambio{c.c})
			if err != nil {
				t.Fatal(err)
			}
			if got != c.quiero {
				t.Fatalf("\n got  %s\n want %s", got, c.quiero)
			}
		})
	}
}

// libro2 es el libro de la tabla tras un cambio descartado (claves ordenadas, como lo escribe Go).
func libro2() string {
	return `{"active":0,"sheets":[{"cells":{"A1":"1"},"id":"h1","name":"A","w":1.50}]}`
}

func TestParcheValidacion(t *testing.T) {
	ok := Op{Kind: OpPatch, Key: ClaveHoja, OpID: "o1", Cambios: []Cambio{{Path: ruta(t, "sheets"), Del: true}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for nombre, mal := range map[string]Op{
		"sin op_id":                {Kind: OpPatch, Key: ClaveHoja, Cambios: ok.Cambios},
		"clave ajena":              {Kind: OpPatch, Key: "doc:html", OpID: "o", Cambios: ok.Cambios},
		"sin cambios":              {Kind: OpPatch, Key: ClaveHoja, OpID: "o"},
		"ruta con id al principio": {Kind: OpPatch, Key: ClaveHoja, OpID: "o", Cambios: []Cambio{{Path: ruta(t, id("x"), "a"), Del: true}}},
		"sin valor ni del":         {Kind: OpPatch, Key: ClaveHoja, OpID: "o", Cambios: []Cambio{{Path: ruta(t, "a")}}},
	} {
		if err := mal.Validate(); !errors.Is(err, ErrInvalidOp) {
			t.Errorf("%s: %v, quiero ErrInvalidOp", nombre, err)
		}
	}
}
