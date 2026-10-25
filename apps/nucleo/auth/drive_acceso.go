// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// IdentidadDrive es un contexto opaco estable. Cambiar un refresh token no cambia
// este hash; un emisor, cliente, persona u organización diferentes sí lo hacen.
func (i IdentitySnapshot) IdentidadDrive() string {
	if i.Issuer == "" || i.Client == "" || i.Subject == "" || i.Organization == "" {
		return ""
	}
	b, _ := json.Marshal([]string{strings.TrimRight(i.Issuer, "/"), i.Client, i.Subject, i.Organization})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (a *AlmacenSesion) publicarAccesoDrive(t Tokens) error {
	identidad := (IdentitySnapshot{a.emisor, a.cliente, t.Subject, organizacionDe(t)}).IdentidadDrive()
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
