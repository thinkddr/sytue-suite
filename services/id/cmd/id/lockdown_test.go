// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import "testing"

// El interruptor se mueve durante una emergencia y a mano: un argumento mal
// leído aquí es un corte que no se levanta o un motivo que se pierde.
func TestLockdownFlags(t *testing.T) {
	motivo, actor, err := lockdownFlags([]string{"on", "--reason", "sesión robada", "--actor", "ddr"})
	if err != nil {
		t.Fatalf("lockdownFlags: %v", err)
	}
	if motivo != "sesión robada" || actor != "ddr" {
		t.Fatalf("motivo=%q actor=%q", motivo, actor)
	}

	if _, _, err := lockdownFlags([]string{"off", "--reason"}); err == nil {
		t.Fatal("un --reason sin valor debe fallar, no quedarse vacío en silencio")
	}
	if _, _, err := lockdownFlags([]string{"on", "--forzar"}); err == nil {
		t.Fatal("un argumento desconocido debe fallar: puede ser un --reason mal escrito")
	}
	// `status` no lleva flags y tampoco debe inventárselos.
	motivo, actor, err = lockdownFlags([]string{"status"})
	if err != nil || motivo != "" || actor != "" {
		t.Fatalf("status: motivo=%q actor=%q err=%v", motivo, actor, err)
	}
}
