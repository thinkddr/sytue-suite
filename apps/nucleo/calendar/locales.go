// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendar

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
	calendarical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
)

// Rutas de Calendar que el catálogo privado sí puede servir en el dispositivo
// (PLAN-EDICION-LOCAL.md Anexo A, A-02 y A-03): responder por UID, la búsqueda con
// filtro y las suscripciones a un `.ics` por URL. Lo que pide red o identidad se
// enchufa desde fuera con Correo y Descargar; a cero, todo falla cerrado como antes.

var (
	_ api.CalendarUIDData    = (*Repository)(nil)
	_ api.CalendarFiltroData = (*Repository)(nil)
)

// MaxSuscripcionICS acota los eventos de un feed, como el servicio.
const MaxSuscripcionICS = 10000

// IntervaloSuscripcion es cada cuánto se propone volver a leer una suscripción.
const IntervaloSuscripcion = 6 * time.Hour

// SearchEventsFiltrada es SearchEventsPage con un calendario propio y un tramo
// [Desde, Hasta): un evento cuenta si se solapa con él y una serie si su recurrencia
// lo alcanza.
func (r *Repository) SearchEventsFiltrada(ctx context.Context, text string, f api.FiltroBusqueda, limit int, cursor string) (api.Busqueda, error) {
	if f == (api.FiltroBusqueda{}) {
		return r.SearchEventsPage(ctx, text, limit, cursor)
	}
	if !f.Desde.IsZero() && !f.Hasta.IsZero() && !f.Hasta.After(f.Desde) {
		return api.Busqueda{}, invalid()
	}
	s, err := r.Snapshot(ctx)
	if err != nil {
		return api.Busqueda{}, err
	}
	if f.Calendario != "" {
		if _, err := ownCalendar(s, f.Calendario, false); err != nil {
			return api.Busqueda{}, err
		}
	}
	text = buscar.Plegar(strings.TrimSpace(text))
	if len(text) < 2 {
		if cursor != "" {
			return api.Busqueda{}, invalid()
		}
		return api.Busqueda{Events: []api.Event{}}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	all, err := events(s, f.Calendario)
	if err != nil {
		return api.Busqueda{}, err
	}
	matches := []event.Event{}
	for _, ev := range matchingEvents(all, text) {
		if dentroDelTramo(ev, f.Desde, f.Hasta) {
			matches = append(matches, ev)
		}
	}
	clave := r.scope.Tenant + "\x00" + r.scope.Subject + "\x00" + text + "\x00" + f.Calendario + "\x00" + f.Desde.UTC().Format(time.RFC3339) + "\x00" + f.Hasta.UTC().Format(time.RFC3339)
	return paginar(matches, clave, limit, cursor)
}

// dentroDelTramo: un evento suelto se solapa con [desde, hasta); una serie empieza antes
// de que acabe el tramo y no ha terminado antes de que empiece.
func dentroDelTramo(ev event.Event, desde, hasta time.Time) bool {
	if !hasta.IsZero() && !ev.DTStart.Before(hasta) {
		return false
	}
	if desde.IsZero() {
		return true
	}
	if ev.IsRecurring() {
		return ev.RecurrenceEnd == nil || !ev.RecurrenceEnd.Before(desde)
	}
	fin := ev.DTEnd
	if fin.IsZero() {
		fin = ev.DTStart
	}
	return fin.After(desde) || fin.Equal(desde) && ev.DTStart.Equal(desde)
}

// paginar es la paginación por cursor de SearchEventsPage, con el cursor atado a la
// consulta (otra consulta no lo acepta).
func paginar(matches []event.Event, clave string, limit int, cursor string) (api.Busqueda, error) {
	sum := sha256.Sum256([]byte(clave))
	binding := hex.EncodeToString(sum[:]) + ":"
	start := 0
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(raw) > 256 || !strings.HasPrefix(string(raw), binding) {
			return api.Busqueda{}, invalid()
		}
		id := strings.TrimPrefix(string(raw), binding)
		found := false
		for i, ev := range matches {
			if ev.ID.String() == id {
				start, found = i+1, true
				break
			}
		}
		if !found {
			return api.Busqueda{}, invalid()
		}
	}
	result := api.Busqueda{Events: []api.Event{}}
	end := min(start+limit, len(matches))
	for _, ev := range matches[start:end] {
		v, err := eventView(ev)
		if err != nil {
			return api.Busqueda{}, err
		}
		result.Events = append(result.Events, v)
	}
	if end < len(matches) {
		result.Siguiente = base64.RawURLEncoding.EncodeToString([]byte(binding + matches[end-1].ID.String()))
	}
	return result, nil
}

