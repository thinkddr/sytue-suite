// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Conversion entre el modelo de dominio de Sytue Calendar y el formato
// iCalendar (RFC 5545) usando emersion/go-ical (MIT). El recurso CalDAV se sirve
// verbatim desde calendar_objects.data; este puente solo se usa al ESCRIBIR
// (PUT CalDAV / API REST) para normalizar el VEVENT a las columnas consultables
// y al construir respuestas iTIP/free-busy.
//
// Clean-room: implementado sobre la especificacion RFC 5545 y la API publica de
// go-ical; sin codigo de terceros AGPL.
package ical

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"

	domain "git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
)

// ProductID es el PRODID de los VCALENDAR generados por Sytue Calendar.
const ProductID = "-//Sytue//Sytue Calendar//ES"

// ParsedObject es el resultado de decodificar un recurso iCalendar: el evento
// maestro normalizado y sus posibles overrides (mismos UID, distinto
// RECURRENCE-ID), junto con el UID comun.
type ParsedObject struct {
	UID       string
	Master    domain.Input
	Overrides []domain.Input
	Component domain.Component
}

// DecodeCalendarObject parsea un VCALENDAR (un recurso CalDAV) en su evento
// maestro y overrides. Exige al menos un VEVENT con UID. data es el iCalendar
// crudo tal cual lo envia el cliente (se conservara verbatim en el store).
func DecodeCalendarObject(data, calendarID string) (ParsedObject, error) {
	cal, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return ParsedObject{}, fmt.Errorf("ical: decodificar VCALENDAR: %w", err)
	}

	var po ParsedObject
	po.Component = domain.CompEvent
	var masterSet bool

	for _, child := range cal.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		evt := &ical.Event{Component: child}
		in, err := eventInputFromICal(evt)
		if err != nil {
			return ParsedObject{}, err
		}
		in.ICalData = data // todos los componentes del objeto comparten el blob.
		if po.UID == "" {
			po.UID = in.UID
		} else if in.UID != po.UID {
			return ParsedObject{}, fmt.Errorf("ical: el recurso mezcla UIDs distintos (%q vs %q)", po.UID, in.UID)
		}
		if in.RecurrenceID == nil {
			po.Master = in
			masterSet = true
		} else {
			po.Overrides = append(po.Overrides, in)
		}
	}

	if po.UID == "" {
		return ParsedObject{}, fmt.Errorf("ical: el recurso no contiene ningun VEVENT con UID")
	}
	if !masterSet && len(po.Overrides) > 0 {
		// Recurso solo-override (poco comun): promueve el primero a "maestro" para
		// poder indexarlo; conserva su RECURRENCE-ID.
		po.Master = po.Overrides[0]
		po.Overrides = po.Overrides[1:]
	}
	return po, nil
}

// eventInputFromICal mapea un *ical.Event a un domain.Input normalizado.
func eventInputFromICal(evt *ical.Event) (domain.Input, error) {
	uid := propText(evt.Props, ical.PropUID)
	if uid == "" {
		return domain.Input{}, fmt.Errorf("ical: VEVENT sin UID")
	}

	dtstart, allDay, err := PropertyTime(evt.Props, ical.PropDateTimeStart)
	if err != nil {
		return domain.Input{}, fmt.Errorf("ical: DTSTART: %w", err)
	}
	dtend, _, endErr := PropertyTime(evt.Props, ical.PropDateTimeEnd)
	if endErr != nil {
		// Sin DTEND: usa DURATION si esta, o asume 0/1 dia para all-day.
		if dur, ok := propDuration(evt.Props); ok {
			dtend = dtstart.Add(dur)
		} else if allDay {
			dtend = dtstart.Add(24 * time.Hour)
		} else {
			dtend = dtstart
		}
	}

	in := domain.Input{
		UID:            uid,
		ResourceName:   uid + ".ics",
		Summary:        propText(evt.Props, ical.PropSummary),
		Description:    propText(evt.Props, ical.PropDescription),
		Location:       propText(evt.Props, ical.PropLocation),
		Organizer:      cleanCalAddress(propText(evt.Props, ical.PropOrganizer)),
		OrganizerName:  organizerName(evt.Props),
		DTStart:        dtstart.UTC(),
		DTEnd:          dtend.UTC(),
		TZID:           tzidOf(evt.Props, ical.PropDateTimeStart),
		AllDay:         allDay,
		Status:         statusOf(propText(evt.Props, ical.PropStatus)),
		Classification: classOf(propText(evt.Props, ical.PropClass)),
		Transparency:   transpOf(propText(evt.Props, ical.PropTransparency)),
		Color:          colorFromICal(evt.Props),
		ConferenceURL:  domain.SanitizeConferenceURL(propText(evt.Props, "CONFERENCE")),
		SequenceNo:     propInt(evt.Props, ical.PropSequence),
		Attendees:      attendeesFromICal(evt),
	}

	rrule, err := recurrenceRuleValue(evt.Props)
	if err != nil {
		return domain.Input{}, err
	}
	if rrule != "" {
		rule, err := domain.ParseRRule(rrule)
		if err != nil {
			return domain.Input{}, err
		}
		in.RRule = rule.String()
		// recurrence_end desde el instante en la zona del evento: para series con
		// COUNT/UNTIL el último instante UTC depende del DST (±1 h). all-day => UTC.
		loc := domain.Location(in.TZID, in.AllDay)
		in.RecurrenceEnd = rule.RecurrenceEnd(in.DTStart.In(loc), in.DTEnd.Sub(in.DTStart))
	}
	in.RDate = propTimes(evt.Props, ical.PropRecurrenceDates)
	in.ExDate = propTimes(evt.Props, ical.PropExceptionDates)

	if rid, allDayRID, err := PropertyTime(evt.Props, ical.PropRecurrenceID); err == nil {
		_ = allDayRID
		u := rid.UTC()
		in.RecurrenceID = &u
	}
	return in, nil
}

