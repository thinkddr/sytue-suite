// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La integración está encendida al entrar. Este control sólo permite retirarla o
// recuperarla: el dominio, la dirección y la credencial los obtiene la propia app.
func configurarSistema(s *sesion.Suite, tr *i18n.Translator, w fyne.Window, abrirSistema func() error) {
	control := ui.NuevoConmutador(tr.T("nav.connectDevices", nil), nil)
	control.Encendido = s.DriveEnSistema()
	errorTexto := ui.NuevoParrafo("", tema.TextoSM, ui.Normal, "danger-text", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	errorTexto.Hide()
	d := ui.NuevoDialogo("Sytue Drive", container.New(ui.Columna{Hueco: tema.Espacio(3)}, control, errorTexto))
	d.Talla = ui.DialogoSM
	abrir := ui.NuevoBoton(tr.T("files.open", i18n.Vars{"name": "Sytue Drive"}), func() {
		go func() {
			if err := abrirSistema(); err != nil {
				slog.Warn("drive: no se pudo abrir la integración del sistema", "error", err)
				fyne.Do(func() { errorTexto.Texto = tr.T("error.connection", nil); errorTexto.Show(); errorTexto.Refresh() })
			}
		}()
	})
	abrir.Inerte = !s.DriveEnSistema()
	guardando := false // Sólo lo toca el hilo de la interfaz.
	control.AlCambiar = func(encendido bool) {
		if guardando {
			return
		}
		guardando = true
		control.Disable()
		abrir.Inerte = true
		abrir.Refresh()
		go func() {
			err := s.PonerDriveEnSistema(encendido)
			actual := s.DriveEnSistema()
			fyne.Do(func() {
				guardando = false
				control.Encendido = actual
				control.Enable()
				abrir.Inerte = !actual
				abrir.Refresh()
				if err != nil {
					errorTexto.Texto = tr.T("error.connection", nil)
					errorTexto.Show()
					errorTexto.Refresh()
					slog.Warn("drive: no se pudo guardar la integración del sistema", "error", err)
				} else {
					errorTexto.Hide()
				}
			})
		}()
	}
	d.Pie = abrir
	d.Mostrar(w.Canvas())
}