// ResponderPorUID contesta a una invitación por el UID del evento: escribe el PARTSTAT
// del asistente que es la persona (Correo) en el propio `.ics` de cada objeto con ese
// UID en sus calendarios. No manda nada a nadie: avisar al organizador por su proveedor
// o por correo es A-11.
func (r *Repository) ResponderPorUID(ctx context.Context, uid, partstat string) error {
	if r.Correo == "" {
		return ErrPrivateIngress
	}
	if uid == "" || !partstatValido(partstat) {
		return invalid()
	}
	_, err := r.mutate(ctx, "reply_by_uid", struct{ UID, PartStat, Correo string }{uid, partstat, r.Correo}, func(s *model.Snapshot) (model.Aggregate, error) {
		for _, row := range s.Rows() {
			if row.Kind != "object" || s.Deleted(row) {
				continue
			}
			var o event.CalendarObject
			if json.Unmarshal(row.JSON, &o) != nil {
				return model.Aggregate{}, model.ErrSnapshot
			}
			if o.UID != uid {
				continue
			}
			if _, err := ownCalendar(s, o.CalendarID.String(), true); err != nil {
				continue
			}
			return r.responderObjeto(s, row, partstat)
		}
		return model.Aggregate{}, missing()
	})
	return err
}

// SetPartStat es la misma respuesta desde el id del evento.
func (r *Repository) SetPartStat(ctx context.Context, eventID, partstat string) error {
	if r.Correo == "" {
		return ErrPrivateIngress
	}
	if !partstatValido(partstat) {
		return invalid()
	}
	_, err := r.mutate(ctx, "set_partstat", struct{ ID, PartStat, Correo string }{eventID, partstat, r.Correo}, func(s *model.Snapshot) (model.Aggregate, error) {
		ev, err := findEvent(s, eventID)
		if err != nil {
			return model.Aggregate{}, err
		}
		if _, err := ownCalendar(s, ev.CalendarID.String(), true); err != nil {
			return model.Aggregate{}, err
		}
		row, ok := s.Row("object", ev.ObjectID.String())
		if !ok {
			return model.Aggregate{}, missing()
		}
		return r.responderObjeto(s, row, partstat)
	})
	return err
}

func partstatValido(p string) bool {
	switch strings.ToUpper(p) {
	case api.AsistenciaSi, api.AsistenciaQuiza, api.AsistenciaNo, "NEEDS-ACTION":
		return true
	}
	return false
}

func (r *Repository) responderObjeto(s *model.Snapshot, row model.Row, partstat string) (model.Aggregate, error) {
	data, err := objectData(row)
	if err != nil {
		return model.Aggregate{}, err
	}
	nuevo, invitado, err := calendarical.ResponderAsistente(data, r.Correo, partstat)
	if err != nil {
		return model.Aggregate{}, err
	}
	if !invitado {
		return model.Aggregate{}, missing()
	}
	a, err := s.Edit("object_state", row.ID)
	if err != nil {
		return a, err
	}
	return r.reproject(a, nuevo, time.Now().UTC())
}

// Subscribe da de alta un calendario de SOLO LECTURA respaldado por un `.ics` en una URL
// y lo lee ya. La URL es red que la persona pide (propósito `suscripcion-ics`): sin
// Descargar, falla cerrado.
func (r *Repository) Subscribe(ctx context.Context, in api.SubscriptionInput) (api.Calendar, error) {
	if r.Descargar == nil {
		return api.Calendar{}, ErrPrivateSubscriptions
	}
	in.URL = strings.TrimSpace(in.URL)
	if strings.HasPrefix(strings.ToLower(in.URL), "webcal://") {
		in.URL = "https://" + in.URL[len("webcal://"):]
	}
	if !event.ValidSubscriptionURL(in.URL) {
		return api.Calendar{}, invalid()
	}
	if in.Color == "" {
		in.Color = "#0E7C86"
	}
	if event.SanitizeColor(in.Color) != in.Color {
		return api.Calendar{}, invalid()
	}
	objetos, err := r.leerFeed(ctx, in.URL)
	if err != nil {
		return api.Calendar{}, err
	}
	nombre := strings.TrimSpace(in.DisplayName)
	if nombre == "" {
		if u, err := url.Parse(in.URL); err == nil {
			nombre = u.Hostname()
		}
	}
	ahora := time.Now().UTC()
	sub := event.Subscription{URL: in.URL, RefreshInterval: int(IntervaloSuscripcion / time.Second), LastFetchedAt: &ahora, NextRefreshAt: ahora.Add(IntervaloSuscripcion), LastStatus: "ok"}
	change, err := r.mutate(ctx, "subscribe_calendar", in, func(_ *model.Snapshot) (model.Aggregate, error) {
		id := uuid.NewString()
		row, err := newRow(r.scope, "calendar", id, map[string]any{"owner_sub": r.scope.Subject, "name": "suscripcion-" + id[:8], "display_name": nombre, "color": in.Color, "timezone": "UTC", "components": []string{"VEVENT"}, "ctag": uuid.NewString(), "sync_token": 1, "created_at": ahora, "updated_at": ahora, "is_default": false, "read_only": true, "subscription": sub})
		return model.Aggregate{Version: 1, Scope: r.scope, Kind: "calendar_state", ID: id, Rows: []model.Row{row}}, err
	})
	if err != nil {
		return api.Calendar{}, err
	}
	cal, err := calendarView(change.Rows[0])
	if err != nil {
		return cal, err
	}
	return cal, r.sincronizar(ctx, cal.ID, objetos)
}