// EncodeEvent serializa un domain.Event a un VCALENDAR (un VEVENT). Se usa para
// respuestas iTIP y para reconstruir recursos cuando el cliente no envia el
// iCalendar (p. ej. eventos creados via API REST).
func EncodeEvent(ev domain.Event, method domain.SchedulingMethod) (string, error) {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropProductID, ProductID)
	cal.Props.SetText(ical.PropVersion, "2.0")
	if method != "" {
		cal.Props.SetText(ical.PropMethod, string(method))
	}

	evt := ical.NewEvent()
	p := evt.Props
	p.SetText(ical.PropUID, ev.UID)
	p.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	// Emite DTSTART/DTEND en la ZONA del evento: go-ical serializa DTSTART;TZID=
	// <IANA>:<hora local> cuando la Location no es UTC, de modo que el TZID hace
	// round-trip sin pérdida y un cliente CalDAV externo expande la serie con DST
	// correcto. UTC y all-day mantienen la forma ...Z (loc = UTC).
	// ponytail: no emitimos VTIMEZONE inline; go-ical y los clientes mayores
	// (Google/Apple) resuelven el TZID IANA por nombre. Generar VTIMEZONE con sus
	// reglas DST es el upgrade si aparece un cliente estricto que lo exija.
	loc := domain.Location(ev.TZID, ev.AllDay)
	p.SetDateTime(ical.PropDateTimeStart, ev.DTStart.In(loc))
	p.SetDateTime(ical.PropDateTimeEnd, ev.DTEnd.In(loc))
	if ev.AllDay {
		p.SetDate(ical.PropDateTimeStart, ev.DTStart.UTC())
		p.SetDate(ical.PropDateTimeEnd, ev.DTEnd.UTC())
	}
	if ev.Summary != "" {
		p.SetText(ical.PropSummary, ev.Summary)
	}
	if ev.Description != "" {
		p.SetText(ical.PropDescription, ev.Description)
	}
	if ev.Location != "" {
		p.SetText(ical.PropLocation, ev.Location)
	}
	if ev.Organizer != "" {
		// Como el ATTENDEE: el valor tal cual, que es un CAL-ADDRESS. `SetText` le ponía
		// «;VALUE=TEXT», un ORGANIZER que no es una dirección para el cliente que recibe
		// la invitación por correo, y entonces no sabe a quién mandar el REPLY.
		org := ical.NewProp(ical.PropOrganizer)
		org.Value = ensureMailto(ev.Organizer)
		if cn := domain.SanitizeCommonName(ev.OrganizerName); cn != "" {
			org.Params.Set("CN", cn)
		}
		p.Set(org)
	}
	if ev.Status != "" {
		p.SetText(ical.PropStatus, string(ev.Status))
	}
	if ev.Classification != "" {
		p.SetText(ical.PropClass, string(ev.Classification))
	}
	if ev.Transparency != "" {
		p.SetText(ical.PropTransparency, string(ev.Transparency))
	}
	if ev.Color != "" {
		// Hex => X-APPLE-CALENDAR-COLOR (extension de Apple, la mas soportada por
		// clientes); nombre CSS3 => COLOR (RFC 7986). El valor viene ya saneado
		// (domain.SanitizeColor: sin ':', ';', CR/LF), asi que no inyecta lineas.
		if strings.HasPrefix(ev.Color, "#") {
			p.SetText("X-APPLE-CALENDAR-COLOR", ev.Color)
		} else {
			p.SetText("COLOR", ev.Color)
		}
	}
	if ev.ConferenceURL != "" {
		// CONFERENCE (RFC 7986) con VALUE=URI y FEATURE=VIDEO: es lo que hace que Apple
		// Calendar, Outlook y Thunderbird pinten «Unirse» en vez de un enlace suelto.
		// LABEL da el texto del boton. La URL viene saneada (domain.SanitizeConferenceURL:
		// solo http(s), sin ';', ',', CR/LF ni controles), asi que no parte la propiedad.
		conf := ical.NewProp("CONFERENCE")
		conf.Value = ev.ConferenceURL
		conf.Params.Set("VALUE", "URI")
		conf.Params.Set("FEATURE", "VIDEO")
		conf.Params.Set("LABEL", "Sytue Rooms")
		evt.Props.Add(conf)
	}
	if err := setRecurrenceRule(p, ev.RRule); err != nil {
		return "", err
	}
	setRecurrenceDates(p, ical.PropRecurrenceDates, ev.RDate, ev.AllDay)
	setRecurrenceDates(p, ical.PropExceptionDates, ev.ExDate, ev.AllDay)
	if ev.RecurrenceID != nil {
		p.SetDateTime(ical.PropRecurrenceID, ev.RecurrenceID.UTC())
		if ev.AllDay {
			p.SetDate(ical.PropRecurrenceID, ev.RecurrenceID.UTC())
		}
	}
	if ev.SequenceNo > 0 {
		p.SetText(ical.PropSequence, fmt.Sprintf("%d", ev.SequenceNo))
	}
	setAttendees(p, ev.Attendees)

	cal.Children = append(cal.Children, evt.Component)

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return "", fmt.Errorf("ical: codificar VCALENDAR: %w", err)
	}
	return buf.String(), nil
}

