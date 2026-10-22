// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
)

// «Actualizar a la marca vigente»: de la v1 a la v2 el diff dice lo que cambiará, aplicarlo
// no toca el contenido, volver a previsualizar no encuentra nada (idempotente) y la marca
// v1 puede reaplicarse (el servicio guarda además el punto de restauración).
func TestMigrarMarcaEntreVersiones(t *testing.T) {
	v1 := marcaDePrueba()
	v1.Version = 1
	v2 := v1
	v2.Version = 2
	v2.Accent = "#0F766E"
	v2.Bg = "#ECFDF5"
	casos := []struct {
		kind      Kind
		contenido string
		scopes    []string
	}{
		{KindDocument, docConTema, []string{"headings", "table_headers"}},
		{KindSpreadsheet, hojaConTema, []string{"sheet_header"}},
		{KindPresentation, `{"slides":[{"id":"s1","boxes":[]}]}`, []string{"slide_theme"}},
	}
	for _, c := range casos {
		t.Run(string(c.kind), func(t *testing.T) {
			// Un documento sin marca: la v1 cambia cosas.
			cambios, err := PrevisualizarTema(c.kind, c.contenido, v1.Tema())
			if err != nil || len(cambios) == 0 {
				t.Fatalf("v1 sobre contenido sin marca: %v %v", cambios, err)
			}
			en1, _ := AplicarTema(c.kind, c.contenido, v1.Tema())
			if otra, _ := PrevisualizarTema(c.kind, en1, v1.Tema()); len(otra) != 0 {
				t.Fatalf("la v1 ya aplicada no debe proponer cambios: %v", otra)
			}
			// v1 -> v2: el diff nombra cada ámbito y trae de/a.
			cambios, err = PrevisualizarTema(c.kind, en1, v2.Tema())
			if err != nil {
				t.Fatal(err)
			}
			if len(cambios) != len(c.scopes) {
				t.Fatalf("ámbitos %+v, quería %v", cambios, c.scopes)
			}
			for i, ch := range cambios {
				if ch.Scope != c.scopes[i] || ch.Count == 0 || ch.To == "" || ch.From == ch.To {
					t.Errorf("cambio sin sentido: %+v", ch)
				}
			}
			// La previsualización es EXACTAMENTE lo que se aplica; aplicarla no toca el contenido
			// y repetirla no cambia nada.
			en2, err := AplicarTema(c.kind, en1, v2.Tema())
			if err != nil || en2 == en1 {
				t.Fatalf("aplicar v2: %v", err)
			}
			if otra, _ := PrevisualizarTema(c.kind, en2, v2.Tema()); len(otra) != 0 {
				t.Errorf("v2 no es idempotente: %v", otra)
			}
			if c.kind == KindDocument && textoPlano(t, en2) != textoPlano(t, c.contenido) {
				t.Error("la migración cambió el texto")
			}
			// Reversible: volver a la v1 deja el mismo contenido que antes de migrar.
			if vuelta, _ := AplicarTema(c.kind, en2, v1.Tema()); vuelta != en1 {
				t.Errorf("volver a la v1 no reproduce el estado anterior:\n%s\n%s", vuelta, en1)
			}
		})
	}
}
