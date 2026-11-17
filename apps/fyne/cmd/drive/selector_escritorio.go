// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !ios

package main

import (
	"errors"
	"log/slog"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// montarSelector pone el selector de ESCRITORIO: el del sistema, que devuelve rutas. Y
// el de carpetas, que en el móvil no hay: la carpeta elegida es una ruta más, que la
// subida recorre entera como si se hubiera soltado en la ventana.
func montarSelector(_ fyne.Window, p *drive.Pantalla) {
	p.ElegirFicheros = func(listo func([]drive.Entrada)) {
		elegir(func(rutas []string) { listo(drive.Rutas(rutas)) })
	}
	p.ElegirCarpeta = func(titulo string, listo func([]drive.Entrada)) {
		elegirCarpeta(titulo, func(ruta string) { listo(drive.Rutas([]string{ruta})) })
	}
}

// elegirCarpeta es [elegir] para una carpeta: el selector bloquea, así que va aparte, y
// lo elegido vuelve a la rutina de la interfaz. Sin selector, no se abre nada.
func elegirCarpeta(titulo string, listo func(string)) {
	go func() {
		ruta, err := ficheros.ElegirCarpeta(titulo)
		if err != nil {
			if !errors.Is(err, ficheros.ErrNoDisponible) {
				slog.Warn("drive: el selector de carpetas ha fallado", "error", err)
			}
			return
		}
		if ruta != "" {
			fyne.Do(func() { listo(ruta) })
		}
	}()
}