// RefreshSubscription vuelve a leer ahora el `.ics` de una suscripción: entran los
// eventos nuevos, cambian los cambiados y desaparecen los que ya no están.
func (r *Repository) RefreshSubscription(ctx context.Context, calendar string) error {
	if r.Descargar == nil {
		return ErrPrivateSubscriptions
	}
	s, err := r.Snapshot(ctx)
	if err != nil {
		return err
	}
	row, err := ownCalendar(s, calendar, false)
	if err != nil {
		return err
	}
	var actual event.Calendar
	if json.Unmarshal(row.JSON, &actual) != nil {
		return model.ErrSnapshot
	}
	if actual.Subscription == nil || !event.ValidSubscriptionURL(actual.Subscription.URL) {
		return invalid()
	}
	sub := *actual.Subscription
	objetos, ferr := r.leerFeed(ctx, sub.URL)
	ahora := time.Now().UTC()
	sub.LastFetchedAt, sub.NextRefreshAt, sub.LastStatus, sub.LastError = &ahora, ahora.Add(IntervaloSuscripcion), "ok", ""
	if ferr != nil {
		sub.LastStatus, sub.LastError = "error", "fetch_failed"
	}
	if _, err := r.mutate(ctx, "refresh_subscription_state", struct {
		ID  string
		Sub event.Subscription
	}{calendar, sub}, func(s *model.Snapshot) (model.Aggregate, error) {
		row, err := ownCalendar(s, calendar, false)
		if err != nil {
			return model.Aggregate{}, err
		}
		row, err = patch(row, map[string]any{"subscription": sub, "updated_at": ahora})
		return model.Aggregate{Version: 1, Scope: r.scope, Kind: "calendar_state", ID: calendar, Rows: []model.Row{row}}, err
	}); err != nil {
		return err
	}
	if ferr != nil {
		return ferr
	}
	return r.sincronizar(ctx, calendar, objetos)
}

func (r *Repository) leerFeed(ctx context.Context, u string) ([]calendarical.ParsedObject, error) {
	datos, err := r.Descargar(ctx, u)
	if err != nil {
		return nil, err
	}
	return calendarical.DecodeICSFeed(string(datos), MaxSuscripcionICS)
}

// sincronizar deja el calendario (de solo lectura) con exactamente los objetos del feed.
func (r *Repository) sincronizar(ctx context.Context, calendar string, objetos []calendarical.ParsedObject) error {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return err
	}
	tenemos, err := objetosDelCalendario(s, calendar)
	if err != nil {
		return err
	}
	vienen := map[string]bool{}
	for _, po := range objetos {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, uid := po.Master.ICalData, po.UID
		if data == "" || uid == "" || len(uid) > 4096 || strings.ContainsAny(uid, "\r\n\x00") || vienen[uid] {
			continue
		}
		vienen[uid] = true
		r.ponerDelFeed(ctx, calendar, uid, data, tenemos)
	}
	for uid, row := range tenemos {
		if vienen[uid] {
			continue
		}
		if _, err := r.mutate(ctx, "subscription_delete", row.ID, func(s *model.Snapshot) (model.Aggregate, error) {
			a, err := s.Edit("object_state", row.ID)
			if err != nil {
				return a, err
			}
			return aLaPapelera(a)
		}); err != nil {
			return err
		}
	}
	return nil
}

// objetosDelCalendario son las filas de los objetos vivos de calendar, por su UID.
func objetosDelCalendario(s *model.Snapshot, calendar string) (map[string]model.Row, error) {
	tenemos := map[string]model.Row{}
	for _, row := range s.Rows() {
		if row.Kind != "object" || s.Deleted(row) {
			continue
		}
		var o event.CalendarObject
		if json.Unmarshal(row.JSON, &o) != nil {
			return nil, model.ErrSnapshot
		}
		if o.CalendarID.String() == calendar {
			tenemos[o.UID] = row
		}
	}
	return tenemos, nil
}

// ponerDelFeed guarda el objeto uid del feed: reemplaza el que ya estaba (si cambió) o
// lo crea. Un fallo no para la sincronización: el objeto se queda como estaba.
func (r *Repository) ponerDelFeed(ctx context.Context, calendar, uid, data string, tenemos map[string]model.Row) {
	if row, ok := tenemos[uid]; ok {
		if viejo, err := objectData(row); err == nil && viejo == data {
			return
		}
		_, _ = r.mutate(ctx, "subscription_update", struct{ Calendar, Object, Data string }{calendar, row.ID, data}, func(s *model.Snapshot) (model.Aggregate, error) {
			a, err := s.Edit("object_state", row.ID)
			if err != nil {
				return a, err
			}
			return r.reproject(a, data, time.Now().UTC())
		})
		return
	}
	_, _ = r.mutate(ctx, "subscription_create", struct{ Calendar, UID, Data string }{calendar, uid, data}, func(s *model.Snapshot) (model.Aggregate, error) {
		if _, err := ownCalendar(s, calendar, false); err != nil {
			return model.Aggregate{}, err
		}
		return r.objetoNuevo(calendar, uuid.NewString(), uid, data, time.Now().UTC())
	})
}
