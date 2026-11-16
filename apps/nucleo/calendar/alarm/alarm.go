// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package alarm extrae los disparos (TRIGGER) de los VALARM de un recurso
// iCalendar y los resuelve a instantes absolutos, para que el store precompute
// la cola de recordatorios (calendar_reminders) que consume el worker de avisos.
//
// Paquete hoja: solo depende de emersion/go-ical (MIT) y de la stdlib; no importa
// store ni caldav (evita ciclos). Clean-room sobre RFC 5545 3.6.6 / 3.8.6.3.
package alarm

import (
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// Trigger es el disparo absoluto de un VALARM: cuándo avisar y con qué ACTION
// (DISPLAY/EMAIL/AUDIO). At está en UTC.
type Trigger struct {
	At     time.Time
	Action string
}

// Triggers resuelve los disparos de los VALARM del VEVENT maestro (el primero)
// del recurso, dados su DTSTART y DTEND. El TRIGGER puede ser:
//   - una DURATION relativa (por defecto a DTSTART; a DTEND si RELATED=END), o
//   - un instante absoluto (VALUE=DATE-TIME).
//
// Los VALARM sin TRIGGER legible se ignoran (best-effort): un recordatorio
// ilegible no debe impedir guardar el evento. Devuelve error solo si el propio
// VCALENDAR no decodifica.
func Triggers(icalData string, dtstart, dtend time.Time) ([]Trigger, error) {
	cal, err := ical.NewDecoder(strings.NewReader(icalData)).Decode()
	if err != nil {
		return nil, err
	}
	var out []Trigger
	for _, comp := range cal.Children {
		if comp.Name != ical.CompEvent {
			continue
		}
		for _, sub := range comp.Children {
			if sub.Name != ical.CompAlarm {
				continue
			}
			p := sub.Props.Get(ical.PropTrigger)
			if p == nil {
				continue
			}
			at, ok := triggerTime(p, dtstart, dtend)
			if !ok {
				continue
			}
			action := ""
			if a := sub.Props.Get(ical.PropAction); a != nil {
				action = strings.ToUpper(strings.TrimSpace(a.Value))
			}
			out = append(out, Trigger{At: at, Action: action})
		}
		break // solo el VEVENT maestro; los overrides comparten los mismos VALARM
	}
	return out, nil
}

// triggerTime resuelve un TRIGGER a su instante absoluto en UTC. VALUE=DATE-TIME
// es absoluto; en cualquier otro caso se interpreta como DURATION (el valor por
// defecto del TRIGGER según RFC 5545) relativa a DTSTART, o a DTEND si RELATED=END.
func triggerTime(p *ical.Prop, dtstart, dtend time.Time) (time.Time, bool) {
	if p.ValueType() == ical.ValueDateTime {
		if t, err := p.DateTime(time.UTC); err == nil {
			return t.UTC(), true
		}
		return time.Time{}, false
	}
	d, err := p.Duration()
	if err != nil {
		return time.Time{}, false
	}
	base := dtstart
	if strings.EqualFold(p.Params.Get(ical.ParamRelated), "END") {
		base = dtend
	}
	return base.Add(d).UTC(), true
}
