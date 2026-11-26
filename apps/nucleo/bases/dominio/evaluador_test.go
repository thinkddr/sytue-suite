// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominio

import (
	"encoding/json"
	"slices"
	"testing"
)

// El evaluador en memoria da lo mismo que el SQL del servicio con sus vectores.
func TestElEvaluadorCumpleLosVectoresDelServidor(t *testing.T) {
	for _, juego := range VectoresDeVista() {
		for _, caso := range juego.Casos {
			got := []string{}
			for _, r := range FiltrarYOrdenar(juego.Campos, juego.Registros, caso.Config) {
				var s string
				_ = json.Unmarshal(r.Data[juego.Texto], &s)
				got = append(got, s)
			}
			if !slices.Equal(got, caso.Quiere) {
				t.Errorf("%s: %v, quería %v", caso.Nombre, got, caso.Quiere)
			}
		}
	}
}

// Sin campos buscables, una búsqueda no casa nada (nunca «todo»); en blanco no filtra.
func TestBusquedaSinCamposBuscables(t *testing.T) {
	campos := []Campo{{ID: "c", Kind: TipoCasilla}}
	regs := []Registro{{ID: "1", Data: map[string]json.RawMessage{"c": json.RawMessage(`true`)}}}
	if n := len(FiltrarYOrdenar(campos, regs, ConfigVista{Search: "x"})); n != 0 {
		t.Fatal("sin campos buscables casa", n)
	}
	if n := len(FiltrarYOrdenar(campos, regs, ConfigVista{Search: "  "})); n != 1 {
		t.Fatal("una búsqueda en blanco filtra", n)
	}
}
