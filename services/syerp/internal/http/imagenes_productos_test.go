// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"strings"
	"testing"
)

func TestIDsDeImagenesProductos(t *testing.T) {
	const id = "10000000-0000-4000-8000-000abc000001"
	ids, err := idsDeImagenesProductos(" " + id + "," + strings.ToUpper(id))
	if err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatalf("una sola consulta para identificadores repetidos: %v, %v", ids, err)
	}
	if ids, err := idsDeImagenesProductos(" "); err != nil || len(ids) != 0 {
		t.Fatalf("la página vacía devuelve un mapa vacío: %v, %v", ids, err)
	}
	for _, entrada := range []string{id + ",", "inventado", strings.Repeat(id+",", maxImagenesProductos) + id} {
		if _, err := idsDeImagenesProductos(entrada); err == nil {
			t.Errorf("no admite una consulta mal formada o sin límite: %q", entrada)
		}
	}
	if ids, err := idsDeImagenesProductos(strings.Repeat(id+",", maxImagenesProductos-1) + id); err != nil || len(ids) != 1 {
		t.Fatalf("admite el límite de 200: %v, %v", ids, err)
	}
}
