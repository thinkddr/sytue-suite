// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"net"

	"git.sytue.eu/sytue/one/apps/fyne/apps/erp"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	localerp "git.sytue.eu/sytue/one/apps/nucleo/local/erp"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// comprobarVIES es «Comprobar en VIES» del ERP de Sytue Office (`erp.Pantalla.ComprobarVIES`):
// pregunta al servicio de la Comisión Europea (`localerp.VIES`) por el permiso PUNTUAL del
// portero de red con el propósito `vies`, que abre el destino solo mientras dura esa
// consulta, la que la persona acaba de pedir. Ninguna configuración lo deja abierto.
func comprobarVIES(l *local.App) func(ctx context.Context, nif, pais string) (erp.ComprobacionVIES, error) {
	return comprobarVIESCon(func() localerp.VIES { return localerp.VIES{HTTP: l.Red.Puntual(red.VIES, "")} })
}

// comprobarVIESCon es comprobarVIES con el cliente de VIES que se le dé: el de la app o,
// en las pruebas, uno contra un VIES de mentira.
func comprobarVIESCon(vies func() localerp.VIES) func(ctx context.Context, nif, pais string) (erp.ComprobacionVIES, error) {
	return func(ctx context.Context, nif, pais string) (erp.ComprobacionVIES, error) {
		r, err := vies().Comprobar(ctx, nif, pais)
		if err != nil {
			return erp.ComprobacionVIES{}, falloVIES(err)
		}
		return erp.ComprobacionVIES{Pais: r.Pais, Numero: r.Numero, Valido: r.Valido, Nombre: r.Nombre, Direccion: r.Direccion}, nil
	}
}

// falloVIES es el fallo que la pantalla dice en su idioma: lo que no es un NIF-IVA, lo
// que el portero no deja salir o no llega (sin red, «Sin conexión») y, lo demás, que VIES
// no responde.
func falloVIES(err error) error {
	var tiempo net.Error
	switch {
	case errors.Is(err, localerp.ErrNoEsNIFIVA):
		return erp.VIESNoEsNIFIVA
	case errors.Is(err, red.ErrDenegado):
		return erp.VIESSinRed
	case errors.Is(err, localerp.ErrVIESNoResponde), errors.Is(err, context.DeadlineExceeded),
		errors.As(err, &tiempo) && tiempo.Timeout():
		return erp.VIESNoResponde
	case sinRed(err):
		return erp.VIESSinRed
	}
	return erp.VIESNoResponde
}

// sinRed: no se pudo ni abrir la conexión (sin red, el nombre no resuelve).
func sinRed(err error) bool {
	var op *net.OpError
	var dns *net.DNSError
	return errors.As(err, &op) || errors.As(err, &dns)
}
