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
package ical

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
	return RecortarSerie(icalData, recurrenceID, mode, OpcionesRecorte{})
}

// OpcionesRecorte son lo que pide Sytue Office al recortar una serie (sin ninguna,
// AmendMasterRecurrence). El servicio de Calendar, y con él la edición conectada, recorta
// con ComoElInicio.
type OpcionesRecorte struct {
	// SinExcepciones quita también las excepciones (RECURRENCE-ID) de lo recortado: la
	// de la ocurrencia que se quita o las de las ocurrencias desde la del corte. Si no,
	// se quedaban sueltas apuntando a ocurrencias que la serie ya no genera, y Calendar
	// las seguía enseñando (`event.ExpandEvents` enseña cada excepción): «solo este» o
	// «este y los siguientes» dejaban la ocurrencia dos veces —la suelta y la excepción
	// que creó mientras tanto la respuesta de un invitado— y borrar desde una dejaba las
	// cambiadas.
	SinExcepciones bool
	// ComoElInicio escribe la EXDATE nueva como el DTSTART del maestro —en la hora de su
	// zona, con su TZID—, no en UTC.
	ComoElInicio bool
}

// RecortarSerie es AmendMasterRecurrence con las [OpcionesRecorte].
func RecortarSerie(icalData string, recurrenceID time.Time, mode AmendMode, o OpcionesRecorte) (string, error) {
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
		addExDate(master, recurrenceID, allDay, o.ComoElInicio)
	case AmendUntil:
		if err := capRRuleUntil(master, recurrenceID, allDay); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("ical: modo de recorte desconocido: %q", mode)
	}
	if o.SinExcepciones {
		quedan := cal.Children[:0]
		for _, c := range cal.Children {
			if !excepcionRecortada(c, recurrenceID, mode == AmendUntil) {
				quedan = append(quedan, c)
			}
		}
		cal.Children = quedan
	}

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return "", fmt.Errorf("ical: re-serializar serie: %w", err)
	}
	return buf.String(), nil
}

// excepcionRecortada dice si c es la excepción de la ocurrencia `rid` o, con `desde`,
// la de una ocurrencia que empieza en `rid` o después.
func excepcionRecortada(c *ical.Component, rid time.Time, desde bool) bool {
	if c.Name != ical.CompEvent || c.Props.Get(ical.PropRecurrenceID) == nil {
		return false
	}
	t, _, err := PropertyTime(c.Props, ical.PropRecurrenceID)
	if err != nil {
		return false
	}
	if desde {
		return !t.Before(rid)
	}
	return t.Equal(rid)
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
// valor con el DTSTART (DATE para día completo; DATE-TIME UTC si no, o con
// `comoElInicio`, en la hora de la zona del DTSTART si la trae).
func addExDate(master *ical.Component, recurrenceID time.Time, allDay, comoElInicio bool) {
	p := ical.NewProp(ical.PropExceptionDates)
	switch {
	case allDay:
		p.Value = fmtICalDate(recurrenceID)
		p.Params.Set(ical.ParamValue, "DATE")
	case comoElInicio && zonaDelInicio(master) != nil:
		p = fechasComoElInicio(ical.PropExceptionDates, master.Props.Get(ical.PropDateTimeStart), []time.Time{recurrenceID})
	default:
		p.Value = fmtICalUTC(recurrenceID)
	}
	master.Props.Add(p)
}

// zonaDelInicio es la zona del DTSTART del maestro si es una hora local con un TZID que
// se conoce; nil si no.
func zonaDelInicio(master *ical.Component) *time.Location {
	p := master.Props.Get(ical.PropDateTimeStart)
	if p == nil || strings.HasSuffix(strings.TrimSpace(p.Value), "Z") {
		return nil
	}
	tz := strings.TrimSpace(p.Params.Get(ical.ParamTimezoneID))
	if tz == "" {
		return nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil
	}
	return loc
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
