// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"runtime"

	"fyne.io/fyne/v2"
)

// AvisoDeRespaldo enseña un aviso con el `SendNotification` de Fyne cuando el del sistema
// (`escritorio.Avisar`) no ha podido. Es el ÚNICO sitio desde el que se llama, y
// `TestSoloSeAbreConElSistemaPorSuPuerta` lo comprueba.
//
// En Windows NO lo enseña. En `v2.8.1-sytue.16` del fork de Fyne (`app/app_windows.go`),
// `SendNotification` y `ScheduleNotification` interpolan título y cuerpo con `%q` (comillas
// dobles) en un `.ps1` temporal ejecutado con `Invoke-Expression`, donde PowerShell evalúa
// `$(…)`. El cuerpo de un aviso del Hub es el asunto de un correo o el principio de un
// mensaje: lo escribe otra persona. El aviso propio de Windows (`escritorio/toast.go`) sí
// escapa todas las comillas simples (recta y tipográficas) y pasa el guion con
// `-EncodedCommand` sin ficheros temporales ni `Invoke-Expression`; si ese falla, el aviso
// queda en la campana de la suite (`TestElRespaldoDelAvisoNoSaleEnWindows`, NAT-12).
func AvisoDeRespaldo(a fyne.App, titulo, cuerpo string) {
	avisoDeRespaldo(runtime.GOOS, a, titulo, cuerpo)
}

func avisoDeRespaldo(sistema string, a fyne.App, titulo, cuerpo string) {
	if sistema == "windows" || sistema == "ios" || a == nil {
		return
	}
	a.SendNotification(fyne.NewNotification(titulo, cuerpo))
}
