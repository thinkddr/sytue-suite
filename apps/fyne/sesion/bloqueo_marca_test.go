// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// TestElBloqueoNoSeQuitaEditandoLosAjustes (NAT-09): con la marca en el almacén
// seguro, quitar `"bloqueo": true` de ajustes.json no apaga el bloqueo; quitarlo
// por su puerta (que verifica) quita las dos cosas.
func TestElBloqueoNoSeQuitaEditandoLosAjustes(t *testing.T) {
	aparte(t)
	if bloqueoPuesto(ajustes.Estado{}) {
		t.Fatal("sin nada puesto no hay bloqueo")
	}
	if err := marcarBloqueo(true); err != nil {
		t.Fatal(err)
	}
	// Alguien edita ajustes.json y deja Bloqueo a false.
	if !bloqueoPuesto(ajustes.Estado{}) {
		t.Fatal("editar los ajustes quitó el bloqueo")
	}
	if err := marcarBloqueo(false); err != nil {
		t.Fatal(err)
	}
	if bloqueoPuesto(ajustes.Estado{}) {
		t.Fatal("quitado por su puerta, no queda bloqueo")
	}
}
