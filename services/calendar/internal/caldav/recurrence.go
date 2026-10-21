// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Edición por alcance de series recurrentes (RFC 5545 3.8.5): dado el recurso
// iCalendar de una serie, aplica el límite que corresponde al alcance elegido en
// la UI («solo este» / «este y siguientes») sobre el VEVENT MAESTRO, sin tocar
// nada más del recurso: conserva VALARM/ATTENDEE, los EXDATE/RDATE previos y los
// VEVENT override (RECURRENCE-ID). La nueva instancia/serie suelta la crea el
// cliente como objeto independiente; aquí solo se recorta el maestro para que la
// serie no vuelva a generar la(s) instancia(s) desgajada(s).
//
// Clean-room sobre RFC 5545, usando go-ical (MIT) y la stdlib.
package caldav

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// AmendMode es el tipo de recorte aplicado al maestro de una serie.
type AmendMode string

const (
	// AmendExclude añade EXDATE=recurrenceID (alcance «solo este»): esa instancia
	// deja de generarse; la edición suelta se materializa aparte.
	AmendExclude AmendMode = "exclude"
	// AmendUntil recorta el RRULE del maestro con UNTIL justo antes de esta
	// instancia (alcance «este y siguientes»): la serie termina antes de ella.
	AmendUntil AmendMode = "until"
)

// AmendMasterRecurrence aplica `mode` al VEVENT maestro del recurso `icalData`
// para la instancia cuyo inicio original es `recurrenceID`, y devuelve el
// recurso re-serializado. Solo muta el maestro (el VEVENT sin RECURRENCE-ID);
// preserva el resto verbatim vía go-ical.
func AmendMasterRecurrence(icalData string, recurrenceID time.Time, mode AmendMode) (string, error) {
	cal, err := ical.NewDecoder(strings.NewReader(icalData)).Decode()
	if err != nil {
		return "", fmt.Errorf("ical: decodificar serie: %w", err)
	}
	master := masterComponent(cal)
	if master == nil {
		return "", fmt.Errorf("ical: el recurso no tiene VEVENT maestro")
	}
	allDay := componentAllDay(master)

	switch mode {
	case AmendExclude:
		addExDate(master, recurrenceID, allDay)
	case AmendUntil:
		if err := capRRuleUntil(master, recurrenceID, allDay); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("ical: modo de recorte desconocido: %q", mode)
	}

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return "", fmt.Errorf("ical: re-serializar serie: %w", err)
	}
	return buf.String(), nil
}

// masterComponent devuelve el VEVENT maestro (sin RECURRENCE-ID) del recurso, o
// nil si no lo hay. Los overrides (con RECURRENCE-ID) se dejan intactos.
func masterComponent(cal *ical.Calendar) *ical.Component {
	for _, child := range cal.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		if child.Props.Get(ical.PropRecurrenceID) == nil {
			return child
		}
	}
	return nil
}

// componentAllDay indica si el DTSTART del componente es de día completo (DATE).
func componentAllDay(c *ical.Component) bool {
	p := c.Props.Get(ical.PropDateTimeStart)
	return p != nil && strings.EqualFold(p.Params.Get(ical.ParamValue), "DATE")
}

// addExDate añade un EXDATE con `recurrenceID` al maestro, conservando cualquier
// EXDATE previo (RFC 5545 permite varias propiedades EXDATE). Casa el tipo de
// valor con el DTSTART (DATE para día completo; DATE-TIME UTC si no).
func addExDate(master *ical.Component, recurrenceID time.Time, allDay bool) {
	p := ical.NewProp(ical.PropExceptionDates)
	if allDay {
		p.Value = fmtICalDate(recurrenceID)
		p.Params.Set(ical.ParamValue, "DATE")
	} else {
		p.Value = fmtICalUTC(recurrenceID)
	}
	master.Props.Add(p)
}

// capRRuleUntil reescribe el RRULE del maestro fijando UNTIL justo antes de
// `recurrenceID` y descartando COUNT/UNTIL previos, preservando el resto de
// componentes de la regla verbatim (FREQ/INTERVAL/BY*/WKST, incl. los que el
// motor no interpreta) para no corromperla.
func capRRuleUntil(master *ical.Component, recurrenceID time.Time, allDay bool) error {
	prop := master.Props.Get(ical.PropRecurrenceRule)
	if prop == nil {
		return fmt.Errorf("ical: el maestro no tiene RRULE que recortar")
	}
	var until string
	if allDay {
		// Día completo: el último día conservado es el anterior a la instancia.
		until = fmtICalDate(recurrenceID.AddDate(0, 0, -1))
	} else {
		// Con hora: hasta un segundo antes (UNTIL es inclusivo en RFC 5545).
		until = fmtICalUTC(recurrenceID.Add(-time.Second))
	}
	prop.Value = replaceRRuleBound(prop.Value, until)
	return nil
}

// replaceRRuleBound elimina los tokens COUNT= y UNTIL= del valor de un RRULE y le
// añade UNTIL=<until>, conservando el orden y el resto de tokens tal cual.
func replaceRRuleBound(value, until string) string {
	parts := strings.Split(value, ";")
	out := parts[:0]
	for _, p := range parts {
		key := strings.ToUpper(strings.TrimSpace(p))
		if strings.HasPrefix(key, "COUNT=") || strings.HasPrefix(key, "UNTIL=") {
			continue
		}
		if strings.TrimSpace(p) == "" {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(append(out, "UNTIL="+until), ";")
}

func fmtICalUTC(t time.Time) string  { return t.UTC().Format("20060102T150405Z") }
func fmtICalDate(t time.Time) string { return t.UTC().Format("20060102") }
