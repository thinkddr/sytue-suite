// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sesion

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/documentos"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

func ponerDriveSistemaCompartido(habilitado bool) error {
	activa, err := sistema.SesionAndroidHabilitada()
	if err != nil {
		return err
	}
	if !activa {
		// Mientras el catálogo siga pendiente de validación en dispositivos,
		// sólo Drive puede cambiar su proveedor local. Hub no finge haberlo hecho.
		a := fyne.CurrentApp()
		if a == nil || a.Metadata().Custom["esquema"] != "sytue-drive" {
			return documentos.ErrNoDisponible
		}
		return nil
	}
	return sistema.PonerDriveSistemaAndroid(habilitado)
}
