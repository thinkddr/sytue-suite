// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import "context"

// Lo que Sytue Office hace al borrar en Calendar (§5.6): compactar el almacén en el acto,
// en segundo plano, para que lo borrado se vaya del disco sin esperar a cerrar la app
// (borrar varios seguidos se junta en una compactación).

// DeleteEvent borra el evento y pide compactar. En un calendario de una cuenta, además,
// se sincroniza ya: la cancelación a los invitados sale con la subida (invitar.go).
func (c *conCumpleanos) DeleteEvent(ctx context.Context, id string) error {
	cal := ""
	if c.cuentas != nil {
		if ev, err := c.Repository.GetEvent(ctx, id); err == nil {
			cal = ev.CalendarID
		}
	}
	err := c.Repository.DeleteEvent(ctx, id)
	if err == nil && c.rec != nil {
		c.rec.CompactarYa(c.RefreshPrivateData) //nolint:contextcheck // compacta en segundo plano y vive más que el borrado que la pidió
	}
	if err == nil {
		c.cuentas.trasEscribir(cal)
	}
	return err
}

// DeleteCalendar borra el calendario y pide compactar.
func (c *conCumpleanos) DeleteCalendar(ctx context.Context, id string) error {
	err := c.Repository.DeleteCalendar(ctx, id)
	if err == nil && c.rec != nil {
		c.rec.CompactarYa(c.RefreshPrivateData) //nolint:contextcheck // compacta en segundo plano y vive más que el borrado que la pidió
	}
	return err
}
