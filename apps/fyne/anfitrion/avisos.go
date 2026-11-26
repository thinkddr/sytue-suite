// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package anfitrion

import (
	"errors"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
)

// La puerta de los avisos de Fyne (`SendNotification` y `ScheduleNotification`): el ÚNICO
// sitio desde el que se llaman, y `TestSoloSeAbreConElSistemaPorSuPuerta` (sesion) lo
// comprueba. Vivía en `sesion` (avisorespaldo.go); `sesion.AvisoDeRespaldo` delega aquí.
//
// En Windows NO se usan. En el fork de Fyne (`app/app_windows.go`), `SendNotification` y
// `ScheduleNotification` interpolan título y cuerpo con `%q` (comillas dobles) en un
// `.ps1` temporal ejecutado con `Invoke-Expression`, donde PowerShell evalúa `$(…)`. El
// cuerpo de un aviso del Hub es el asunto de un correo, y el título de un recordatorio
// puede venir de un `.ics` ajeno: lo escribe otra persona. El aviso propio de Windows
// (`escritorio/toast.go`, y `escritorio.ProgramarToast` para los programados) sí escapa
// todas las comillas simples y pasa el guion con `-EncodedCommand`
// (`TestElRespaldoDelAvisoNoSaleEnWindows`, NAT-12).

// AvisoDeRespaldo enseña un aviso con el `SendNotification` de Fyne cuando el del
// sistema (`escritorio.Avisar`) no ha podido.
func AvisoDeRespaldo(a fyne.App, titulo, cuerpo string) {
	AvisoDeRespaldoEn(runtime.GOOS, a, titulo, cuerpo)
}

// AvisoDeRespaldoEn es AvisoDeRespaldo como si corriera en `sistema` (las pruebas).
func AvisoDeRespaldoEn(sistema string, a fyne.App, titulo, cuerpo string) {
	if sistema == "windows" || sistema == "ios" || a == nil {
		return
	}
	a.SendNotification(fyne.NewNotification(titulo, cuerpo))
}

// ErrAvisoNoDisponible: en este sistema el aviso programado no va por Fyne.
var ErrAvisoNoDisponible = errors.New("anfitrion: el aviso programado de Fyne no se usa en este sistema")

// ProgramarAviso deja al sistema un aviso para `cuando` con el `ScheduleNotification` de
// Fyne: `UNUserNotificationCenter` en macOS empaquetado e iOS, `AlarmManager` en Android
// (los dos lo entregan con la app cerrada) y un reloj del proceso en Linux. Devuelve el
// identificador con que quitarlo. En Windows, ErrAvisoNoDisponible (ver arriba).
func ProgramarAviso(a fyne.App, titulo, cuerpo string, cuando time.Time) (string, error) {
	if runtime.GOOS == "windows" || a == nil {
		return "", ErrAvisoNoDisponible
	}
	s, err := a.ScheduleNotification(fyne.NewNotification(titulo, cuerpo), cuando)
	if err != nil {
		return "", err
	}
	return s.ID(), nil
}

// CancelarAviso quita un aviso de ProgramarAviso.
func CancelarAviso(a fyne.App, id string) error {
	if runtime.GOOS == "windows" || a == nil {
		return ErrAvisoNoDisponible
	}
	return a.CancelScheduledNotification(id)
}
