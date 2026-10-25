// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// DriveEnSistema indica si Drive debe aparecer en el explorador del sistema.
// Está activado hasta que la persona lo desactiva explícitamente en este dispositivo.
func (s *Suite) DriveEnSistema() bool {
	return !ajustes.Leer().DriveSistemaDesactivado
}

// PonerDriveEnSistema guarda la elección y avisa a las integraciones de la sesión.
func (s *Suite) PonerDriveEnSistema(habilitado bool) error {
	if err := ponerDriveSistemaCompartido(habilitado); err != nil {
		return err
	}
	if !habilitado {
		if err := ajustes.PonerAccesoDrive("", false); err != nil {
			return err
		}
	}
	if err := s.Poner(func(a *ajustes.Ajustes) { a.DriveSistemaDesactivado = !habilitado }); err != nil {
		return err
	}
	// La sesión guardada tampoco permite saltarse el bloqueo de esta app ni
	// volver a autorizar después de un cierre local que no pudo persistirse.
	if s.almacen == nil || s.reidentificando.Load() || !s.Estado().Dentro() {
		return ajustes.PonerAccesoDrive("", false)
	}
	return s.almacen.ActualizarAccesoDrive(context.Background())
}

// AbrirDriveEnSistema abre la ubicación de Drive mediante el explorador nativo.
// No muestra perfiles de conexión ni pide una segunda credencial.
func (s *Suite) AbrirDriveEnSistema() error { return abrirDriveEnSistema(s) }
