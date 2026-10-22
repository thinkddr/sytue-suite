// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import "testing"

// TestCadaInterruptorDeSerieApagaAlgo: un interruptor de Admin solo sirve si alguna
// plantilla que un producto usa lo comprueba (`proveedorDePlantilla`). Box y Zoom
// estuvieron en la lista sin que nada los mirase: Admin los apagaba y no pasaba nada.
//
// VISTO EN ROJO (2026-10-21): devolviendo "zoom" a `integracionesDeSerie`, el test lo
// nombra. Deshecho.
func TestCadaInterruptorDeSerieApagaAlgo(t *testing.T) {
	plantillas := make([]string, 0, 7+len(personalStorageProviders)+len(mailProviderTemplates))
	plantillas = append(plantillas,
		tmplSlackRooms, tmplTeamsRooms, tmplGoogleChat, tmplGoogleMeet,
		tmplSlackChat, tmplTeamsChat, tmplGoogleConversations,
	)
	for _, k := range personalStorageProviders {
		plantillas = append(plantillas, k)
	}
	for _, k := range mailProviderTemplates {
		plantillas = append(plantillas, k)
	}
	comprobados := map[string]bool{}
	for _, k := range plantillas {
		comprobados[proveedorDePlantilla(k)] = true
	}
	for _, p := range integracionesDeSerie {
		if !comprobados[p] {
			t.Errorf("el interruptor %q no lo comprueba ninguna plantilla: no apaga nada", p)
		}
	}
}
