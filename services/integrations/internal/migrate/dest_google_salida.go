// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Correo y agenda hacia Google (salida por entregas, ver entregas.go).

// gmailImportPath importa un .eml TAL CUAL, con sus etiquetas: la fecha sale de su
// cabecera y no se filtra como spam (es correo de la propia persona).
const gmailImportPath = "/upload/gmail/v1/users/me/messages/import" +
	"?uploadType=multipart&internalDateSource=dateHeader&neverMarkSpam=true&processForCalendar=false"

// etiquetasDeSistema son los buzones de Sytue que Gmail ya tiene. Los borradores no
// están: la importación no deja crearlos, y van a su etiqueta como el resto.
var etiquetasDeSistema = map[string]string{"inbox": "INBOX", "sent": "SENT", "trash": "TRASH", "junk": "SPAM"}

// MailFolder devuelve la etiqueta de un buzón: la de sistema si la hay, y si no
// «Sytue/<ruta>», que Gmail anida por las barras.
func (g *googleDest) MailFolder(ctx context.Context, ruta, rol string) (string, error) {
	if id, ok := etiquetasDeSistema[rol]; ok {
		return id, nil
	}
	nombre := "Sytue/" + ruta
	cuerpo, _ := json.Marshal(map[string]string{
		"name": nombre, "labelListVisibility": "labelShow", "messageListVisibility": "show",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.gmailBase+"/gmail/v1/users/me/labels", bytes.NewReader(cuerpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var creada struct {
		ID string `json:"id"`
	}
	err = g.do(ctx, req, &creada)
	var se *suiteError
	if !errors.As(err, &se) || se.status != http.StatusConflict {
		return creada.ID, err
	}
	// Ya existía: la creó una pasada anterior cuyo mapa no llegó a guardarse.
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, g.gmailBase+"/gmail/v1/users/me/labels", http.NoBody)
	if err != nil {
		return "", err
	}
	var lista struct {
		Labels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := g.do(ctx, req, &lista); err != nil {
		return "", err
	}
	for _, l := range lista.Labels {
		if l.Name == nombre {
			return l.ID, nil
		}
	}
	return "", se
}

// ImportMessage importa el .eml en streaming con su etiqueta.
func (g *googleDest) ImportMessage(ctx context.Context, folderID string, _ int64, raw io.Reader) (string, error) {
	meta, _ := json.Marshal(map[string][]string{"labelIds": {folderID}})
	cuerpo, tipo := relacionado(meta, "message/rfc822", raw)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.gmailBase+gmailImportPath, cuerpo)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", tipo)
	var creado struct {
		ID string `json:"id"`
	}
	if err := g.do(ctx, req, &creado); err != nil {
		return "", err
	}
	return creado.ID, nil
}

// CreateCalendar crea un calendario secundario con el nombre del de Sytue: con
// calendar.app.created la aplicación solo toca los calendarios que ella crea.
func (g *googleDest) CreateCalendar(ctx context.Context, nombre string) (string, error) {
	cuerpo, _ := json.Marshal(map[string]string{"summary": nombre})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.calBase+"/calendar/v3/calendars", bytes.NewReader(cuerpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var creado struct {
		ID string `json:"id"`
	}
	if err := g.do(ctx, req, &creado); err != nil {
		return "", err
	}
	return creado.ID, nil
}

// ImportEvent importa un evento con su iCalUID: la importación no envía
// invitaciones a nadie y vuelve a importar el mismo sin duplicarlo. Las series van
// con su RRULE y sus EXDATE, como en Sytue.
//
//nolint:tagliatelle // contrato de la Calendar API de Google (camelCase).
func (g *googleDest) ImportEvent(ctx context.Context, calendarID string, ev EventoSalida) (string, error) {
	type momento struct {
		Date     string `json:"date,omitempty"`
		DateTime string `json:"dateTime,omitempty"`
		TimeZone string `json:"timeZone,omitempty"`
	}
	type asistenteGoogle struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName,omitempty"`
	}
	en := func(t time.Time) momento {
		if ev.AllDay {
			return momento{Date: t.UTC().Format("2006-01-02")}
		}
		return momento{DateTime: t.Format(time.RFC3339), TimeZone: ev.TZID}
	}
	evento := struct {
		ICalUID     string            `json:"iCalUID"`
		Summary     string            `json:"summary,omitempty"`
		Description string            `json:"description,omitempty"`
		Location    string            `json:"location,omitempty"`
		Start       momento           `json:"start"`
		End         momento           `json:"end"`
		Status      string            `json:"status,omitempty"`
		Recurrence  []string          `json:"recurrence,omitempty"`
		Attendees   []asistenteGoogle `json:"attendees,omitempty"`
	}{
		ICalUID: ev.UID, Summary: ev.Summary, Description: ev.Description, Location: ev.Location,
		Start: en(ev.DTStart), End: en(ev.DTEnd), Status: strings.ToLower(ev.Status),
	}
	if ev.RecurrenceID != nil {
		// Un cambio en una sola repetición va como evento suelto (la serie ya lo
		// excluye con su EXDATE), así que no puede compartir el iCalUID de la serie.
		evento.ICalUID = ev.UID + "-" + ev.RecurrenceID.UTC().Format("20060102T150405Z")
	} else if ev.RRule != "" || len(ev.RDate) > 0 {
		if ev.RRule != "" {
			evento.Recurrence = []string{"RRULE:" + ev.RRule}
		}
		fecha := func(prop string, x time.Time) string {
			if ev.AllDay {
				return prop + ";VALUE=DATE:" + x.UTC().Format("20060102")
			}
			return prop + ":" + x.UTC().Format("20060102T150405Z")
		}
		for _, x := range ev.RDate {
			evento.Recurrence = append(evento.Recurrence, fecha("RDATE", x))
		}
		for _, x := range ev.Excluidas() {
			evento.Recurrence = append(evento.Recurrence, fecha("EXDATE", x))
		}
	}
	for _, a := range ev.Attendees {
		evento.Attendees = append(evento.Attendees, asistenteGoogle{Email: a.Email, DisplayName: a.CommonName})
	}
	cuerpo, err := json.Marshal(evento)
	if err != nil {
		return "", err
	}
	u := g.calBase + "/calendar/v3/calendars/" + url.PathEscape(calendarID) + "/events/import"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(cuerpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var creado struct {
		ID string `json:"id"`
	}
	if err := g.do(ctx, req, &creado); err != nil {
		return "", err
	}
	return creado.ID, nil
}

var (
	_ MailExporter     = (*googleDest)(nil)
	_ CalendarExporter = (*googleDest)(nil)
)
