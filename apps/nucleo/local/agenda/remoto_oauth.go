// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// Lo común de las API JSON de Google y de Microsoft Graph: el token de la cuenta en
// cada petición, los errores traducidos a los del sincronizador y el iCalendar que se
// guarda aquí, escrito y leído a partir de sus campos.

// remotoDe abre el proveedor OAuth de un servicio.
func remotoDe(proveedor string, s Servicio, tok func(context.Context) (string, error), hc *http.Client) (Remoto, error) {
	a := apiJSON{Token: tok, HTTP: hc}
	switch proveedor + "/" + string(s) {
	case ProveedorGoogle + "/" + string(Calendario):
		return &GoogleCalendar{api: a, Base: "https://" + hostGoogleCalendar + "/calendar/v3"}, nil
	case ProveedorGoogle + "/" + string(Contactos):
		return &GooglePeople{api: a, Base: "https://" + hostGooglePeople + "/v1"}, nil
	case ProveedorMicrosoft + "/" + string(Calendario):
		return &GraphCalendario{api: a, Base: "https://" + hostGraph + "/v1.0"}, nil
	case ProveedorMicrosoft + "/" + string(Contactos):
		return &GraphContactos{api: a, Base: "https://" + hostGraph + "/v1.0"}, nil
	}
	return nil, ErrSinCuenta
}

type apiJSON struct {
	Token func(context.Context) (string, error)
	HTTP  *http.Client
}

// errorHTTP es una respuesta que no es un éxito, con su estado y un trozo del cuerpo.
type errorHTTP struct {
	Estado int
	Cuerpo string
}

func (e *errorHTTP) Error() string {
	return fmt.Sprintf("agenda: el proveedor respondió %d", e.Estado)
}

const maxJSON = 32 << 20

func (a apiJSON) pedir(ctx context.Context, metodo, u string, cuerpo any, cab map[string]string, out any) error {
	tok, err := a.Token(ctx)
	if err != nil {
		return err
	}
	var r io.Reader = http.NoBody
	if cuerpo != nil {
		b, err := json.Marshal(cuerpo)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, u, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range cab {
		req.Header.Set(k, v)
	}
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, maxJSON+1))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out != nil && len(bytes.TrimSpace(datos)) > 0 {
			return json.Unmarshal(datos, out)
		}
		return nil
	}
	return errorDeEstado(resp.StatusCode, datos)
}

// errorDeEstado es el error de una respuesta que no es un éxito: el *errorHTTP, junto al
// error de este paquete que la explica cuando lo hay.
func errorDeEstado(estado int, datos []byte) error {
	e := &errorHTTP{Estado: estado, Cuerpo: string(datos[:min(len(datos), 2048)])}
	switch estado {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.Join(ErrAcceso, e)
	case http.StatusNotFound:
		return errors.Join(ErrNoEsta, e)
	case http.StatusGone:
		return errors.Join(ErrFichaCaducada, e)
	case http.StatusPreconditionFailed, http.StatusConflict:
		return errors.Join(ErrConflicto, e)
	case http.StatusBadRequest:
		if strings.Contains(e.Cuerpo, "FAILED_PRECONDITION") {
			return errors.Join(ErrConflicto, e)
		}
		if strings.Contains(e.Cuerpo, "EXPIRED_SYNC_TOKEN") || strings.Contains(e.Cuerpo, "syncStateNotFound") {
			return errors.Join(ErrFichaCaducada, e)
		}
	}
	return e
}

// ----- iCalendar a mano -----

// textoICS escapa un TEXT de iCalendar (RFC 5545 §3.3.11).
func textoICS(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", "")
	return r.Replace(s)
}

// paramICS entrecomilla un valor de parámetro si hace falta.
func paramICS(s string) string {
	s = strings.NewReplacer("\"", "'", "\r", "", "\n", " ").Replace(s)
	if strings.ContainsAny(s, ":;,") {
		return `"` + s + `"`
	}
	return s
}

// lineas escribe un VCALENDAR o una vCard línea a línea.
type lineas struct{ b strings.Builder }

func (l *lineas) poner(nombre, params, valor string) {
	l.b.WriteString(nombre)
	if params != "" {
		l.b.WriteString(";" + params)
	}
	l.b.WriteString(":" + valor + "\r\n")
}

func (l *lineas) texto(nombre, valor string) {
	if strings.TrimSpace(valor) != "" {
		l.poner(nombre, "", textoICS(valor))
	}
}

// momento es una fecha (día completo) o un instante con su zona.
type momento struct {
	Dia  bool
	T    time.Time
	Zona string // IANA; vacía, UTC
}

func (m momento) ics() (params, valor string) {
	if m.Dia {
		return "VALUE=DATE", m.T.Format("20060102")
	}
	if m.Zona != "" && m.Zona != "UTC" {
		if loc, err := time.LoadLocation(m.Zona); err == nil {
			return "TZID=" + m.Zona, m.T.In(loc).Format("20060102T150405")
		}
	}
	return "", m.T.UTC().Format("20060102T150405Z")
}

