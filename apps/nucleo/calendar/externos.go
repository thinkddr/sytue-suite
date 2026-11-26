// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
	calendarical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
)

// Lo que necesita el sincronizador de cuentas externas (`local/agenda`, A-10): cada
// calendario de una cuenta (CalDAV, Google, Microsoft) es un calendario de aquí, y sus
// eventos se leen y se escriben como el iCalendar ENTERO de cada objeto, igual que los
// guarda CalDAV. Las escrituras pasan por la misma proyección que las de la pantalla
// (`reproject`), así que lo que entra se ve, se busca y avisa como lo creado a mano.
//
// A diferencia de lo que hace la persona, el sincronizador escribe también en los
// calendarios de SOLO LECTURA (los de una cuenta donde la persona solo puede ver): la
// pantalla los sigue sin poder tocar.

// ObjetoCrudo es un objeto de un calendario: un UID, con su maestro y sus excepciones.
type ObjetoCrudo struct {
	ID    string // el del objeto en este equipo
	UID   string
	Datos string // el VCALENDAR
}

// CrearCalendarioExterno da de alta el calendario de aquí que respalda uno de una
// cuenta. soloLectura: la persona no puede escribir en él.
func (r *Repository) CrearCalendarioExterno(ctx context.Context, nombre, color string, soloLectura bool) (api.Calendar, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		nombre = "Calendario"
	}
	if color == "" || event.SanitizeColor(color) != color {
		color = "#0E7C86"
	}
	change, err := r.mutate(ctx, "create_external_calendar", struct {
		Nombre, Color string
		SoloLectura   bool
	}{nombre, color, soloLectura}, func(_ *model.Snapshot) (model.Aggregate, error) {
		id := uuid.NewString()
		now := time.Now().UTC()
		row, err := newRow(r.scope, "calendar", id, map[string]any{"owner_sub": r.scope.Subject, "name": "externo-" + id[:8], "display_name": nombre, "color": color, "timezone": "UTC", "components": []string{"VEVENT"}, "ctag": uuid.NewString(), "sync_token": 1, "created_at": now, "updated_at": now, "is_default": false, "read_only": soloLectura})
		return model.Aggregate{Version: 1, Scope: r.scope, Kind: "calendar_state", ID: id, Rows: []model.Row{row}}, err
	})
	if err != nil {
		return api.Calendar{}, err
	}
	return calendarView(change.Rows[0])
}

// ActualizarCalendarioExterno pone el nombre, el color y si es de solo lectura según la
// cuenta (vacío no cambia).
func (r *Repository) ActualizarCalendarioExterno(ctx context.Context, id, nombre, color string, soloLectura bool) error {
	_, err := r.mutate(ctx, "update_external_calendar", struct {
		ID, Nombre, Color string
		SoloLectura       bool
	}{id, nombre, color, soloLectura}, func(s *model.Snapshot) (model.Aggregate, error) {
		row, err := ownCalendar(s, id, false)
		if err != nil {
			return model.Aggregate{}, err
		}
		var actual event.Calendar
		if json.Unmarshal(row.JSON, &actual) != nil {
			return model.Aggregate{}, model.ErrSnapshot
		}
		cambios := map[string]any{"read_only": soloLectura, "updated_at": time.Now().UTC(), "ctag": uuid.NewString(), "sync_token": actual.SyncToken + 1}
		if n := strings.TrimSpace(nombre); n != "" {
			cambios["display_name"] = n
		}
		if color != "" && event.SanitizeColor(color) == color {
			cambios["color"] = color
		}
		row, err = patch(row, cambios)
		return model.Aggregate{Version: 1, Scope: r.scope, Kind: "calendar_state", ID: id, Rows: []model.Row{row}}, err
	})
	return err
}

// ExisteCalendario dice si el calendario sigue (no se borró).
func (r *Repository) ExisteCalendario(ctx context.Context, id string) (bool, error) {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return false, err
	}
	_, err = ownCalendar(s, id, false)
	return err == nil, nil
}

// ObjetosCrudos son los objetos vivos de un calendario.
func (r *Repository) ObjetosCrudos(ctx context.Context, calendar string) ([]ObjetoCrudo, error) {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	var out []ObjetoCrudo
	for _, row := range s.Rows() {
		if row.Kind != "object" || s.Deleted(row) {
			continue
		}
		var o struct {
			Calendar string `json:"calendar_id"`
			UID      string `json:"uid"`
			Data     string `json:"data"`
		}
		if json.Unmarshal(row.JSON, &o) != nil {
			return nil, model.ErrSnapshot
		}
		if o.Calendar == calendar {
			out = append(out, ObjetoCrudo{ID: row.ID, UID: o.UID, Datos: o.Data})
		}
	}
	return out, nil
}

// objetoCrudo es un objeto vivo por su id.
func objetoCrudo(s *model.Snapshot, id string) (ObjetoCrudo, string, bool) {
	row, ok := s.Row("object", id)
	if !ok || s.Deleted(row) {
		return ObjetoCrudo{}, "", false
	}
	var o struct {
		Calendar string `json:"calendar_id"`
		UID      string `json:"uid"`
		Data     string `json:"data"`
	}
	if json.Unmarshal(row.JSON, &o) != nil {
		return ObjetoCrudo{}, "", false
	}
	return ObjetoCrudo{ID: id, UID: o.UID, Datos: o.Data}, o.Calendar, true
}

// ErrCambioLocal: el objeto cambió en este equipo desde que se leyó (la persona lo
// editó mientras se sincronizaba). No se pisa: el sincronizador lo verá la próxima vez.
var ErrCambioLocal = errors.New("calendar: el objeto cambió en este equipo")