func setAttendees(props ical.Props, attendees []domain.Attendee) {
	for _, a := range attendees {
		prop := ical.NewProp(ical.PropAttendee)
		prop.Value = ensureMailto(a.Email)
		if a.CommonName != "" {
			prop.Params.Set("CN", a.CommonName)
		}
		if a.Role != "" {
			prop.Params.Set("ROLE", string(a.Role))
		}
		if a.PartStat != "" {
			prop.Params.Set("PARTSTAT", string(a.PartStat))
		}
		if a.CUType != "" {
			prop.Params.Set("CUTYPE", a.CUType)
		}
		prop.Params.Set("RSVP", boolStr(a.RSVP))
		props.Add(prop)
	}
}

// --- Helpers de propiedades --------------------------------------------------

func setRecurrenceDates(props ical.Props, name string, dates []time.Time, allDay bool) {
	if len(dates) == 0 {
		return
	}
	values := make([]string, len(dates))
	for i, value := range dates {
		if allDay {
			values[i] = value.UTC().Format("20060102")
		} else {
			values[i] = value.UTC().Format("20060102T150405Z")
		}
	}
	property := ical.NewProp(name)
	property.Value = strings.Join(values, ",")
	if allDay {
		property.Params.Set(ical.ParamValue, "DATE")
	}
	props.Set(property)
}

func setRecurrenceRule(props ical.Props, value string) error {
	if value == "" {
		return nil
	}
	rule, err := domain.ParseRRule(value)
	if err != nil {
		return err
	}
	property := ical.NewProp(ical.PropRecurrenceRule)
	property.Value = strings.TrimPrefix(rule.String(), "RRULE:")
	props.Set(property)
	return nil
}

// Older Sytue writers marked RRULE as TEXT and escaped separators. Decode that
// explicit historical representation through the maintained codec; valid RECUR
// input remains untouched. Retained source bytes are never rewritten here.
func recurrenceRuleValue(props ical.Props) (string, error) {
	property := props.Get(ical.PropRecurrenceRule)
	if property == nil {
		return "", nil
	}
	if property.ValueType() == ical.ValueText {
		value, err := property.Text()
		return strings.TrimSpace(value), err
	}
	return strings.TrimSpace(property.Value), nil
}

func PropertyText(props ical.Props, name string) string {
	return propText(props, name)
}

func propText(props ical.Props, name string) string {
	p := props.Get(name)
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.Value)
}

func propInt(props ical.Props, name string) int {
	p := props.Get(name)
	if p == nil {
		return 0
	}
	n := 0
	_, _ = fmt.Sscanf(strings.TrimSpace(p.Value), "%d", &n)
	return n
}

