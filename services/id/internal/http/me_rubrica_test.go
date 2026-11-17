// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"math"
	"testing"
)

// TestValidarRubrica fija qué se acepta como firma que recordar: trazos dentro
// del lienzo de CertiPDF y con algo dibujado; nada fuera de los límites.
func TestValidarRubrica(t *testing.T) {
	buena := [][][2]float64{{{10, 10}, {40, 30}, {80, 20}}, {{100, 50}}}
	norm, err := validarRubrica(buena)
	if err != nil {
		t.Fatalf("una firma normal se rechazó: %v", err)
	}
	if norm != "[[[10,10],[40,30],[80,20]],[[100,50]]]" {
		t.Errorf("JSON canónico = %s", norm)
	}

	largo := make([][2]float64, rubricaMaxPuntosTrazo+1)
	muchos := make([][][2]float64, rubricaMaxTrazos+1)
	for i := range muchos {
		muchos[i] = [][2]float64{{1, 1}, {2, 2}}
	}
	casos := map[string][][][2]float64{
		"vacía":              nil,
		"solo puntos":        {{{10, 10}}, {{20, 20}}},
		"trazo vacío":        {{{1, 1}, {2, 2}}, {}},
		"fuera por la x":     {{{1, 1}, {481, 2}}},
		"fuera por la y":     {{{1, 1}, {2, 149}}},
		"negativa":           {{{-1, 1}, {2, 2}}},
		"NaN":                {{{math.NaN(), 1}, {2, 2}}},
		"trazo interminable": {largo},
		"demasiados trazos":  muchos,
	}
	for nombre, trazos := range casos {
		if _, err := validarRubrica(trazos); err == nil {
			t.Errorf("%s: se aceptó y no debía", nombre)
		}
	}
}
