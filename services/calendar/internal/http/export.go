// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// Todo el tiempo que cabe en una fecha de Postgres con holgura: exportar no es
// mirar un rango, es llevárselo todo. Las series salen una vez, con su RRULE.
var (
	exportDesde = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	exportHasta = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
)

// Export entrega la agenda para «Exportar tus datos» (GET /api/v1/export): las
// tablas de calendarios y eventos, y un .ics por calendario que se importa tal
// cual en Google Calendar, Outlook o Apple Calendar.
func (h *Handlers) Export(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	p, err := portabilidad.Nuevo(w, r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}
	if err := h.exportar(r.Context(), sub, p); err != nil {
		_ = p.Fallo(err)
		return
	}
	_ = p.Cerrar()
}

func (h *Handlers) exportar(ctx context.Context, sub string, p *portabilidad.Paquete) error {
	cals, err := h.svc.ListCalendars(ctx, sub)
	if err != nil {
		return err
	}
	if err := portabilidad.Tabla(p, "calendars", "calendar", portabilidad.Todas(cals)); err != nil {
		return err
	}
	var eventos []domain.Event
	for _, c := range cals {
		objs, err := h.svc.ListObjects(ctx, c.ID)
		if err != nil {
			return err
		}
		datos := make([]string, len(objs))
		for i, o := range objs {
			datos[i] = o.Data
		}
		nombre := c.DisplayName
		if strings.TrimSpace(nombre) == "" {
			nombre = c.Name
		}
		ics := domain.UnirICS(nombre, datos)
		if err := p.Fichero("calendars/"+nombre+".ics", c.UpdatedAt, strings.NewReader(ics)); err != nil {
			return err
		}
		evs, err := h.svc.ListEventsInRange(ctx, c.ID, exportDesde, exportHasta)
		if err != nil {
			return err
		}
		eventos = append(eventos, evs...)
	}
	return portabilidad.Tabla(p, "events", "event", portabilidad.Todas(eventos))
}

// ExportCalendarICS entrega UN calendario como .ics (GET
// /api/v1/calendars/{id}/export.ics): el «Descargar .ics» del menú de un
// calendario, para llevárselo a otra app o dárselo a quien no está en la
// organización. Reusa la misma unión de objetos que «Tus datos»; a diferencia
// de ese, aquí sí hace falta el nombre para el Content-Disposition, así que
// primero se resuelve el calendario y solo después sus objetos — GetCalendar y
// ListObjects comparten el mismo requireCalendarRead, así que el segundo no
// repite el 403/404, pero tampoco cuesta nada de más.
func (h *Handlers) ExportCalendarICS(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	c, err := h.svc.GetCalendar(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	objs, err := h.svc.ListObjects(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	nombre := c.DisplayName
	if strings.TrimSpace(nombre) == "" {
		nombre = c.Name
	}
	datos := make([]string, len(objs))
	for i, o := range objs {
		datos[i] = o.Data
	}
	ics := domain.UnirICS(nombre, datos)

	// RFC 6266 (filename*): sin ella, un nombre con acentos o espacios llega
	// truncado en el primer carácter no ASCII a quien lo descarga con curl o con
	// un navegador antiguo; con las dos, el resto se queda con el ASCII simple
	// (filename) y los que la entienden usan el UTF-8 completo.
	nombreFichero := strings.ReplaceAll(nombre, `"`, "'") + ".ics"
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set(
		"Content-Disposition",
		`attachment; filename="`+nombreFichero+`"; filename*=UTF-8''`+url.PathEscape(nombreFichero),
	)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(ics))
}
