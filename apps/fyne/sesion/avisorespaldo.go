// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/anfitrion"
)

// AvisoDeRespaldo enseña un aviso con el `SendNotification` de Fyne cuando el del sistema
// (`escritorio.Avisar`) no ha podido. La puerta vive en `anfitrion/avisos.go` (C-04), que
// explica por qué en Windows no sale.
func AvisoDeRespaldo(a fyne.App, titulo, cuerpo string) { anfitrion.AvisoDeRespaldo(a, titulo, cuerpo) }

func avisoDeRespaldo(sistema string, a fyne.App, titulo, cuerpo string) {
	anfitrion.AvisoDeRespaldoEn(sistema, a, titulo, cuerpo)
}
