// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/emersion/go-ical"

	"git.sytue.eu/sytue/one/apps/nucleo/eml"
)

// Las invitaciones de fuera en el lector (M-17), como la tarjeta de la web
// (`InvitacionCalendario.tsx`): un correo de Google, Outlook o Apple trae su evento en
// una parte `text/calendar` (iMIP, RFC 6047). Se resume —qué, cuándo, dónde, quién
// convoca— y se contesta Sí, Quizá o No:
//
//   - a quien organiza le llega el REPLY de iTIP (RFC 5546) por correo, desde la cuenta
//     que recibió la invitación, por la bandeja de salida;
//   - el evento, con esa respuesta, se abre en Sytue Calendar de este equipo (la app de
//     Mail no escribe en el calendario: se lo pasa como un `.ics`).

// Respuestas a una invitación (PARTSTAT).
const (
	RespuestaSi     = "ACCEPTED"
	RespuestaQuiza  = "TENTATIVE"
	RespuestaNo     = "DECLINED"
	metodoSolicitud = "REQUEST"
)

// ErrSinInvitacion: el mensaje no trae una invitación que se pueda contestar.
var ErrSinInvitacion = errors.New("correo: el mensaje no trae una invitación")

// Invitacion es el resumen de la invitación que trae un correo.
type Invitacion struct {
	UID                            string
	Resumen, Lugar, Descripcion    string
	Organizador, NombreOrganizador string
	Inicio, Fin                    time.Time
	TodoElDia                      bool
	// Metodo es el METHOD del calendario: REQUEST es una invitación que se contesta;
	// CANCEL o PUBLISH solo se enseñan.
	Metodo string
	// ICS es el calendario tal como llegó.
	ICS []byte
}

// Contestable dice si se ofrece Sí/Quizá/No.
func (i Invitacion) Contestable() bool {
	return (i.Metodo == "" || i.Metodo == metodoSolicitud) && i.Organizador != ""
}

// esCalendario dice si un adjunto es un iCalendar.
func esCalendario(a eml.Adjunto) bool {
	t := strings.ToLower(a.Tipo)
	return t == "text/calendar" || t == "application/ics" || strings.EqualFold(path.Ext(a.Nombre), ".ics")
}

// LeerInvitacion busca la invitación de un mensaje ya leído. Sin ninguna, false.
func LeerInvitacion(m eml.Mensaje) (Invitacion, bool) {
	for _, a := range m.Adjuntos {
		if !esCalendario(a) || len(a.Datos) == 0 {
			continue
		}
		if inv, err := decodificarInvitacion(a.Datos); err == nil {
			return inv, true
		}
	}
	return Invitacion{}, false
}

func decodificarInvitacion(datos []byte) (Invitacion, error) {
	cal, err := ical.NewDecoder(bytes.NewReader(datos)).Decode()
	if err != nil {
		return Invitacion{}, err
	}
	ev := evento(cal)
	if ev == nil {
		return Invitacion{}, ErrSinInvitacion
	}
	inv := Invitacion{ICS: datos}
	if p := cal.Props.Get(ical.PropMethod); p != nil {
		inv.Metodo = strings.ToUpper(strings.TrimSpace(p.Value))
	}
	texto := func(n string) string { t, _ := ev.Props.Text(n); return strings.TrimSpace(t) }
	inv.UID, inv.Resumen, inv.Lugar, inv.Descripcion = texto(ical.PropUID), texto(ical.PropSummary), texto(ical.PropLocation), texto(ical.PropDescription)
	if p := ev.Props.Get(ical.PropOrganizer); p != nil {
		inv.Organizador = direccionCal(p.Value)
		inv.NombreOrganizador = p.Params.Get(ical.ParamCommonName)
	}
	e := &ical.Event{Component: ev}
	if p := ev.Props.Get(ical.PropDateTimeStart); p != nil {
		inv.TodoElDia = p.ValueType() == ical.ValueDate
	}
	inv.Inicio, _ = e.DateTimeStart(time.Local)
	inv.Fin, _ = e.DateTimeEnd(time.Local)
	return inv, nil
}

