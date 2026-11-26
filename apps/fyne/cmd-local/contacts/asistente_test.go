// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestElAsistenteDeAjustesLlegaSinRed(t *testing.T) {
	if asistente(nil) != nil {
		t.Fatal("con la IA apagada no hay asistente: «Rellenar» no sale")
	}
	var recibida api.PeticionIA
	a := asistente(func(_ context.Context, q api.PeticionIA) (string, error) {
		recibida = q
		return `{"nombre":"Ada"}`, nil
	})
	texto, err := a.Asistir(t.Context(), api.PeticionIA{Accion: "ficha", Contexto: "Ada, ada@ejemplo.org", Idioma: "es"})
	if err != nil || texto != `{"nombre":"Ada"}` || recibida.Contexto != "Ada, ada@ejemplo.org" || recibida.Idioma != "es" {
		t.Fatal(texto, err, recibida)
	}
}
