// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import "testing"

// El filtro de notas no mira acentos ni mayúsculas, en los dos sentidos.
func TestFiltrarSinAcentos(t *testing.T) {
	notas := []Nota{{ID: "1", Titulo: "Acta de la reunión"}, {ID: "2", Titulo: "Presupuesto", Contenido: "<p>la politica nueva</p>"}}
	for q, quiero := range map[string]string{"reunion": "1", "REUNIÓN": "1", "política": "2"} {
		if got := Filtrar(notas, q, ""); len(got) != 1 || got[0].ID != quiero {
			t.Errorf("Filtrar(%q) = %v, quiero %s", q, got, quiero)
		}
	}
}
