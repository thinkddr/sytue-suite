// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// AgendaDeLaSuite es el `/api/hoy` del BFF de la web (`pages/api/hoy.ts`) hecho desde el
// cliente: aquí no hay BFF, y el token ya lo tiene la sesión transversal.
type AgendaDeLaSuite struct {
	Calendario *api.Calendars
	Correo     *api.Mail
}

var _ Agendable = AgendaDeLaSuite{}

// Hoy es api.Calendars.Hoy: la misma agenda que publica el widget del Hub.
func (a AgendaDeLaSuite) Hoy(ctx context.Context, desde, hasta time.Time) ([]EventoDeHoy, error) {
	return a.Calendario.Hoy(ctx, desde, hasta)
}

// SinLeer son los no leídos de la bandeja de entrada o, si no hay buzón con ese rol, los
// de todos: el `sinLeer` del BFF.
func (a AgendaDeLaSuite) SinLeer(ctx context.Context) (int, error) {
	buzones, err := a.Correo.Mailboxes(ctx)
	if err != nil {
		return 0, err
	}
	return SinLeerDe(buzones), nil
}

// SinLeerDe es la cuenta de SinLeer sobre una lista ya pedida.
func SinLeerDe(buzones []api.Mailbox) int {
	return api.MailInboxUnread(buzones)
}
