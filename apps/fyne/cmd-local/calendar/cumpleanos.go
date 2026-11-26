// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// El calendario de cumpleaños (A-07): un calendario VIRTUAL y de solo lectura con los
// cumpleaños de Contacts, leídos con el lector entre apps (K-04). No se guarda nada en
// Calendar: se calcula al pedir las ocurrencias, así que lo que cambie en Contacts sale
// en la siguiente carga. Si no hay ningún cumpleaños, el calendario no aparece.

// idCumpleanos es el identificador fijo del calendario virtual (los ocultos se
// recuerdan por él).
const idCumpleanos = "b1a7d4a0-0000-4000-8000-63756d706c65"

// espacioCumpleanos da a cada contacto el mismo identificador de evento siempre.
var espacioCumpleanos = uuid.MustParse("6f1d2c3b-5a49-4e87-9c10-0d6b7a8e9f21")

// vigenciaCumpleanos es cuánto vale lo leído de Contacts: una carga de la pantalla pide
// la lista y luego las ocurrencias, y no hace falta abrir Contacts dos veces.
const vigenciaCumpleanos = 30 * time.Second

// conCumpleanos es el repositorio de Calendar con el calendario de cumpleaños encima.
// Todo lo demás (y los grupos opcionales de la fachada) pasa tal cual al repositorio.
type conCumpleanos struct {
	*privateCalendar.Repository
	lector *lector.Lector
	nombre func() string                        // «Cumpleaños», en el idioma de la app
	titulo func(nombre string, edad int) string // el título de cada uno
	// cuentas, si no es nil, sincroniza los calendarios de una cuenta externa: su
	// «Actualizar ahora» (el de los de solo lectura) sincroniza la cuenta.
	cuentas *sincronia
	// rec es el almacén: borrar un evento o un calendario pide compactarlo en el acto.
	rec *almacen.Recursos

	mu      sync.Mutex
	leidos  time.Time
	cumples []lector.Cumple
}

func (c *conCumpleanos) leer(ctx context.Context) []lector.Cumple {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.leidos) < vigenciaCumpleanos {
		return c.cumples
	}
	l, err := c.lector.Cumpleanos(ctx)
	if err != nil {
		// Sin Contacts legible no hay cumpleaños; Calendar sigue funcionando.
		l = nil
	}
	c.cumples, c.leidos = l, time.Now()
	return l
}

func (c *conCumpleanos) calendario() api.Calendar {
	return api.Calendar{ID: idCumpleanos, Name: "cumpleanos", DisplayName: c.nombre(), Color: "#C026D3", Timezone: "UTC", ReadOnly: true}
}

// List añade el calendario de cumpleaños al final, si hay alguno.
func (c *conCumpleanos) List(ctx context.Context) ([]api.Calendar, error) {
	cals, err := c.Repository.List(ctx)
	if err != nil {
		return cals, err
	}
	if len(c.leer(ctx)) > 0 {
		cals = append(cals, c.calendario())
	}
	return cals, nil
}

// Instances da, para el calendario de cumpleaños, un evento de día completo por
// cumpleaños y año dentro de [desde, hasta). Un 29 de febrero cae el 28 los años que no
// son bisiestos.
func (c *conCumpleanos) Instances(ctx context.Context, cal string, desde, hasta time.Time) ([]api.Instance, error) {
	if cal != idCumpleanos {
		return c.Repository.Instances(ctx, cal, desde, hasta)
	}
	var out []api.Instance
	for _, b := range c.leer(ctx) {
		for ano := desde.UTC().Year(); ano <= hasta.UTC().Year(); ano++ {
			dia := b.Dia
			if b.Mes == time.February && dia == 29 && !bisiesto(ano) {
				dia = 28
			}
			inicio := time.Date(ano, b.Mes, dia, 0, 0, 0, 0, time.UTC)
			fin := inicio.AddDate(0, 0, 1)
			if !fin.After(desde) || !inicio.Before(hasta) {
				continue
			}
			edad := 0
			if b.Ano > 0 && ano > b.Ano {
				edad = ano - b.Ano
			}
			ev := c.evento(b, edad, inicio)
			out = append(out, api.Instance{Event: ev, Start: inicio, End: fin})
		}
	}
	slices.SortFunc(out, func(a, b api.Instance) int { return a.Start.Compare(b.Start) })
	return out, nil
}

