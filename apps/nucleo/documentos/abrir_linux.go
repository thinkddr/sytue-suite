// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package documentos

import (
	"context"
	"os/exec"
)

var abrirUbicacion = func(ctx context.Context, uri string) error {
	return exec.CommandContext(ctx, "gio", "open", uri).Run()
}

func abrir(ctx context.Context) error {
	cred, err := cargarCredencial()
	if err != nil {
		return err
	}
	if cred.Clave == "" {
		return ErrNoDisponible
	}
	u, err := ubicacion(cred)
	if err != nil {
		return err
	}
	return abrirUbicacion(ctx, u)
}

func prepararApertura(ctx context.Context, dueno Identidad, url string) (_ *Apertura, err error) {
	if err := comprobarDuenoApertura(ctx, dueno); err != nil {
		return nil, err
	}
	soltar, err := excluir(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			soltar()
		}
	}()
	cred, err := cargarCredencial()
	if err != nil {
		return nil, err
	}
	if err := comprobarDuenoApertura(ctx, dueno); err != nil {
		return nil, err
	}
	if cred.Clave == "" || cred.Dueno != dueno || cred.URL != url {
		return nil, errAperturaAjena
	}
	uri, err := ubicacion(cred)
	if err != nil {
		return nil, err
	}
	acceso, err := accesoDeApertura(ctx, dueno)
	if err != nil {
		return nil, err
	}
	return &Apertura{uri: uri, dueno: dueno, acceso: acceso, abrir: abrirUbicacion, soltar: soltar}, nil
}
