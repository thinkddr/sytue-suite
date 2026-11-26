// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// IdentidadDrive, IdentidadSesion y su ContextoSesion son métodos de IdentitySnapshot
// (`nucleo/autorizacion`).

func (a *AlmacenSesion) publicarAccesoDrive(t Tokens) error {
	identidad := (IdentitySnapshot{Issuer: a.emisor, Client: a.cliente, Subject: t.Subject, Organization: organizacionDe(t)}).IdentidadDrive()
	return ajustes.PonerAccesoDrive(identidad, t.Refresh != "" && identidad != "" && !ajustes.Leer().DriveSistemaDesactivado)
}

// ActualizarAccesoDrive publica sólo la identidad que sigue guardada. Mantiene
// el mismo cerrojo que login, refresh y logout; la memoria de una app que aún no
// recibió el cierre de otra nunca puede volver a autorizar File Provider.
//
//nolint:contextcheck // Los callbacks del llavero tienen plazo propio y no reciben contexto.
func (a *AlmacenSesion) ActualizarAccesoDrive(ctx context.Context) error {
	soltar, err := ajustes.CerrojoSesion(ctx)
	if err != nil {
		return errors.Join(err, ajustes.PonerAccesoDrive("", false))
	}
	defer soltar()
	t, err := a.Cargar()
	if err != nil {
		return err
	}
	return a.publicarAccesoDrive(t)
}
