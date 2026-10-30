// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/documentos"
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
func (s *Suite) AbrirDriveEnSistema() error {
	return s.AbrirDriveEnSistemaCon(context.Background())
}

// AbrirDriveEnSistemaCon usa el único proveedor de One. Un contexto con intención
// queda ligado a la cuenta que se mostró; no adopta otra durante las esperas.
func (s *Suite) AbrirDriveEnSistemaCon(ctx context.Context) error {
	identidad, err := s.prepararAperturaDrive(ctx)
	if err != nil {
		return err
	}
	if auth.RequestAuthorizationIntent(ctx) != nil && !s.DriveEnSistema() {
		return documentos.ErrNoDisponible // La preferencia desactivada no se cambia.
	}
	return abrirDriveEnSistema(ctx, s, identidad)
}

func (s *Suite) prepararAperturaDrive(ctx context.Context) (auth.IdentitySnapshot, error) {
	if err := ctx.Err(); err != nil {
		return auth.IdentitySnapshot{}, err
	}
	if s == nil || s.Sesion == nil {
		return auth.IdentitySnapshot{}, auth.ErrLoginRequired
	}
	intent := auth.RequestAuthorizationIntent(ctx)
	if intent == nil {
		return s.Sesion.SnapshotIdentity(), nil
	}
	token, scope, err := s.Sesion.AccessTokenScoped(ctx)
	if err != nil {
		return auth.IdentitySnapshot{}, err
	}
	if !intent.Allows(scope, "Bearer "+token) {
		return auth.IdentitySnapshot{}, auth.ErrAuthorizationChanged
	}
	return intent.Identity(), nil
}

func (s *Suite) comprobarAperturaDrive(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if intent := auth.RequestAuthorizationIntent(ctx); intent != nil {
		if !intent.Current() {
			return auth.ErrAuthorizationChanged
		}
		if !s.DriveEnSistema() {
			return documentos.ErrNoDisponible
		}
	}
	return nil
}

// aplicarAperturaDrive es la última guarda antes de entregar la apertura al SO.
func (s *Suite) aplicarAperturaDrive(ctx context.Context, abrir func() error) error {
	if err := s.comprobarAperturaDrive(ctx); err != nil {
		return err
	}
	return abrir()
}
