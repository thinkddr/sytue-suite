// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package sistema

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

var (
	leerAccesoAperturaArchivos    = ajustes.LeerAccesoDrive
	hayCredencialAperturaArchivos = hayCredencialArchivosDe
	abrirDominioAperturaArchivos  = abrirDominioArchivos
)

var errAperturaArchivosAjena = errors.New("sistema: el dominio de archivos no pertenece a la autorización mostrada")

// AperturaArchivos congela el dominio de una identidad y revisión concretas.
// No conserva claves y no resuelve el dominio global al aplicar la apertura.
type AperturaArchivos struct {
	dominio string
	acceso  ajustes.AccesoDrive
	abrir   func(string, bool) error
}

// PrepararAperturaArchivos consulta el propietario real del proveedor. Una marca
// ausente, desalineada con el llavero o de otra cuenta nunca produce un destino.
func PrepararAperturaArchivos(ctx context.Context, identidad string) (*AperturaArchivos, error) {
	a, err := accesoAperturaArchivos(ctx, identidad)
	if err != nil {
		return nil, err
	}
	dominio, err := identificadorDominioArchivos(a.Identidad, a.Revision)
	if err != nil {
		return nil, err
	}
	return &AperturaArchivos{dominio: dominio, acceso: a, abrir: abrirDominioAperturaArchivos}, nil
}

func accesoAperturaArchivos(ctx context.Context, identidad string) (ajustes.AccesoDrive, error) {
	if err := ctx.Err(); err != nil {
		return ajustes.AccesoDrive{}, err
	}
	a, err := leerAccesoAperturaArchivos()
	if err != nil {
		return ajustes.AccesoDrive{}, err
	}
	if err := ctx.Err(); err != nil {
		return ajustes.AccesoDrive{}, err
	}
	if a.Version != 1 || !a.Activo || a.Identidad != identidad {
		return ajustes.AccesoDrive{}, errAperturaArchivosAjena
	}
	if _, err := identificadorDominioArchivos(a.Identidad, a.Revision); err != nil {
		return ajustes.AccesoDrive{}, err
	}
	hay, err := hayCredencialAperturaArchivos(identidad)
	if err != nil {
		return ajustes.AccesoDrive{}, err
	}
	if err := ctx.Err(); err != nil {
		return ajustes.AccesoDrive{}, err
	}
	if !hay {
		return ajustes.AccesoDrive{}, errAperturaArchivosAjena
	}
	return a, nil
}

// Abrir revalida después de resolver dueño/llavero y entrega al SO sólo el ID
// congelado. La guardia permite comprobar la sesión sin depender de auth aquí.
func (a *AperturaArchivos) Abrir(ctx context.Context, validar func(context.Context) error) error {
	if a == nil || a.abrir == nil || validar == nil {
		return ErrArchivosNoDisponible
	}
	actual, err := accesoAperturaArchivos(ctx, a.acceso.Identidad)
	if err != nil {
		return err
	}
	if actual != a.acceso {
		return errAperturaArchivosAjena
	}
	if err := validar(ctx); err != nil {
		return err
	}
	return a.abrir(a.dominio, true)
}
