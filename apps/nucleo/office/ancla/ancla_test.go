// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ancla

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidar(t *testing.T) {
	largo := strings.Repeat("x", 301)
	casos := []struct {
		ancla string
		vale  bool
	}{
		{``, true},
		{`null`, true},
		{`{"mark":"abc","text":"hola"}`, true},
		{`"texto suelto"`, false},
		{`{"mark":"abc","text":"` + strings.Repeat("y", Max) + `"}`, false},
		{`{"dibujo":"d1","nodo":"n1","text":"forma"}`, true},
		{`{"dibujo":"d1","x":10,"y":20}`, true},
		{`{"dibujo":"d1","nodo":"n1","x":1,"y":2}`, false},
		{`{"dibujo":"d1","x":1}`, false},
		{`{"dibujo":"<d>"}`, false},
		{`{"dibujo":"d1","x":1e7,"y":0}`, false},
		{`{"hoja":"h1","fila":"t3","col":"n1a2","text":"B4"}`, true},
		{`{"hoja":"","fila":"t3","col":"t1"}`, false},
		{`{"hoja":"` + largo + `","fila":"t3","col":"t1"}`, false},
		{`{"hoja":"h1","fila":"t 3","col":"t1"}`, false},
		{`{"hoja":"h1","fila":"t3","col":"t1","mark":"abc"}`, false},
		{`{"hoja":"h1","fila":"t3","col":"t1","text":"` + largo + `"}`, false},
	}
	for _, c := range casos {
		if got := Validar(json.RawMessage(c.ancla)); (got == "") != c.vale {
			t.Errorf("Validar(%.80s) = %q, quiero vale=%v", c.ancla, got, c.vale)
		}
	}
}

func TestDeCeldaYMarca(t *testing.T) {
	if DeCelda("h1", "t0", "t1", "B1") == nil || DeCelda("h1", "t0", "", "B1") != nil || DeCelda("", "t0", "t1", "B1") != nil {
		t.Error("DeCelda no valida como el servicio")
	}
	if !EsMarca("0af3") || EsMarca("<x>") || EsMarca("ABC") {
		t.Error("EsMarca no valida como el servicio")
	}
}
