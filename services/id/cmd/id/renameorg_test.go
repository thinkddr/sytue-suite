// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/merge"
)

// El renombrado usa el mismo motor que la fusión, así que hereda su contrato: el
// DSN de cada base se deriva del de Sytue ID cambiando SOLO el nombre de la base.
// Si eso se rompiera, el renombrado escribiría en la base equivocada en vez de
// fallar, que es el peor modo posible.
func TestRenameOrgUsaElMismoContratoDeDSNQueLaFusion(t *testing.T) {
	// Se comprueba a través del paquete merge para que un cambio allí rompa aquí
	// también: son la misma operación sobre las mismas filas.
	if !strings.Contains(strings.Join(merge.Services, ","), "mail") {
		t.Fatal("el renombrado no recorrería la base de correo")
	}
	// Y el destino nunca se construye a mano: si el DSN no es una URL, falla en
	// vez de adivinar.
	if _, err := merge.DSNForDatabase("host=localhost dbname=id", "mail"); err == nil {
		t.Error("un DSN key=value debería rechazarse, no adivinarse")
	}
}
