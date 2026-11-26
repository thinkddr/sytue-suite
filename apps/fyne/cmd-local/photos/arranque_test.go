// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build edicionlocal

package main

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/local/prueba"
)

// TestRecienInstaladaNoAbreNingunaConexion monta este `main`, recién instalado, en el
// driver de pruebas de Fyne: pinta su pantalla inicial sin intentar ni una conexión de
// red y sin tocar las carpetas de la edición conectada (R-RED, R-CONVIVE; ver
// `local/prueba.Arrancar`). Con la etiqueta de la edición: sin ella `local.Abrir` no
// arranca (D-02).
func TestRecienInstaladaNoAbreNingunaConexion(t *testing.T) {
	prueba.Arrancar(t, "photos", main)
}
