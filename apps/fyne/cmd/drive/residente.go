// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"log/slog"
	"os"
	"runtime"
	"slices"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// Windows necesita un proceso que atienda CFAPI. Drive se distribuye por separado
// del Hub: usa la bandeja y el inicio de sesión ya compartidos, sin depender de él.
func ejecutarDrive(a fyne.App, w fyne.Window, s *sesion.Suite, t *i18n.Translator) {
	if runtime.GOOS != "windows" {
		w.ShowAndRun()
		return
	}
	mostrar := fyne.NewMenuItem("Sytue Drive", func() { w.Show(); w.RequestFocus() })
	habilitar := fyne.NewMenuItem(t.T("shell.mountDrive", nil), nil)
	menu := fyne.NewMenu("Sytue Drive", mostrar, habilitar)
	habilitar.Action = func() {
		if err := s.PonerDriveEnSistema(!s.DriveEnSistema()); err != nil {
			slog.Warn("drive: no se ha podido cambiar la integración con Windows", "error", err)
		}
	}
	residente := sesion.Residente(a, w, menu)
	s.AlCambiar(func(sesion.Estado) {
		habilitar.Checked = s.DriveEnSistema()
		menu.Refresh()
		if residente {
			if habilitar.Checked {
				w.SetCloseIntercept(w.Hide)
			} else {
				w.SetCloseIntercept(nil)
			}
		}
	})
	habilitar.Checked = s.DriveEnSistema()
	if residente && slices.Contains(os.Args[1:], escritorio.ArgBandeja) {
		if !s.Estado().Dentro() || !s.DriveEnSistema() {
			a.Quit()
			return
		}
		a.Run()
		return
	}
	w.ShowAndRun()
}
