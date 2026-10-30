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
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/documentos"
	"git.sytue.eu/sytue/one/apps/nucleo/nube"
)

func abrirDriveEnSistema(ctx context.Context, s *Suite, i auth.IdentitySnapshot) error {
	if err := s.comprobarAperturaDrive(ctx); err != nil {
		return err
	}
	if !s.Estado().Dentro() || !s.DriveEnSistema() {
		return documentos.ErrNoDisponible
	}
	if runtime.GOOS == "android" && auth.RequestAuthorizationIntent(ctx) != nil {
		// El selector global/handoff Android no conserva un destino por dueño.
		return documentos.ErrNoDisponible
	}
	if runtime.GOOS == "android" && s.app.Metadata().Custom["esquema"] != "sytue-drive" {
		if err := s.comprobarAperturaDrive(ctx); err != nil {
			return err
		}
		return s.aplicarAperturaDrive(ctx, func() error { return api.AbrirEn(s.app, s.Apex(), "drive", i.Organization, "/?open-system=1") })
	}
	ctx, parar := context.WithTimeout(ctx, time.Minute)
	defer parar()
	if runtime.GOOS == "windows" {
		return abrirCarpetaDriveWindows(ctx, s, i)
	}
	// La app puede llegar aquí por un enlace del Hub durante el arranque en
	// frío, antes de que termine el controlador automático. La misma operación
	// serializada e idempotente prepara la raíz antes de pedir que se muestre.
	if auth.RequestAuthorizationIntent(ctx) == nil {
		// El arranque en frío existente conserva su preparación idempotente.
		if err := s.conectarDriveSistema(ctx, estadoDriveSistema{i, true}); err != nil {
			return err
		}
	}
	// El wizard sólo abre el proveedor One ya existente. No activa otra raíz ni
	// vuelve a habilitar un proveedor después de la elección de desactivarlo.
	if err := s.comprobarAperturaDrive(ctx); err != nil {
		return err
	}
	if auth.RequestAuthorizationIntent(ctx) != nil {
		if s.almacen == nil {
			return documentos.ErrNoDisponible
		}
		destino, err := documentos.PrepararApertura(ctx,
			identidadDocumentos(i, s.almacen.Identificador()), s.URL("drive")+"/dav/")
		if err != nil {
			return err
		}
		defer destino.Close()
		return s.aplicarAperturaDrive(ctx, func() error { return destino.Abrir(ctx) })
	}
	return s.aplicarAperturaDrive(ctx, func() error { return documentos.Abrir(ctx) })
}

func abrirCarpetaDriveWindows(ctx context.Context, s *Suite, i auth.IdentitySnapshot) error {
	dir, err := prepararCarpetaDriveWindows(ctx, s, i)
	if err != nil {
		return err
	}
	return s.aplicarAperturaDrive(ctx, func() error { return exec.CommandContext(context.WithoutCancel(ctx), "explorer.exe", dir).Start() }) //nolint:gosec // G204: ruta generada y ligada a la cuenta por nube.Carpeta.
}

func prepararCarpetaDriveWindows(ctx context.Context, s *Suite, i auth.IdentitySnapshot) (string, error) {
	if _, err := s.autorizacionDriveSistema(i).AccessToken(ctx); err != nil {
		return "", err
	}
	dir, err := nube.CarpetaCon(ctx, cuentaNube(i))
	if err != nil {
		return "", err
	}
	if err := s.comprobarAperturaDrive(ctx); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := s.comprobarAperturaDrive(ctx); err != nil {
		return "", err
	}
	return dir, nil
}
