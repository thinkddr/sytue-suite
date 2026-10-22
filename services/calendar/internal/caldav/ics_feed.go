// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// DecodeICSFeed parsea un feed ICS EXTERNO completo (un VCALENDAR con MUCHOS
// VEVENT de UIDs distintos, como un calendario de festivos o de equipo) en un
// ParsedObject por UID — el recurso CalDAV que el store cachea. Agrupa los
// VEVENT por UID (maestro + overrides RECURRENCE-ID comparten UID), re-serializa
// cada grupo en un mini-VCALENDAR y REUSA DecodeCalendarObject para normalizarlo,
// de modo que el parser/RRULE/TZ existentes se aplican tal cual.
//
// El contenido es DATOS: se parsea, nunca se ejecuta. maxObjects acota el número
// de recursos (defensa frente a feeds abusivos); superarlo es un error (no se
// cachea un feed truncado a medias). Los VEVENT sin UID se ignoran (no pueden ser
// un recurso estable).
//
// ponytail: no copiamos VTIMEZONE a cada objeto; los TZID IANA se resuelven por
// nombre (time/tzdata embebido), igual que EncodeEvent y el resto de Calendar.
// Un TZID no-IANA degrada a UTC (comportamiento documentado). Emitir VTIMEZONE
// inline por objeto es el upgrade si aparece un feed que lo exija.
func DecodeICSFeed(data string, maxObjects int) ([]ParsedObject, error) {
	cal, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return nil, fmt.Errorf("ics: decodificar feed VCALENDAR: %w", err)
	}
	if maxObjects <= 0 {
		maxObjects = 10000
	}

	// Agrupa los VEVENT por UID conservando el orden de primera aparición (para
	// una salida determinista).
	order := make([]string, 0, 16)
	groups := make(map[string][]*ical.Component)
	for _, child := range cal.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		uid := strings.TrimSpace(child.Props.Get(ical.PropUID).Value)
		if uid == "" {
			continue // VEVENT sin UID: no es un recurso indexable
		}
		if _, seen := groups[uid]; !seen {
			order = append(order, uid)
			if len(order) > maxObjects {
				return nil, fmt.Errorf("ics: el feed excede el máximo de %d eventos", maxObjects)
			}
		}
		groups[uid] = append(groups[uid], child)
	}

	out := make([]ParsedObject, 0, len(order))
	for _, uid := range order {
		objData, err := encodeGroup(groups[uid])
		if err != nil {
			continue // grupo ilegible: se omite, no rompe el feed entero
		}
		po, err := DecodeCalendarObject(objData, "")
		if err != nil {
			continue
		}
		out = append(out, po)
	}
	return out, nil
}

// encodeGroup serializa los VEVENT de un mismo UID en un VCALENDAR mínimo (el
// recurso CalDAV que se cachea verbatim). PRODID/VERSION propios; sin VTIMEZONE.
func encodeGroup(children []*ical.Component) (string, error) {
	c := ical.NewCalendar()
	c.Props.SetText(ical.PropProductID, ProductID)
	c.Props.SetText(ical.PropVersion, "2.0")
	for _, child := range children {
		// go-ical exige exactamente un DTSTAMP por VEVENT (RFC 5545). Algunos feeds
		// externos lo omiten: lo fijamos a "ahora" para que el recurso re-serialice.
		if child.Props.Get(ical.PropDateTimeStamp) == nil {
			child.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
		}
		c.Children = append(c.Children, child)
	}
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(c); err != nil {
		return "", err
	}
	return buf.String(), nil
}