// PropertyTime devuelve el DATE/DATE-TIME de una propiedad y si es de dia completo
// (VALUE=DATE). Usa el parser de go-ical, que respeta TZID/UTC.
func PropertyTime(props ical.Props, name string) (time.Time, bool, error) {
	p := props.Get(name)
	if p == nil {
		return time.Time{}, false, fmt.Errorf("propiedad %s ausente", name)
	}
	allDay := strings.EqualFold(p.Params.Get(ical.ParamValue), "DATE")
	t, err := p.DateTime(time.UTC)
	if err != nil {
		return time.Time{}, allDay, err
	}
	return t, allDay, nil
}

func PropertyTimes(props ical.Props, name string) []time.Time {
	return propTimes(props, name)
}

// propTimes parsea una propiedad multivaluada de fechas (RDATE/EXDATE), separada
// por comas, en sus instantes UTC. Ignora valores no parseables.
func propTimes(props ical.Props, name string) []time.Time {
	values := props[name]
	if len(values) == 0 {
		return nil
	}
	var out []time.Time
	for i := range values {
		for _, raw := range strings.Split(values[i].Value, ",") {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			if t, err := parseICalRaw(raw); err == nil {
				out = append(out, t.UTC())
			}
		}
	}
	return out
}

func PropertyDuration(props ical.Props) (time.Duration, bool) {
	return propDuration(props)
}

func propDuration(props ical.Props) (time.Duration, bool) {
	p := props.Get(ical.PropDuration)
	if p == nil {
		return 0, false
	}
	d, err := parseICalDuration(p.Value)
	if err != nil {
		return 0, false
	}
	return d, true
}

func TimezoneIDOf(props ical.Props, name string) string {
	return tzidOf(props, name)
}

func tzidOf(props ical.Props, name string) string {
	p := props.Get(name)
	if p == nil {
		return "UTC"
	}
	if tz := p.Params.Get(ical.ParamTimezoneID); tz != "" {
		return tz
	}
	return "UTC"
}

func statusOf(s string) domain.Status {
	st := domain.Status(strings.ToUpper(s))
	if domain.ValidStatus(st) {
		return st
	}
	return domain.StatusConfirmed
}

func classOf(s string) domain.Classification {
	c := domain.Classification(strings.ToUpper(s))
	if domain.ValidClassification(c) {
		return c
	}
	return domain.ClassPublic
}

func transpOf(s string) domain.Transparency {
	t := domain.Transparency(strings.ToUpper(s))
	if domain.ValidTransparency(t) {
		return t
	}
	return domain.TranspOpaque
}

// colorFromICal lee el color del evento: X-APPLE-CALENDAR-COLOR (hex, la
// extension mas extendida) y, si no, COLOR (nombre CSS3, RFC 7986). Saneado para
// descartar valores peligrosos (domain.SanitizeColor).
func colorFromICal(props ical.Props) string {
	if c := domain.SanitizeColor(propText(props, "X-APPLE-CALENDAR-COLOR")); c != "" {
		return c
	}
	return domain.SanitizeColor(propText(props, "COLOR"))
}

func attendeesFromICal(evt *ical.Event) []domain.Attendee {
	props := evt.Props[ical.PropAttendee]
	if len(props) == 0 {
		return nil
	}
	out := make([]domain.Attendee, 0, len(props))
	for i := range props {
		p := props[i]
		a := domain.Attendee{
			Email:      cleanCalAddress(p.Value),
			CommonName: p.Params.Get("CN"),
			Role:       domain.AttendeeRole(orDefault(strings.ToUpper(p.Params.Get("ROLE")), string(domain.RoleRequired))),
			PartStat:   domain.ParticipationStatus(orDefault(strings.ToUpper(p.Params.Get("PARTSTAT")), string(domain.PartNeedsAction))),
			CUType:     orDefault(strings.ToUpper(p.Params.Get("CUTYPE")), "INDIVIDUAL"),
			RSVP:       strings.EqualFold(p.Params.Get("RSVP"), "TRUE"),
		}
		if a.Email == "" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// organizerName es el CN del ORGANIZER, si lo trae: el nombre con el que la web y el
// cliente de correo de quien recibe la invitación presentan a quien organiza.
func organizerName(props ical.Props) string {
	if p := props.Get(ical.PropOrganizer); p != nil {
		return domain.SanitizeCommonName(p.Params.Get("CN"))
	}
	return ""
}

func CleanCalAddress(v string) string {
	return cleanCalAddress(v)
}

// cleanCalAddress normaliza una direccion de calendario ("mailto:a@b" -> "a@b").
func cleanCalAddress(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(strings.ToLower(v), "mailto:") {
		return v[len("mailto:"):]
	}
	return v
}

func ensureMailto(email string) string {
	if strings.Contains(email, ":") {
		return email
	}
	return "mailto:" + email
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func boolStr(b bool) string {
	if b {
		return "TRUE"
	}
	return "FALSE"
}
