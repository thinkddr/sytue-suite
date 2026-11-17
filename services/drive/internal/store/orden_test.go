// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import "testing"

// TestParseOrdenFicheros: los valores del contrato HTTP y nada más.
func TestParseOrdenFicheros(t *testing.T) {
	t.Parallel()
	validos := map[[2]string]OrdenFicheros{
		{"", ""}:               {Campo: OrdenNombre},
		{"nombre", "asc"}:      {Campo: OrdenNombre},
		{"nombre", "desc"}:     {Campo: OrdenNombre, Desc: true},
		{"modificado", "desc"}: {Campo: OrdenModificado, Desc: true},
		{"tamano", ""}:         {Campo: OrdenTamano},
	}
	for in, want := range validos {
		got, err := ParseOrdenFicheros(in[0], in[1])
		if err != nil || got != want {
			t.Errorf("%q: %+v, %v; quiero %+v", in, got, err, want)
		}
	}
	for _, in := range [][2]string{{"tamaño", ""}, {"size", ""}, {"nombre", "DESC"}, {"", "up"}} {
		if _, err := ParseOrdenFicheros(in[0], in[1]); err == nil {
			t.Errorf("%q debería ser un error", in)
		}
	}
}