// evento es el VEVENT principal: el que no es una excepción (sin RECURRENCE-ID), o el
// primero.
func evento(cal *ical.Calendar) *ical.Component {
	var primero *ical.Component
	for _, c := range cal.Children {
		if c.Name != ical.CompEvent {
			continue
		}
		if primero == nil {
			primero = c
		}
		if c.Props.Get(ical.PropRecurrenceID) == nil {
			return c
		}
	}
	return primero
}

// direccionCal es el correo de una CAL-ADDRESS (`mailto:ana@example.org`).
func direccionCal(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 7 && strings.EqualFold(v[:7], "mailto:") {
		v = v[7:]
	}
	return strings.ToLower(strings.TrimSpace(v))
}

// RespuestaInvitacion compone, para quien contesta (yo) con partstat:
//
//   - reply: el VCALENDAR METHOD:REPLY con el UID, la secuencia, las fechas y SOLO el
//     ATTENDEE de quien contesta, que es lo que pide RFC 5546 §3.2.3;
//   - copia: el calendario de la invitación sin METHOD y con el PARTSTAT de quien
//     contesta puesto (o añadido), para guardarlo en el calendario propio.
func RespuestaInvitacion(inv Invitacion, yo, nombre, partstat string, ahora time.Time) (reply, copia []byte, err error) {
	switch partstat {
	case RespuestaSi, RespuestaQuiza, RespuestaNo:
	default:
		return nil, nil, fmt.Errorf("correo: respuesta desconocida %q", partstat)
	}
	yo = strings.ToLower(strings.TrimSpace(yo))
	cal, err := ical.NewDecoder(bytes.NewReader(inv.ICS)).Decode()
	if err != nil {
		return nil, nil, err
	}
	ev := evento(cal)
	if ev == nil {
		return nil, nil, ErrSinInvitacion
	}

	// El REPLY.
	r := ical.NewCalendar()
	r.Props.SetText(ical.PropVersion, "2.0")
	r.Props.SetText(ical.PropProductID, "-//Sytue//Sytue Office Mail//ES")
	r.Props.SetText(ical.PropMethod, "REPLY")
	re := ical.NewComponent(ical.CompEvent)
	for _, n := range []string{
		ical.PropUID, ical.PropSequence, ical.PropRecurrenceID, ical.PropDateTimeStart, ical.PropDateTimeEnd,
		ical.PropDuration, ical.PropSummary, ical.PropOrganizer,
	} {
		for _, p := range ev.Props.Values(n) {
			re.Props.Add(&p)
		}
	}
	re.Props.SetDateTime(ical.PropDateTimeStamp, ahora.UTC())
	at := ical.NewProp(ical.PropAttendee)
	at.Value = "mailto:" + yo
	at.Params.Set(ical.ParamParticipationStatus, partstat)
	if nombre != "" {
		at.Params.Set(ical.ParamCommonName, nombre)
	}
	re.Props.Add(at)
	r.Children = append(r.Children, re)
	var out bytes.Buffer
	if err := ical.NewEncoder(&out).Encode(r); err != nil {
		return nil, nil, err
	}

	// La copia propia.
	cal.Props.Del(ical.PropMethod)
	for _, c := range cal.Children {
		if c.Name != ical.CompEvent {
			continue
		}
		ats := c.Props.Values(ical.PropAttendee)
		i := slices.IndexFunc(ats, func(p ical.Prop) bool { return direccionCal(p.Value) == yo })
		if i >= 0 {
			ats[i].Params.Set(ical.ParamParticipationStatus, partstat)
			ats[i].Params.Del(ical.ParamRSVP)
		} else {
			nuevo := ical.NewProp(ical.PropAttendee)
			nuevo.Value = "mailto:" + yo
			nuevo.Params.Set(ical.ParamParticipationStatus, partstat)
			c.Props.Add(nuevo)
		}
	}
	var cp bytes.Buffer
	if err := ical.NewEncoder(&cp).Encode(cal); err != nil {
		return nil, nil, err
	}
	return out.Bytes(), cp.Bytes(), nil
}

// asuntoRespuesta es el asunto del REPLY. Va en inglés, como lo escriben Google y
// Outlook para quien no comparte idioma: lo lee el calendario de quien organiza.
func asuntoRespuesta(partstat, resumen string) string {
	pre := map[string]string{RespuestaSi: "Accepted", RespuestaQuiza: "Tentative", RespuestaNo: "Declined"}[partstat]
	return pre + ": " + resumen
}

