// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"

	domain "git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
)

// Lo que ven los invitados, lo escriba quien lo escriba.
//
// Un mismo evento llega escrito de muchas formas: la EXDATE en la hora de su zona
// (iCloud) o en UTC (el formulario), sin STATUS o con `STATUS:CONFIRMED`, el fin con
// DTEND o con DURATION, la regla con sus partes en otro orden o con el UNTIL a otra hora
// del mismo día… Comparar el TEXTO tomaba por un cambio para los invitados volver a
// escribir lo mismo de otra forma: la primera vez que se guardaba desde el formulario un
// evento que vino de la cuenta, subía la SEQUENCE y salía la invitación. Aquí cada
// propiedad de [PropiedadesDeLaInvitacion] se reduce a lo que SIGNIFICA:
//
//   - una fecha o una hora, al instante (en UTC) o a la fecha, en la zona que viniera;
//     el inicio de una serie lleva además su zona, que decide la hora de cada repetición
//     a los dos lados de un cambio de hora;
//   - el fin, al instante en que acaba (DTEND, o el inicio más DURATION);
//   - RDATE y EXDATE, al conjunto de sus instantes, en una o en varias líneas;
//   - la regla, a sus partes en un orden fijo, sin lo que vale lo de siempre
//     (`INTERVAL=1`, `WKST=MO` o un WKST que no cuenta) y con el UNTIL como la última
//     repetición que deja;
//   - un texto, a lo que dice (sin los escapes);
//   - el STATUS que falta, a CONFIRMED; el organizador, a su dirección.

// ContenidoDeLaInvitacion es lo que ven los invitados de un VCALENDAR —cada VEVENT con
// las [PropiedadesDeLaInvitacion] por lo que significan y quiénes están invitados—, en
// un orden fijo. Dos recursos con el mismo contenido son la misma invitación aunque
// estén escritos de otra forma. Es lo que compara [CambiaLaInvitacion] y lo que firma
// la invitación por correo (`local/agenda`).
func ContenidoDeLaInvitacion(cal *ical.Calendar) string {
	var trozos []string
	for _, ev := range cal.Children {
		if ev.Name != ical.CompEvent {
			continue
		}
		var b strings.Builder
		for _, n := range PropiedadesDeLaInvitacion {
			switch n {
			case ical.PropDuration:
				continue // va en el fin
			case ical.PropDateTimeEnd:
				b.WriteString("FIN:" + finSemantico(ev) + "\n")
			default:
				if v, ok := valorSemantico(ev, n); ok {
					b.WriteString(n + ":" + v + "\n")
				}
			}
		}
		var as []string
		for _, p := range ev.Props.Values(ical.PropAttendee) {
			if d := cleanCalAddress(strings.ToLower(p.Value)); d != "" {
				as = append(as, d)
			}
		}
		sort.Strings(as)
		b.WriteString(strings.Join(slices.Compact(as), ","))
		trozos = append(trozos, b.String())
	}
	sort.Strings(trozos)
	return strings.Join(trozos, "\x00")
}

// valorSemantico es lo que significa la propiedad `nombre` del VEVENT ev; ok=false si
// no la tiene y no hay un valor que valga por ella.
func valorSemantico(ev *ical.Component, nombre string) (string, bool) {
	if siFalta, ok := valoresSiFaltan[nombre]; ok {
		if v := strings.ToUpper(propText(ev.Props, nombre)); v != "" {
			return v, true
		}
		return siFalta, true
	}
	switch nombre {
	case ical.PropOrganizer:
		p := ev.Props.Get(nombre)
		if p == nil {
			return "", false
		}
		return cleanCalAddress(strings.ToLower(strings.TrimSpace(p.Value))), true
	case ical.PropDateTimeStart:
		return inicioSemantico(ev)
	case ical.PropDateTimeEnd, ical.PropDuration:
		return finSemantico(ev), true
	case ical.PropRecurrenceID:
		return recurrenciaSemantica(ev)
	case ical.PropRecurrenceDates, ical.PropExceptionDates:
		if len(ev.Props[nombre]) == 0 {
			return "", false
		}
		return fechasSemanticas(ev.Props[nombre]), true
	case ical.PropRecurrenceRule:
		if ev.Props.Get(nombre) == nil {
			return "", false
		}
		return reglaSemantica(ev), true
	case "COLOR", "X-APPLE-CALENDAR-COLOR":
		c := colorFromICal(ev.Props)
		return strings.ToLower(c), c != ""
	}
	ps := ev.Props[nombre]
	if len(ps) == 0 {
		return "", false
	}
	vs := make([]string, 0, len(ps))
	for i := range ps {
		vs = append(vs, textoSemantico(&ps[i]))
	}
	return strings.Join(vs, "\x01"), true
}

// valoresSiFaltan son las propiedades que, si faltan, valen lo que dice RFC 5545: el
// STATUS, CONFIRMED; la clase, PUBLIC; la transparencia, OPAQUE.
var valoresSiFaltan = map[string]string{
	ical.PropStatus:       string(domain.StatusConfirmed),
	ical.PropClass:        string(domain.ClassPublic),
	ical.PropTransparency: string(domain.TranspOpaque),
}