// PonerObjetoCrudo guarda el iCalendar de un objeto en calendar: id "" lo crea (si ya
// hay un objeto con su UID en ese calendario, falla); si no, reemplaza el de ese id, que
// tiene que tener el mismo UID y, si esperado no es "", seguir con esos datos (si no,
// ErrCambioLocal). Devuelve el objeto tal como quedó guardado.
func (r *Repository) PonerObjetoCrudo(ctx context.Context, calendar, id, esperado, datos string) (ObjetoCrudo, error) {
	objetos, err := calendarical.DecodeICSFeed(datos, 2)
	if err != nil || len(objetos) != 1 {
		return ObjetoCrudo{}, event.ErrInvalidEvent
	}
	po := objetos[0]
	data, uid := po.Master.ICalData, po.UID
	if data == "" || uid == "" || len(uid) > 4096 || strings.ContainsAny(uid, "\r\n\x00") {
		return ObjetoCrudo{}, event.ErrInvalidEvent
	}
	var nuevo string
	_, err = r.mutate(ctx, "put_external_object", struct{ Calendar, ID, UID, Data, Esperado string }{calendar, id, uid, data, esperado}, func(s *model.Snapshot) (model.Aggregate, error) {
		if _, err := ownCalendar(s, calendar, false); err != nil {
			return model.Aggregate{}, err
		}
		now := time.Now().UTC()
		if id != "" {
			nuevo = id
			return r.reemplazarCrudo(s, calendar, id, uid, esperado, data, now)
		}
		if hayObjetoConUID(s, calendar, uid) {
			return model.Aggregate{}, event.ErrInvalidEvent
		}
		nuevo = uuid.NewString()
		return r.objetoNuevo(calendar, nuevo, uid, data, now)
	})
	if err != nil {
		return ObjetoCrudo{}, err
	}
	s, err := r.Snapshot(ctx)
	if err != nil {
		return ObjetoCrudo{}, err
	}
	o, _, ok := objetoCrudo(s, nuevo)
	if !ok {
		return ObjetoCrudo{}, missing()
	}
	return o, nil
}

// reemplazarCrudo es el objeto id de calendar con los datos nuevos, si sigue con el
// mismo UID y, con esperado, con esos datos (si no, ErrCambioLocal).
func (r *Repository) reemplazarCrudo(s *model.Snapshot, calendar, id, uid, esperado, data string, now time.Time) (model.Aggregate, error) {
	actual, cal, ok := objetoCrudo(s, id)
	if !ok || cal != calendar {
		return model.Aggregate{}, missing()
	}
	if esperado != "" && actual.Datos != esperado {
		return model.Aggregate{}, ErrCambioLocal
	}
	if actual.UID != uid {
		return model.Aggregate{}, event.ErrInvalidEvent
	}
	a, err := s.Edit("object_state", id)
	if err != nil {
		return a, err
	}
	return r.reproject(a, data, now)
}

// hayObjetoConUID: en calendar ya hay un objeto vivo con ese UID.
func hayObjetoConUID(s *model.Snapshot, calendar, uid string) bool {
	for _, row := range s.Rows() {
		if row.Kind != "object" || s.Deleted(row) {
			continue
		}
		var o struct {
			Calendar string `json:"calendar_id"`
			UID      string `json:"uid"`
		}
		if json.Unmarshal(row.JSON, &o) == nil && o.Calendar == calendar && o.UID == uid {
			return true
		}
	}
	return false
}

// objetoNuevo es el objeto id de calendar, con el UID uid y el iCalendar data, ya
// proyectado.
func (r *Repository) objetoNuevo(calendar, id, uid, data string, now time.Time) (model.Aggregate, error) {
	row, err := newRow(r.scope, "object", id, map[string]any{"calendar_id": calendar, "uid": uid, "resource_name": uuid.NewString() + ".ics", "component": "VEVENT", "data": data, "size_bytes": len(data), "etag": uuid.NewString(), "sequence_no": 0, "created_at": now, "updated_at": now})
	if err != nil {
		return model.Aggregate{}, err
	}
	return r.reproject(model.Aggregate{Version: 1, Scope: r.scope, Kind: "object_state", ID: id, Rows: []model.Row{row}}, data, now)
}

// aLaPapelera marca el objeto de `a` y sus eventos como borrados ahora (a la papelera,
// como el borrado de la pantalla).
func aLaPapelera(a model.Aggregate) (model.Aggregate, error) {
	now := time.Now().UTC()
	a.DeletedAt = &now
	for i, row := range a.Rows {
		if row.Kind == "object" || row.Kind == "event" {
			var err error
			if a.Rows[i], err = patch(row, map[string]any{"deleted_at": now, "updated_at": now}); err != nil {
				return a, err
			}
		}
	}
	return a, nil
}

// BorrarObjetoCrudo quita un objeto (a la papelera, como el borrado de la pantalla),
// también de un calendario de solo lectura, si sigue con los datos esperado ("" no lo
// mira; si cambió, ErrCambioLocal). Si ya no estaba, no es un error.
func (r *Repository) BorrarObjetoCrudo(ctx context.Context, id, esperado string) error {
	_, err := r.mutate(ctx, "delete_external_object", id, func(s *model.Snapshot) (model.Aggregate, error) {
		actual, _, ok := objetoCrudo(s, id)
		if !ok {
			return model.Aggregate{}, errYaBorrado
		}
		if esperado != "" && actual.Datos != esperado {
			return model.Aggregate{}, ErrCambioLocal
		}
		a, err := s.Edit("object_state", id)
		if err != nil {
			return a, err
		}
		return aLaPapelera(a)
	})
	if errors.Is(err, errYaBorrado) {
		return nil
	}
	return err
}

type errorSimple string

func (e errorSimple) Error() string { return string(e) }

const errYaBorrado = errorSimple("calendar: el objeto ya no está")
