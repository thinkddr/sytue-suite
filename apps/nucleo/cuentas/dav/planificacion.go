// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dav

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/google/uuid"
)

// Lo que el cliente sabe de la planificación de la persona (RFC 6638) y de sus libretas:
// las direcciones con que el servidor la conoce (`calendar-user-address-set`, la del
// ORGANIZER de lo que crea), la ocupación de otras personas por su buzón de salida
// (`schedule-outbox-URL` y una petición VFREEBUSY, §5; A-13) y crear una libreta con el
// MKCOL extendido (RFC 5689; RFC 6352 §6.3.1).

const nsCalDAVSched = nsCalDAV

// Planificacion es lo que el principal dice de la planificación de la persona.
type Planificacion struct {
	// Buzon es el `schedule-outbox-URL`; vacío, el servidor no planifica.
	Buzon string
	// Direcciones son las `mailto:` del `calendar-user-address-set`, sin «mailto:» y en
	// minúsculas, en el orden del servidor.
	Direcciones []string
}

// Planificacion lee del principal el buzón de salida y las direcciones de la persona.
func (c *Cliente) Planificacion(ctx context.Context) (Planificacion, error) {
	principal, _, _, err := c.Principal(ctx)
	if err != nil {
		return Planificacion{}, err
	}
	ms, desde, err := c.multiestado(ctx, "PROPFIND", principal, "0",
		`<d:propfind xmlns:d="DAV:" xmlns:c="`+nsCalDAVSched+`"><d:prop><c:schedule-outbox-URL/><c:calendar-user-address-set/></d:prop></d:propfind>`)
	if err != nil {
		return Planificacion{}, err
	}
	var out Planificacion
	for _, r := range ms.Responses {
		p, ok := r.props()
		if !ok {
			continue
		}
		if p.Outbox != nil && out.Buzon == "" {
			out.Buzon = c.resolver(desde, p.Outbox.Href)
		}
		if p.Direcciones != nil {
			for _, h := range p.Direcciones.Href {
				h = strings.TrimSpace(h)
				if len(h) > 7 && strings.EqualFold(h[:7], "mailto:") {
					d := strings.ToLower(strings.TrimSpace(h[7:]))
					if d != "" && !slices.Contains(out.Direcciones, d) {
						out.Direcciones = append(out.Direcciones, d)
					}
				}
			}
		}
	}
	return out, nil
}

// Tramo es un rato ocupado, en UTC.
type Tramo struct {
	Desde, Hasta time.Time
	// Provisional: BUSY-TENTATIVE (una invitación sin contestar, un «quizá»).
	Provisional bool
}

// ErrSinPlanificacion: el servidor no tiene buzón de salida (no planifica) y no sabe
// dar la ocupación de otras personas.
var ErrSinPlanificacion = errors.New("dav: el servidor no da la ocupación de otras personas")

// MaxOcupacion es cuántas personas se preguntan a la vez.
const MaxOcupacion = 50

// Ocupacion pregunta al servidor, por el buzón de salida de la persona (organizador), cuándo
// están ocupadas otras personas entre desde y hasta (RFC 6638 §5: un VFREEBUSY con
// METHOD:REQUEST). Devuelve sus ratos ocupados por dirección (en minúsculas): quien el
// servidor no conoce o no deja ver no está en el mapa.
func (c *Cliente) Ocupacion(ctx context.Context, buzon, organizador string, correos []string, desde, hasta time.Time) (map[string][]Tramo, error) {
	if buzon == "" {
		return nil, ErrSinPlanificacion
	}
	organizador = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(organizador, "mailto:"), "MAILTO:")))
	quienes := quienesOcupacion(correos)
	if len(quienes) == 0 {
		return map[string][]Tramo{}, nil
	}
	if organizador == "" || strings.ContainsAny(organizador, "\r\n,;:\"") {
		return nil, errors.New("dav: falta la dirección de quien pregunta")
	}
	peticionFB, destinos, err := pedirOcupacion(organizador, quienes, desde, hasta)
	if err != nil {
		return nil, err
	}
	// Originator y Recipient son del borrador anterior a la RFC: los servidores de
	// Apple aún los miran, y a los demás no les estorban.
	resp, datos, err := c.hacer(ctx, peticion{ //nolint:bodyclose // hacer ya leyó y cerró el cuerpo
		metodo: http.MethodPost, destino: buzon, cuerpo: peticionFB, tipo: "text/calendar; charset=utf-8",
		cabeceras: map[string]string{"Originator": "mailto:" + organizador, "Recipient": strings.Join(destinos, ", ")},
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, errorDe(resp, datos)
	}
	return leerOcupacion(datos, quienes)
}

// quienesOcupacion son las direcciones por las que se pregunta: en minúsculas, sin
// repetir, sin las que romperían la petición y como mucho MaxOcupacion.
func quienesOcupacion(correos []string) []string {
	var quienes []string
	for _, x := range correos {
		x = strings.ToLower(strings.TrimSpace(x))
		if x != "" && !strings.ContainsAny(x, "\r\n,;:\"") && !slices.Contains(quienes, x) && len(quienes) < MaxOcupacion {
			quienes = append(quienes, x)
		}
	}
	return quienes
}