// inicioSemantico es el DTSTART del VEVENT ev por lo que significa: el instante y, en
// el maestro de una serie con hora, su zona. ok=false sin DTSTART.
func inicioSemantico(ev *ical.Component) (string, bool) {
	p := ev.Props.Get(ical.PropDateTimeStart)
	if p == nil {
		return "", false
	}
	v := instanteSemantico(p)
	if seRepite(ev) && p.ValueType() != ical.ValueDate {
		v += "@" + zonaSemantica(p)
	}
	return v, true
}

// recurrenciaSemantica es el RECURRENCE-ID del VEVENT ev por lo que significa: el
// instante y su RANGE. ok=false sin RECURRENCE-ID.
func recurrenciaSemantica(ev *ical.Component) (string, bool) {
	p := ev.Props.Get(ical.PropRecurrenceID)
	if p == nil {
		return "", false
	}
	v := instanteSemantico(p)
	if r := strings.ToUpper(strings.TrimSpace(p.Params.Get(ical.ParamRange))); r != "" {
		v += ";" + r
	}
	return v, true
}

// textoSemantico es lo que dice un texto: sin los escapes ni los blancos de los
// extremos, y con los saltos de línea de una sola forma.
func textoSemantico(p *ical.Prop) string {
	t, err := p.Text()
	if err != nil {
		t = p.Value
	}
	return strings.TrimSpace(strings.ReplaceAll(t, "\r\n", "\n"))
}

// seRepite: el VEVENT es el maestro de una serie (RRULE o RDATE).
func seRepite(ev *ical.Component) bool {
	return ev.Props.Get(ical.PropRecurrenceID) == nil &&
		(ev.Props.Get(ical.PropRecurrenceRule) != nil || ev.Props.Get(ical.PropRecurrenceDates) != nil)
}

// instanteSemantico es el instante de una propiedad DATE o DATE-TIME: «D20261201» una
// fecha y la hora en UTC («20261201T090000Z») una hora, la traiga en UTC, en su TZID o
// flotante (que Calendar lee en UTC). Lo que no se deja leer vale por lo que pone.
func instanteSemantico(p *ical.Prop) string {
	v := strings.TrimSpace(p.Value)
	if p.ValueType() == ical.ValueDate || len(v) == 8 {
		return "D" + v
	}
	t, err := p.DateTime(time.UTC)
	if err != nil {
		return "?" + lineaDe(*p)
	}
	return t.UTC().Format("20060102T150405Z")
}

// zonaSemantica es la zona en la que se repite una serie: la de su TZID, como la
// resuelve la expansión (`event.LoadLocationCached`); una hora en UTC o flotante, UTC.
func zonaSemantica(p *ical.Prop) string {
	tz := strings.TrimSpace(p.Params.Get(ical.ParamTimezoneID))
	if tz == "" || strings.HasSuffix(strings.TrimSpace(p.Value), "Z") {
		return "UTC"
	}
	switch n := domain.LoadLocationCached(tz).String(); n {
	case "UTC", "Etc/UTC", "Etc/GMT", "GMT", "Etc/Universal", "Universal", "Zulu", "Etc/Zulu":
		return "UTC"
	default:
		return n
	}
}

// finSemantico es cuándo acaba el VEVENT: su DTEND o, si no, su inicio más su
// DURATION; sin ninguno, lo que dice RFC 5545 §3.6.1 (un día si es de fecha, nada si
// es de hora).
func finSemantico(ev *ical.Component) string {
	if p := ev.Props.Get(ical.PropDateTimeEnd); p != nil {
		return instanteSemantico(p)
	}
	ini := ev.Props.Get(ical.PropDateTimeStart)
	if ini == nil {
		return ""
	}
	inicio, err := ini.DateTime(time.UTC)
	if err != nil {
		return "?" + lineaDe(*ini)
	}
	esFecha := ini.ValueType() == ical.ValueDate || len(strings.TrimSpace(ini.Value)) == 8
	if d, ok := propDuration(ev.Props); ok {
		if esFecha && d%(24*time.Hour) == 0 {
			return "D" + inicio.Add(d).Format("20060102")
		}
		return inicio.Add(d).UTC().Format("20060102T150405Z")
	}
	if esFecha {
		return "D" + inicio.AddDate(0, 0, 1).Format("20060102")
	}
	return inicio.UTC().Format("20060102T150405Z")
}

