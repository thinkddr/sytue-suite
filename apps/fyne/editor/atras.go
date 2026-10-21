// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// EngancharAtras conecta el retorno común de Docus, Calcus y Diapos después de montar
// su raíz en la ventana: documento → portada, papelera → portada, portada → sistema.
// El cierre es el MISMO de la cinta: conserva el guardado de lo que quedara pendiente.
func EngancharAtras(raiz fyne.CanvasObject, portada *Portada, abierto func() bool, volver func()) {
	a := fyne.CurrentApp()
	if a == nil {
		return
	}
	c := a.Driver().CanvasForObject(raiz)
	if c == nil {
		return
	}
	ui.AlAtras(c, func() bool {
		if abierto() {
			volver()
			return true
		}
		if portada.Papelera() != nil {
			portada.CerrarPapelera()
			return true
		}
		return false
	})
}
