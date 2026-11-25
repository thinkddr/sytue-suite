// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestCapturasEsteticas deja con SYTUE_CAPTURAS «Mis bases» vacía, la lista con dos y
// una base abierta en las ocho combinaciones de la auditoría del 2026-11-24 (ver
// `montaje.Capturar`).
func TestCapturasEsteticas(t *testing.T) {
	_, _, vacia := montar(t, map[string]string{"GET /bases": "[]"})
	montaje.Capturar(t, vacia, "bases-vacia", iconos.Bases)

	_, _, lista := montar(t, map[string]string{"GET /bases": "[" + baseClientes + "," + basePedidos + "]"})
	montaje.Capturar(t, lista, "bases-lista", iconos.Bases)

	_, base := abrir(t, nuevoFalso(12))
	montaje.Capturar(t, base, "bases-base", iconos.Bases)
}
