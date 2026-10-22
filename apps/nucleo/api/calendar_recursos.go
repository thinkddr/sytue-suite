// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// Salas y recursos reservables de la organización (`/api/v1/resources` de Sytue
// Calendar, patrón resource-calendar). Listar con disponibilidad y reservar: cualquiera
// de la organización. Dar de alta: solo `owner`/`admin`; editar y dar de baja: quien lo
// gestiona o un admin. La web habla con los mismos endpoints por su BFF
// (apps/frontend/apps/calendar/src/pages/api/resources.ts).

// Las dos clases de recurso (domain.ResourceKind).
const (
	RecursoSala   = "room"
	RecursoEquipo = "equipment"
)

// Los tres estados de una reserva, como los devuelve BookResource.
const (
	ReservaAceptada  = "accepted"
	ReservaRechazada = "declined"
	ReservaPendiente = "pending"
)

// BusyPeriod es un tramo OCUPADO de un recurso.
type BusyPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Resource es una sala o un equipo. `Busy` solo viene cuando se pide con franja.
type Resource struct {
	ID         string       `json:"id"`
	CalendarID string       `json:"calendar_id"`
	Kind       string       `json:"kind"`
	Name       string       `json:"name"`
	Capacity   int          `json:"capacity"`
	Location   string       `json:"location,omitempty"`
	Email      string       `json:"email,omitempty"`
	AutoAccept bool         `json:"auto_accept"`
	Managers   []string     `json:"managers"`
	Active     bool         `json:"active"`
	Busy       []BusyPeriod `json:"busy,omitempty"`
}

// Ocupado es `isBusy` de ResourcePicker.tsx: algún tramo ocupado cruza [desde, hasta).
func (r Resource) Ocupado(desde, hasta time.Time) bool {
	for _, p := range r.Busy {
		if desde.Before(p.End) && p.Start.Before(hasta) {
			return true
		}
	}
	return false
}

// Resources lista los recursos activos. Con una franja (las dos horas no nulas) cada uno
// trae sus tramos ocupados en ella; sin franja, solo el catálogo.
func (c *Calendars) Resources(ctx context.Context, desde, hasta time.Time) ([]Resource, error) {
	var q url.Values
	if !desde.IsZero() && !hasta.IsZero() {
		q = url.Values{"start": {desde.UTC().Format(time.RFC3339)}, "end": {hasta.UTC().Format(time.RFC3339)}}
	}
	r, err := JSON[struct {
		Resources []Resource `json:"resources"`
	}](ctx, c.c, Request{Path: "/resources", Query: q})
	return r.Resources, err
}

// ResourceInput es el alta y la edición de un recurso. La clase solo cuenta en el alta:
// no se puede cambiar después.
type ResourceInput struct {
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name"`
	Capacity   int    `json:"capacity"`
	Location   string `json:"location"`
	Email      string `json:"email"`
	AutoAccept bool   `json:"auto_accept"`
}

// CreateResource da de alta un recurso.
func (c *Calendars) CreateResource(ctx context.Context, in ResourceInput) (Resource, error) {
	return PostJSON[Resource](ctx, c.c, "/resources", in)
}

// UpdateResource cambia un recurso (PATCH: nombre, aforo, ubicación, correo y si acepta
// solo).
func (c *Calendars) UpdateResource(ctx context.Context, id string, in ResourceInput) (Resource, error) {
	in.Kind = ""
	datos, err := json.Marshal(in)
	if err != nil {
		return Resource{}, err
	}
	return JSON[Resource](ctx, c.c, Request{
		Method: http.MethodPatch, Path: "/resources/" + url.PathEscape(id), Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// DeleteResource da de baja un recurso.
func (c *Calendars) DeleteResource(ctx context.Context, id string) error {
	resp, err := c.c.Do(ctx, Request{Method: http.MethodDelete, Path: "/resources/" + url.PathEscape(id)})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Booking es la reserva de un recurso para un evento, ligada por el UID del evento.
type Booking struct {
	UID     string    `json:"uid"`
	Summary string    `json:"summary,omitempty"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	TZID    string    `json:"tzid,omitempty"`
	AllDay  bool      `json:"all_day,omitempty"`
	RRule   string    `json:"rrule,omitempty"`
}

// BookResource reserva un recurso y devuelve ReservaAceptada, ReservaRechazada (ya
// estaba cogido) o ReservaPendiente (lo aprueba alguien a mano). El servicio comprueba
// la disponibilidad de forma atómica.
func (c *Calendars) BookResource(ctx context.Context, id string, b Booking) (string, error) {
	r, err := PostJSON[struct {
		Status string `json:"status"`
	}](ctx, c.c, "/resources/"+url.PathEscape(id)+"/book", b)
	return r.Status, err
}