// pedirOcupacion es el VCALENDAR con el VFREEBUSY (METHOD:REQUEST) que pregunta por
// quienes entre desde y hasta, y sus destinatarios («mailto:…»).
func pedirOcupacion(organizador string, quienes []string, desde, hasta time.Time) ([]byte, []string, error) {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//Sytue//Sytue Office Calendar//ES")
	cal.Props.SetText(ical.PropMethod, "REQUEST")
	fb := ical.NewComponent(ical.CompFreeBusy)
	fb.Props.SetText(ical.PropUID, uuid.NewString())
	fb.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	fb.Props.SetDateTime(ical.PropDateTimeStart, desde.UTC())
	fb.Props.SetDateTime(ical.PropDateTimeEnd, hasta.UTC())
	org := ical.NewProp(ical.PropOrganizer)
	org.Value = "mailto:" + organizador
	fb.Props.Add(org)
	destinos := make([]string, 0, len(quienes))
	for _, x := range quienes {
		a := ical.NewProp(ical.PropAttendee)
		a.Value = "mailto:" + x
		fb.Props.Add(a)
		destinos = append(destinos, "mailto:"+x)
	}
	cal.Children = append(cal.Children, fb)
	var b bytes.Buffer
	if err := ical.NewEncoder(&b).Encode(cal); err != nil {
		return nil, nil, err
	}
	return b.Bytes(), destinos, nil
}

// leerOcupacion lee la respuesta del buzón: los ratos ocupados de cada una de quienes
// que el servidor conoce y deja ver.
func leerOcupacion(datos []byte, quienes []string) (map[string][]Tramo, error) {
	var r struct {
		Respuestas []struct {
			Destino struct {
				Href string `xml:"DAV: href"`
			} `xml:"urn:ietf:params:xml:ns:caldav recipient"`
			Estado string `xml:"urn:ietf:params:xml:ns:caldav request-status"`
			Datos  string `xml:"urn:ietf:params:xml:ns:caldav calendar-data"`
		} `xml:"urn:ietf:params:xml:ns:caldav response"`
	}
	if err := xml.Unmarshal(datos, &r); err != nil {
		return nil, fmt.Errorf("%w: XML ilegible: %w", ErrServidor, err)
	}
	out := map[string][]Tramo{}
	for _, x := range r.Respuestas {
		quien := strings.ToLower(strings.TrimSpace(x.Destino.Href))
		if len(quien) > 7 && quien[:7] == "mailto:" {
			quien = quien[7:]
		}
		if !strings.HasPrefix(strings.TrimSpace(x.Estado), "2.") || !slices.Contains(quienes, quien) {
			continue
		}
		tramos, err := TramosVFREEBUSY(x.Datos)
		if err != nil {
			continue
		}
		out[quien] = tramos
	}
	return out, nil
}

// TramosVFREEBUSY lee los ratos ocupados de un VCALENDAR con un VFREEBUSY: cada FREEBUSY
// (los periodos «inicio/fin» o «inicio/duración» separados por comas) que no es FREE.
func TramosVFREEBUSY(datos string) ([]Tramo, error) {
	cal, err := ical.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil {
		return nil, err
	}
	out := []Tramo{}
	for _, comp := range cal.Children {
		if comp.Name != ical.CompFreeBusy {
			continue
		}
		for _, p := range comp.Props.Values(ical.PropFreeBusy) {
			tipo := strings.ToUpper(p.Params.Get(ical.ParamFreeBusyType))
			if tipo == "FREE" {
				continue
			}
			for _, periodo := range strings.Split(p.Value, ",") {
				ini, fin, ok := leerPeriodo(strings.TrimSpace(periodo))
				if ok && fin.After(ini) {
					out = append(out, Tramo{Desde: ini, Hasta: fin, Provisional: tipo == "BUSY-TENTATIVE"})
				}
			}
		}
	}
	slices.SortFunc(out, func(a, b Tramo) int { return a.Desde.Compare(b.Desde) })
	return out, nil
}

// leerPeriodo lee un PERIOD de iCalendar en UTC: «20260101T090000Z/20260101T100000Z» o
// «20260101T090000Z/PT1H».
func leerPeriodo(s string) (time.Time, time.Time, bool) {
	a, b, ok := strings.Cut(s, "/")
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	ini, err := time.Parse("20060102T150405Z", a)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	if fin, err := time.Parse("20060102T150405Z", b); err == nil {
		return ini, fin, true
	}
	p := ical.NewProp(ical.PropDuration)
	p.Value = b
	d, err := p.Duration()
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	return ini, ini.Add(d), true
}

// CrearLibreta crea una libreta en la casa de libretas de la cuenta con el MKCOL
// extendido (RFC 5689), con su nombre, y devuelve su URL. Como en CrearCalendario, la
// ruta es un UUID.
func (c *Cliente) CrearLibreta(ctx context.Context, casa, nombre string) (string, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return "", errors.New("dav: la libreta necesita un nombre")
	}
	destino := strings.TrimSuffix(casa, "/") + "/" + uuid.NewString() + "/"
	cuerpo := `<?xml version="1.0" encoding="utf-8"?><d:mkcol xmlns:d="DAV:" xmlns:card="` + nsCard + `"><d:set><d:prop>` +
		`<d:resourcetype><d:collection/><card:addressbook/></d:resourcetype><d:displayname>` + escapar(nombre) + `</d:displayname>` +
		`</d:prop></d:set></d:mkcol>`
	resp, datos, err := c.hacer(ctx, peticion{metodo: "MKCOL", destino: destino, cuerpo: []byte(cuerpo), tipo: "application/xml; charset=utf-8"}) //nolint:bodyclose // hacer ya leyó y cerró el cuerpo
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", errorDe(resp, datos)
	}
	return destino, nil
}
