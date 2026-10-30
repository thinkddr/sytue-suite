// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin || ios

package sesion

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

func abrirDriveEnSistema(ctx context.Context, s *Suite, i auth.IdentitySnapshot) error {
	if err := s.comprobarAperturaDrive(ctx); err != nil {
		return err
	}
	if !sistema.ArchivosDisponible() {
		if auth.RequestAuthorizationIntent(ctx) != nil {
			return sistema.ErrArchivosNoDisponible
		}
		// Las otras apps comparten sesión, pero sólo Drive lleva la extensión.
		if err := s.comprobarAperturaDrive(ctx); err != nil {
			return err
		}
		return s.aplicarAperturaDrive(ctx, func() error { return api.AbrirEn(s.app, s.Apex(), "drive", i.Organization, "/?open-system=1") })
	}
	if err := s.comprobarAperturaDrive(ctx); err != nil {
		return err
	}
	if auth.RequestAuthorizationIntent(ctx) != nil {
		destino, err := sistema.PrepararAperturaArchivos(ctx, i.IdentidadDrive())
		if err != nil {
			return err
		}
		return destino.Abrir(ctx, s.comprobarAperturaDrive)
	}
	return s.aplicarAperturaDrive(ctx, sistema.AbrirArchivos)
}
