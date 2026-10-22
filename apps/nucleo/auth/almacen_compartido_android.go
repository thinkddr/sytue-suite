// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package auth

import (
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

func cargarCompartida(a *AlmacenSesion) (Tokens, bool, error) {
	habilitada, err := sistema.SesionAndroidHabilitada()
	if err != nil {
		return Tokens{}, true, errors.Join(ErrSesionNoDisponible, err)
	}
	if !habilitada {
		return Tokens{}, false, nil
	}
	texto, err := sistema.CargarSesionAndroid(a.cuenta, a.emisor, a.cliente)
	if err != nil {
		return Tokens{}, true, errors.Join(ErrSesionNoDisponible, err)
	}
	if texto == "" {
		return Tokens{}, true, nil
	}
	t, err := recuperar([]byte(texto))
	if err != nil {
		return Tokens{}, true, errors.Join(ErrSesionNoDisponible, err)
	}
	return t, true, err
}

func guardarCompartida(a *AlmacenSesion, t Tokens) (bool, error) {
	habilitada, err := sistema.SesionAndroidHabilitada()
	if err != nil {
		return true, errors.Join(ErrSesionNoDisponible, err)
	}
	if !habilitada {
		return false, nil
	}
	var datos []byte
	if t.Refresh != "" {
		t.Issuer, t.Cliente = a.emisor, a.cliente
		datos, err = serializar(t)
		if err != nil {
			return true, err
		}
	}
	err = sistema.GuardarSesionAndroid(a.cuenta, a.emisor, a.cliente, string(datos))
	if err != nil {
		err = errors.Join(ErrSesionNoDisponible, err)
	}
	return true, err
}

func migrarCompartida(a *AlmacenSesion, permitirSinEmisor bool) (bool, error) {
	habilitada, err := sistema.SesionAndroidHabilitada()
	if err != nil || !habilitada {
		return habilitada || err != nil, err
	}
	return true, sistema.MigrarSesionAndroid(a.cuenta, a.emisor, a.cliente, permitirSinEmisor)
}
