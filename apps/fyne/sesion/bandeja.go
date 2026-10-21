// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// Residente deja la aplicación en la bandeja del sistema —la barra de menús en macOS, el
// área de notificación en Windows, el StatusNotifierItem en Linux— con `menu`, y hace que
// cerrar la ventana la ESCONDA en vez de salir. Así sigue viva con la ventana cerrada, y
// con ella su flujo del Hub y sus avisos al sistema: es lo que en escritorio hace el
// papel del «push con la app cerrada». Para salir de verdad está el «Salir» que Fyne
// añade él solo al final del menú.
//
// Dice si lo ha hecho. No lo hace en el móvil, donde no hay bandeja, ni en un Linux sin
// nadie que pinte la bandeja: ahí esconder la ventana sería perder la app, que seguiría
// corriendo sin forma de volver a ella.
func Residente(a fyne.App, w fyne.Window, menu *fyne.Menu) bool {
	bandeja, ok := a.(desktop.App)
	if !ok || a.Driver().Device().IsMobile() || !escritorio.HayBandeja() {
		return false
	}
	bandeja.SetSystemTrayMenu(menu)
	w.SetCloseIntercept(w.Hide)
	return true
}