// fechasSemanticas es el conjunto de los instantes (o las fechas) de las RDATE o las
// EXDATE de un VEVENT, en orden y sin repetir, estén en una línea o en varias y en la
// zona que estén.
func fechasSemanticas(ps []ical.Prop) string {
	var out []string
	for i := range ps {
		p := &ps[i]
		tipo := strings.ToUpper(p.Params.Get(ical.ParamValue))
		var zona *time.Location
		if tz := strings.TrimSpace(p.Params.Get(ical.ParamTimezoneID)); tz != "" {
			zona = domain.LoadLocationCached(tz)
		}
		for _, crudo := range strings.Split(p.Value, ",") {
			crudo = strings.TrimSpace(crudo)
			switch {
			case crudo == "":
				continue
			case tipo == "DATE" || len(crudo) == 8:
				out = append(out, "D"+crudo)
				continue
			case tipo == "PERIOD" || strings.Contains(crudo, "/"):
				out = append(out, "P"+crudo)
				continue
			case zona != nil && len(crudo) == 15:
				if t, err := time.ParseInLocation("20060102T150405", crudo, zona); err == nil {
					out = append(out, t.UTC().Format("20060102T150405Z"))
					continue
				}
			default:
				if t, err := parseICalRaw(crudo); err == nil {
					out = append(out, t.UTC().Format("20060102T150405Z"))
					continue
				}
			}
			out = append(out, "?"+crudo)
		}
	}
	sort.Strings(out)
	return strings.Join(slices.Compact(out), ",")
}

// partesDeLaRegla son las de RRULE cuyo valor es una lista: su orden no cuenta.
var partesDeLaRegla = map[string]bool{
	"BYSECOND": true, "BYMINUTE": true, "BYHOUR": true, "BYDAY": true,
	"BYMONTHDAY": true, "BYYEARDAY": true, "BYWEEKNO": true, "BYMONTH": true, "BYSETPOS": true,
}

// reglaSemantica es la RRULE del VEVENT con sus partes en un orden fijo y sin las que
// valen lo de siempre. El UNTIL es la última repetición que deja (dos UNTIL a horas
// distintas entre las que no cae ninguna son el mismo final).
func reglaSemantica(ev *ical.Component) string {
	crudo, err := recurrenceRuleValue(ev.Props)
	if err != nil || crudo == "" {
		return "?" + valorDe(ev, ical.PropRecurrenceRule)
	}
	crudo = strings.TrimPrefix(crudo, "RRULE:")
	partes := map[string]string{}
	for _, kv := range strings.Split(crudo, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if !ok || k == "" {
			continue
		}
		k, v = strings.ToUpper(strings.TrimSpace(k)), strings.ToUpper(strings.TrimSpace(v))
		if partesDeLaRegla[k] {
			l := strings.Split(v, ",")
			for i := range l {
				l[i] = strings.TrimSpace(strings.TrimPrefix(l[i], "+"))
				if n, err := strconv.Atoi(l[i]); err == nil {
					l[i] = strconv.Itoa(n)
				}
			}
			sort.Strings(l)
			v = strings.Join(slices.Compact(l), ",")
		}
		partes[k] = v
	}
	if partes["INTERVAL"] == "1" {
		delete(partes, "INTERVAL")
	}
	// WKST solo cuenta en una semanal cada varias semanas o con BYWEEKNO.
	if w := partes["WKST"]; w == "MO" || partes["BYWEEKNO"] == "" && (partes["FREQ"] != "WEEKLY" || partes["INTERVAL"] == "") {
		delete(partes, "WKST")
	}
	if u, ok := partes["UNTIL"]; ok {
		partes["UNTIL"] = ultimaRepeticion(ev, crudo, u)
	}
	claves := make([]string, 0, len(partes))
	for k := range partes {
		claves = append(claves, k)
	}
	sort.Strings(claves)
	var b strings.Builder
	for i, k := range claves {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(k + "=" + partes[k])
	}
	return b.String()
}

// ultimaRepeticion es el inicio de la última repetición de la serie que deja su UNTIL
// (`hasta`, tal cual), con la expansión de Calendar; si no se puede expandir, el UNTIL
// en UTC.
func ultimaRepeticion(ev *ical.Component, regla, hasta string) string {
	fijo := "U" + hasta
	if t, err := parseICalRaw(hasta); err == nil {
		fijo = "U" + t.UTC().Format("20060102T150405Z")
	}
	r, err := domain.ParseRRule(regla)
	if err != nil || r.Until == nil {
		return fijo
	}
	ini := ev.Props.Get(ical.PropDateTimeStart)
	if ini == nil {
		return fijo
	}
	inicio, err := ini.DateTime(time.UTC)
	if err != nil {
		return fijo
	}
	esFecha := ini.ValueType() == ical.ValueDate
	loc := domain.Location(strings.TrimSpace(ini.Params.Get(ical.ParamTimezoneID)), esFecha)
	if strings.HasSuffix(strings.TrimSpace(ini.Value), "Z") {
		loc = time.UTC
	}
	l := r.Expand(inicio.In(loc), nil, nil, inicio, r.Until.Add(time.Second))
	if len(l) == 0 {
		return "U-"
	}
	if esFecha {
		return "U" + l[len(l)-1].Format("20060102")
	}
	return "U" + l[len(l)-1].UTC().Format("20060102T150405Z")
}
