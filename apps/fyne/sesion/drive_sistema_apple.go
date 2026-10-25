// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin || ios

package sesion

import (
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

func abrirDriveEnSistema(s *Suite) error {
	if !sistema.ArchivosDisponible() {
		// Las otras apps comparten sesión, pero sólo Drive lleva la extensión.
		return api.AbrirEn(s.app, s.Apex(), "drive", s.Sesion.Organizacion(), "/?open-system=1")
	}
	return sistema.AbrirArchivos()
}