// propLinea vuelve a escribir una propiedad de go-ical como línea («EXDATE;TZID=…:…»),
// la forma en que Google guarda la recurrencia.
func propLinea(p ical.Prop) string {
	var b strings.Builder
	b.WriteString(p.Name)
	claves := make([]string, 0, len(p.Params))
	for k := range p.Params {
		claves = append(claves, k)
	}
	sort.Strings(claves)
	for _, k := range claves {
		vs := make([]string, 0, len(p.Params[k]))
		for _, v := range p.Params[k] {
			vs = append(vs, paramICS(v))
		}
		b.WriteString(";" + k + "=" + strings.Join(vs, ","))
	}
	b.WriteString(":" + p.Value)
	return b.String()
}

// eventosDe separa el maestro y las excepciones de un VCALENDAR.
func eventosDe(datos []byte) (maestro *ical.Event, excepciones []ical.Event, err error) {
	cal, err := ical.NewDecoder(bytes.NewReader(datos)).Decode()
	if err != nil {
		return nil, nil, err
	}
	for _, ev := range cal.Events() {
		if ev.Props.Get(ical.PropRecurrenceID) != nil {
			excepciones = append(excepciones, ev)
			continue
		}
		if maestro == nil {
			e := ev
			maestro = &e
		}
	}
	if maestro == nil && len(excepciones) > 0 {
		return nil, nil, fmt.Errorf("%w: excepciones sin maestro", ErrNoSoportado)
	}
	if maestro == nil {
		return nil, nil, errors.New("agenda: el calendario no tiene eventos")
	}
	return maestro, excepciones, nil
}

// momentoDe lee DTSTART o DTEND (o el fin por DURATION) de un evento.
func momentoDe(ev ical.Event, nombre string) (momento, error) {
	p := ev.Props.Get(nombre)
	if p == nil && nombre == ical.PropDateTimeEnd {
		ini, err := momentoDe(ev, ical.PropDateTimeStart)
		if err != nil {
			return ini, err
		}
		fin, err := ev.DateTimeEnd(time.UTC)
		if err != nil || fin.IsZero() {
			if ini.Dia {
				ini.T = ini.T.AddDate(0, 0, 1)
			}
			return ini, nil
		}
		ini.T = fin
		return ini, nil
	}
	if p == nil {
		return momento{}, fmt.Errorf("agenda: el evento no tiene %s", nombre)
	}
	if p.ValueType() == ical.ValueDate {
		t, err := time.Parse("20060102", p.Value)
		return momento{Dia: true, T: t}, err
	}
	zona := p.Params.Get(ical.ParamTimezoneID)
	loc := time.UTC
	if zona != "" {
		if l, err := time.LoadLocation(zona); err == nil {
			loc = l
		} else {
			zona = ""
		}
	}
	t, err := p.DateTime(loc)
	return momento{T: t, Zona: zona}, err
}

func textoDe(ev ical.Event, nombre string) string {
	v, _ := ev.Props.Text(nombre)
	return v
}

// asistente es un ATTENDEE leído o por escribir.
type asistente struct {
	Correo, Nombre, Estado string // Estado: PARTSTAT
	Organiza               bool
}

func asistentesDe(ev ical.Event) []asistente {
	var out []asistente
	for _, p := range ev.Props.Values(ical.PropAttendee) {
		correo := strings.TrimPrefix(strings.TrimPrefix(p.Value, "mailto:"), "MAILTO:")
		if correo == "" {
			continue
		}
		out = append(out, asistente{Correo: correo, Nombre: p.Params.Get(ical.ParamCommonName), Estado: strings.ToUpper(p.Params.Get(ical.ParamParticipationStatus))})
	}
	return out
}

func (l *lineas) asistentes(as []asistente) {
	for _, a := range as {
		var ps []string
		if a.Nombre != "" {
			ps = append(ps, "CN="+paramICS(a.Nombre))
		}
		if a.Estado != "" {
			ps = append(ps, "PARTSTAT="+a.Estado)
		}
		l.poner("ATTENDEE", strings.Join(ps, ";"), "mailto:"+a.Correo)
	}
}

func (l *lineas) organizador(correo, nombre string) {
	if correo == "" {
		return
	}
	ps := ""
	if nombre != "" {
		ps = "CN=" + paramICS(nombre)
	}
	l.poner("ORGANIZER", ps, "mailto:"+correo)
}

// recurrenciaDe son las líneas RRULE, EXDATE, RDATE y EXRULE del maestro.
func recurrenciaDe(ev ical.Event) []string {
	var out []string
	for _, n := range []string{ical.PropRecurrenceRule, ical.PropExceptionDates, ical.PropRecurrenceDates, "EXRULE"} {
		for _, p := range ev.Props.Values(n) {
			out = append(out, propLinea(p))
		}
	}
	return out
}

// abrirVCALENDAR y cerrar envuelven los VEVENT de un objeto.
func abrirVCALENDAR(l *lineas) {
	l.poner("BEGIN", "", "VCALENDAR")
	l.poner("VERSION", "", "2.0")
	l.poner("PRODID", "", "-//Sytue//Sytue Office Calendar//ES")
}

func ordenarIDs(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
