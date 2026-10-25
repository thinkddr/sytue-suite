// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !darwin && !ios

package sesion

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/documentos"
	"git.sytue.eu/sytue/one/apps/nucleo/nube"
)

func abrirDriveEnSistema(s *Suite) error {
	if !s.Estado().Dentro() || !s.DriveEnSistema() {
		return documentos.ErrNoDisponible
	}
	if runtime.GOOS == "android" && s.app.Metadata().Custom["esquema"] != "sytue-drive" {
		return api.AbrirEn(s.app, s.Apex(), "drive", s.Sesion.Organizacion(), "/?open-system=1")
	}
	ctx, parar := context.WithTimeout(context.Background(), time.Minute)
	defer parar()
	if runtime.GOOS == "windows" {
		i := s.Sesion.SnapshotIdentity()
		if _, err := s.autorizacionDriveSistema(i).AccessToken(ctx); err != nil {
			return err
		}
		dir, err := nube.CarpetaCon(ctx, nube.Cuenta{Emisor: i.Issuer, Sub: i.Subject, Org: i.Organization})
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		return exec.CommandContext(context.Background(), "explorer.exe", dir).Start() //nolint:gosec // G204: ruta generada y ligada a la cuenta por nube.Carpeta.
	}
	// La app puede llegar aquí por un enlace del Hub durante el arranque en
	// frío, antes de que termine el controlador automático. La misma operación
	// serializada e idempotente prepara la raíz antes de pedir que se muestre.
	if err := s.conectarDriveSistema(ctx, estadoDriveSistema{s.Sesion.SnapshotIdentity(), true}); err != nil {
		return err
	}
	return documentos.Abrir(ctx)
}
