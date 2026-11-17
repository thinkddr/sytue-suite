// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// rutaVectoresCalcus son los vectores compartidos de Calcus que escribe la web
// (apps/frontend/packages/office/test/vectores/calcus.ts).
const rutaVectoresCalcus = "../../../frontend/packages/office/test/vectores/calcus.json"

// De punta a punta con la web: `propiaHoja` es lo que guarda el lienzo de la web
// (`desplazarHojas` de refshift.ts) al insertar o borrar filas y columnas en una hoja
// protegida con esos permisos, con sus fórmulas bloqueadas, su rango editable y su tabla
// dinámica, y otra hoja protegida que la cita. El servidor lo acepta de quien no es el
// propietario, y no a ciegas: con la suma sin reescribir, no.
//
// VISTO EN ROJO sin proteccion_propia.go: los casos que reescriben fórmulas de la propia
// hoja daban ErrProtegido.
func TestVectoresDeLaWebEnLaPropiaHoja(t *testing.T) {
	b, err := os.ReadFile(rutaVectoresCalcus)
	if err != nil {
		t.Fatalf("no se leen los vectores de la web: %v", err)
	}
	var v struct {
		PropiaHoja struct {
			Hojas []any
			Casos []struct {
				Hoja int
				D    map[string]any
				Sale []any
			}
		} `json:"propiaHoja"` //nolint:tagliatelle // el nombre lo fija la web
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("los vectores no se entienden: %v", err)
	}
	if len(v.PropiaHoja.Casos) == 0 {
		t.Fatal("calcus.json no trae la sección «propiaHoja»")
	}
	libro := func(hojas []any, activa int) mj { return mj{"active": activa, "sheets": hojas} }
	sumaAntes := celdasVectorT(v.PropiaHoja.Hojas, 1)["A10"]
	reescritas := 0
	for _, c := range v.PropiaHoja.Casos {
		s := nuevaSimP(t)
		nuevas := map[string]string{AmbitoHoja("Datos"): "d", AmbitoHoja("Resumen"): "r"}
		if err := s.escribir(libro(v.PropiaHoja.Hojas, c.Hoja), false, Proteccion{Nuevas: nuevas}); err != nil {
			t.Fatalf("proteger: %v", err)
		}
		if err := s.clonar(t).escribir(libro(c.Sale, c.Hoja), false, Proteccion{Quien: luis}); err != nil {
			t.Errorf("%v: el servidor rechaza lo que guarda la web: %v", c.D, err)
		}
		// La suma de «Datos», donde la deja el desplazamiento, sin reescribir.
		for ref, val := range celdasVectorT(c.Sale, 1) {
			if texto, _ := val.(string); !strings.Contains(texto, "SUM(") || val == sumaAntes {
				continue
			}
			reescritas++
			celdasVectorT(c.Sale, 1)[ref] = sumaAntes
			if err := s.clonar(t).escribir(libro(c.Sale, c.Hoja), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
				t.Errorf("%v: la suma sin reescribir en %s: quiero ErrProtegido y es %v", c.D, ref, err)
			}
			celdasVectorT(c.Sale, 1)[ref] = val
		}
	}
	if reescritas == 0 {
		t.Fatal("ningún vector reescribe la suma de la propia hoja")
	}
}

// celdasVectorT son las celdas de la hoja `i` de una lista de hojas leída del JSON.
func celdasVectorT(hojas []any, i int) map[string]any {
	h, _ := hojas[i].(map[string]any)
	c, _ := h["cells"].(map[string]any)
	return c
}