// componerRespuesta escribe el correo del REPLY: texto y `text/calendar; method=REPLY`
// en un `multipart/alternative`, como iMIP (RFC 6047 §2.4).
func componerRespuesta(de, deNombre, para, partstat string, inv Invitacion, reply []byte, ahora time.Time) ([]byte, error) {
	var b bytes.Buffer
	cab := func(n, v string) { b.WriteString(n + ": " + v + "\r\n") }
	desde := (&mail.Address{Name: deNombre, Address: de}).String()
	var id [12]byte
	_, _ = rand.Read(id[:])
	dominio := de[strings.LastIndex(de, "@")+1:]
	alt := multipart.NewWriter(&b)
	cab("From", desde)
	cab("To", (&mail.Address{Address: para}).String())
	cab("Subject", mime.QEncoding.Encode("utf-8", asuntoRespuesta(partstat, inv.Resumen)))
	cab("Date", ahora.Format(time.RFC1123Z))
	cab("Message-ID", "<"+hex.EncodeToString(id[:])+"@"+dominio+">")
	cab("MIME-Version", "1.0")
	cab("Content-Type", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": alt.Boundary()}))
	b.WriteString("\r\n")
	texto := asuntoRespuesta(partstat, inv.Resumen) + "\r\n"
	w, err := alt.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=utf-8"}, "Content-Transfer-Encoding": {"8bit"}})
	if err != nil {
		return nil, err
	}
	_, _ = w.Write([]byte(texto))
	w, err = alt.CreatePart(textproto.MIMEHeader{"Content-Type": {`text/calendar; charset=utf-8; method=REPLY`}, "Content-Transfer-Encoding": {"8bit"}})
	if err != nil {
		return nil, err
	}
	_, _ = w.Write(reply)
	if err := alt.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Invitacion lee la invitación de un mensaje del buzón.
func (b *Buzon) Invitacion(ctx context.Context, id string) (Invitacion, error) {
	rc, err := b.Raw(ctx, id)
	if err != nil {
		return Invitacion{}, err
	}
	defer rc.Close()
	m, err := eml.LeerConLimite(ctx, rc, eml.DefaultMessageByteLimit)
	if err != nil && len(m.Adjuntos) == 0 {
		return Invitacion{}, err
	}
	inv, ok := LeerInvitacion(m)
	if !ok {
		return Invitacion{}, ErrSinInvitacion
	}
	return inv, nil
}

// ResponderInvitacion contesta la invitación del mensaje id con partstat: el REPLY sale
// a quien organiza por la bandeja de salida, desde la cuenta que la recibió. Devuelve la
// copia del evento con la respuesta, para el calendario propio, y si el REPLY salió a la
// bandeja (sin organizador, o si no se pudo, false: la copia vale igual).
func (b *Buzon) ResponderInvitacion(ctx context.Context, id, partstat string) (copia []byte, enviado bool, err error) {
	n, err := leerID(id)
	if err != nil {
		return nil, false, err
	}
	s, err := b.x.sobre(ctx, n)
	if err != nil {
		return nil, false, err
	}
	inv, err := b.Invitacion(ctx, id)
	if err != nil {
		return nil, false, err
	}
	c, ok := b.reg.cuenta(s.cuenta)
	if !ok {
		if c, err = b.cuentaDe(""); err != nil {
			return nil, false, err
		}
	}
	ahora := time.Now()
	reply, copia, err := RespuestaInvitacion(inv, c.Direccion, c.Nombre, partstat, ahora)
	if err != nil {
		return nil, false, err
	}
	if !inv.Contestable() {
		return copia, false, nil
	}
	// Si el REPLY no sale, la copia vale igual: enviado=false lo dice, y la causa queda
	// en el registro.
	crudo, err := componerRespuesta(c.Direccion, c.Nombre, inv.Organizador, partstat, inv, reply, ahora)
	if err != nil {
		slog.Warn("correo: no se pudo componer la respuesta a la invitación", "error", err)
		return copia, false, nil
	}
	if _, err := b.Send(ctx, c.Direccion, []string{inv.Organizador}, crudo, 0); err != nil {
		slog.Warn("correo: la respuesta a la invitación no pasó a la bandeja de salida", "error", err)
		return copia, false, nil
	}
	return copia, true, nil
}
