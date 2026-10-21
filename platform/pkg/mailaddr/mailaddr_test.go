// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mailaddr

import "testing"

func TestReservado(t *testing.T) {
	casos := map[string]bool{
		"admin":          true,
		"Admin":          true,
		"no-reply":       true,
		"no.reply":       true,
		"noreply2024":    true,
		"soporte":        true,
		"sytue-id":       true,
		"equipo.sytue":   true,
		"ventas":         true,
		"ana.perez":      false,
		"ana":            false,
		"ventasdelnorte": false,
		"":               false,
		"---":            false,
	}
	for local, quiere := range casos {
		if got := Reservado(local); got != quiere {
			t.Errorf("Reservado(%q) = %v, quiero %v", local, got, quiere)
		}
	}
}

func TestSuplantaSoloEnElDominioRaiz(t *testing.T) {
	if !Suplanta("admin", "sytue.eu", "sytue.eu") {
		t.Fatal("admin@sytue.eu debe suplantar")
	}
	if !Suplanta("admin", "SYTUE.EU", " sytue.eu ") {
		t.Fatal("la comparación de dominios pliega mayúsculas y espacios")
	}
	if Suplanta("admin", "acme.sytue.eu", "sytue.eu") {
		t.Fatal("admin@acme.sytue.eu es asunto de acme, no nuestro")
	}
	if Suplanta("admin", "acme.com", "sytue.eu") {
		t.Fatal("un dominio propio verificado no está sujeto a la lista")
	}
	if Suplanta("ana", "sytue.eu", "sytue.eu") {
		t.Fatal("un nombre corriente en el raíz es válido")
	}
	if Suplanta("admin", "sytue.eu", "") {
		t.Fatal("sin raíz configurada no hay nada que suplantar")
	}
}