func (c *conCumpleanos) evento(b lector.Cumple, edad int, inicio time.Time) api.Event {
	id := uuid.NewSHA1(espacioCumpleanos, []byte(b.ContactoID)).String()
	return api.Event{
		ID: id, CalendarID: idCumpleanos, UID: id + "@cumpleanos", Summary: c.titulo(b.Nombre, edad),
		DTStart: inicio, DTEnd: inicio.AddDate(0, 0, 1), TZID: "UTC", AllDay: true, Status: "CONFIRMED",
		Transparency: "TRANSPARENT", RRule: "FREQ=YEARLY",
	}
}

// GetEvent sirve también los eventos de cumpleaños (la vista rápida y el formulario
// de consulta los piden por id).
func (c *conCumpleanos) GetEvent(ctx context.Context, id string) (api.Event, error) {
	for _, b := range c.leer(ctx) {
		if uuid.NewSHA1(espacioCumpleanos, []byte(b.ContactoID)).String() != id {
			continue
		}
		hoy := time.Now().UTC()
		ev := c.evento(b, 0, time.Date(hoy.Year(), b.Mes, b.Dia, 0, 0, 0, 0, time.UTC))
		ninguno := []int{}
		ev.Reminders = &ninguno
		return ev, nil
	}
	return c.Repository.GetEvent(ctx, id)
}

// ExportICS no tiene sentido para un calendario que no se guarda.
func (c *conCumpleanos) ExportICS(ctx context.Context, cal string) ([]byte, error) {
	if cal == idCumpleanos {
		return nil, fmt.Errorf("calendar: el calendario de cumpleaños no se exporta")
	}
	return c.Repository.ExportICS(ctx, cal)
}

func bisiesto(a int) bool { return a%4 == 0 && (a%100 != 0 || a%400 == 0) }

// CreateEvent guarda el evento; en un calendario de una cuenta, con invitados y sin
// organizador, la persona organiza con su dirección de esa cuenta, lleva la reunión de
// Meet o Teams si se pidió en el formulario (reunion.go), y se sincroniza ya para que la
// invitación salga (invitar.go).
func (c *conCumpleanos) CreateEvent(ctx context.Context, cal string, in api.EventInput) (api.Event, error) {
	in = c.cuentas.conOrganizador(cal, in)
	ev, err := c.Repository.CreateEvent(ctx, cal, in)
	if err == nil {
		c.cuentas.marcarReunion(ctx, ev)
		c.cuentas.trasEscribir(cal)
	}
	return ev, err
}

// UpdateEvent es CreateEvent al cambiar un evento.
func (c *conCumpleanos) UpdateEvent(ctx context.Context, id string, in api.EventInput) (api.Event, error) {
	cal := ""
	if ev, err := c.Repository.GetEvent(ctx, id); err == nil {
		cal = ev.CalendarID
	}
	in = c.cuentas.conOrganizador(cal, in)
	ev, err := c.Repository.UpdateEvent(ctx, id, in)
	if err == nil {
		c.cuentas.marcarReunion(ctx, ev)
		c.cuentas.trasEscribir(cmpOr(ev.CalendarID, cal))
	}
	return ev, err
}

// RefreshSubscription de un calendario de una cuenta lo sincroniza (en segundo plano);
// el de una suscripción por URL vuelve a leer el `.ics`, como siempre.
func (c *conCumpleanos) RefreshSubscription(ctx context.Context, cal string) error {
	if c.cuentas != nil && c.cuentas.esDeCuenta(cal) {
		c.cuentas.motor.Ya()
		return nil
	}
	return c.Repository.RefreshSubscription(ctx, cal)
}
