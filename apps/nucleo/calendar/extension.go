// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendar

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-ical"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
)

// Una EXTENSIÓN (propiedad X- de RFC 5545 §3.8.8.2) en el VEVENT principal de un evento:
// lo que una app de la edición local apunta en el propio `.ics` para su sincronización
// (Sytue Office: «que el proveedor le cree el enlace de la reunión al subirlo», A-13).
// Es una adición: nadie más la usa, y lo demás del evento no cambia. Las ediciones de
// después la conservan como cualquier propiedad que la API no expresa
// (`conservarPropiedades`).

// nombreExtension es el de una propiedad X- que se deja poner: letras, cifras y guiones.
var nombreExtension = regexp.MustCompile(`^X-[A-Z0-9-]{1,60}$`)

// PonerExtension pone en el VEVENT principal del evento eventID la propiedad X- nombre con
// valor, o la quita con valor "". El calendario tiene que dejar escribir.
func (r *Repository) PonerExtension(ctx context.Context, eventID, nombre, valor string) error {
	nombre = strings.ToUpper(strings.TrimSpace(nombre))
	if !nombreExtension.MatchString(nombre) || strings.ContainsAny(valor, "\r\n\x00") || len(valor) > 512 {
		return invalid()
	}
	_, err := r.mutate(ctx, "set_extension", struct{ ID, Nombre, Valor string }{eventID, nombre, valor}, func(s *model.Snapshot) (model.Aggregate, error) {
		ev, err := findEvent(s, eventID)
		if err != nil {
			return model.Aggregate{}, err
		}
		if _, err := ownCalendar(s, ev.CalendarID.String(), true); err != nil {
			return model.Aggregate{}, err
		}
		row, ok := s.Row("object", ev.ObjectID.String())
		if !ok {
			return model.Aggregate{}, missing()
		}
		data, err := objectData(row)
		if err != nil {
			return model.Aggregate{}, err
		}
		data, err = conExtension(data, nombre, valor)
		if err != nil {
			return model.Aggregate{}, err
		}
		a, err := s.Edit("object_state", row.ID)
		if err != nil {
			return a, err
		}
		return r.reproject(a, data, time.Now().UTC())
	})
	return err
}

// conExtension es el iCalendar data con la propiedad X- nombre puesta a valor (o
// quitada con valor "") en su VEVENT principal.
func conExtension(data, nombre, valor string) (string, error) {
	cal, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return "", invalid()
	}
	maestro := principal(cal)
	if maestro == nil {
		return "", invalid()
	}
	if valor == "" {
		maestro.Props.Del(nombre)
	} else {
		p := ical.NewProp(nombre)
		p.SetText(valor)
		p.Params.Del(ical.ParamValue) // TEXT es el tipo de una X- sin VALUE
		maestro.Props.Set(p)
	}
	var b bytes.Buffer
	if err := ical.NewEncoder(&b).Encode(cal); err != nil {
		return "", err
	}
	return b.String(), nil
}

// principal es el VEVENT principal de cal: el maestro (sin RECURRENCE-ID) o, si solo
// hay excepciones, la primera. nil si no hay ninguno.
func principal(cal *ical.Calendar) *ical.Component {
	var maestro *ical.Component
	for _, c := range cal.Children {
		if c.Name != ical.CompEvent {
			continue
		}
		if maestro == nil || c.Props.Get(ical.PropRecurrenceID) == nil && maestro.Props.Get(ical.PropRecurrenceID) != nil {
			maestro = c
		}
	}
	return maestro
}
