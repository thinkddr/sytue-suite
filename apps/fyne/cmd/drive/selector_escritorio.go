// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !ios

package main

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
)

// montarSelector pone el selector de ESCRITORIO: el del sistema, que devuelve rutas.
func montarSelector(_ fyne.Window, p *drive.Pantalla) {
	p.ElegirFicheros = func(listo func([]drive.Entrada)) {
		elegir(func(rutas []string) { listo(drive.Rutas(rutas)) })
	}
}
