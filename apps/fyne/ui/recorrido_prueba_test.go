// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestLaBarraDePruebaRestauraElCatalogoComunYElProducto(t *testing.T) {
	idioma, producto := comun.Load(), tema.AppElegida()
	_, restaurar := BarraSuiteDePrueba(traductorPrueba(t), iconos.Sheets)
	restaurar()
	if comun.Load() != idioma || tema.AppElegida() != producto {
		t.Fatal("la barra de pruebas cambió el idioma común o el producto de otra escena")
	}
}
