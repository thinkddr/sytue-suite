// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendar

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
	calendarical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
)

// MaxImportarICS acota los eventos de un `.ics` importado de una vez: el mismo orden de
// magnitud que el tope de los feeds (DecodeICSFeed), que defiende de un fichero abusivo.
const MaxImportarICS = 10000

// ResultadoICS dice lo que entró de un `.ics` y lo que no: un evento con un UID que ya
// está en el calendario se omite (importar dos veces el mismo fichero no duplica), y uno
// ilegible o inválido falla sin tumbar el resto.
type ResultadoICS struct {
	Importados, Omitidos, Fallidos int
}

// ImportICS crea en calendar los eventos de un fichero iCalendar (RFC 5545), cada UID con
// su maestro y sus excepciones tal cual vienen (se conserva el VCALENDAR del objeto,
// como hace CalDAV). Es la importación «desde fichero» de Sytue Office (A-04): no hay
// red, ni invitaciones que mandar.
func (r *Repository) ImportICS(ctx context.Context, calendar string, datos []byte) (ResultadoICS, error) {
	objetos, err := calendarical.DecodeICSFeed(string(datos), MaxImportarICS)
	if err != nil {
		return ResultadoICS{}, err
	}
	var res ResultadoICS
	for _, po := range objetos {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		existe, err := r.tieneUID(ctx, calendar, po.UID)
		if err != nil {
			return res, err
		}
		if existe {
			res.Omitidos++
			continue
		}
		if err := r.importarObjeto(ctx, calendar, po); err != nil {
			res.Fallidos++
			continue
		}
		res.Importados++
	}
	return res, nil
}

func (r *Repository) tieneUID(ctx context.Context, calendar, uid string) (bool, error) {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return false, err
	}
	for _, row := range s.Rows() {
		if row.Kind != "object" || s.Deleted(row) {
			continue
		}
		var o struct {
			Calendar string `json:"calendar_id"`
			UID      string `json:"uid"`
		}
		if json.Unmarshal(row.JSON, &o) == nil && o.Calendar == calendar && o.UID == uid {
			return true, nil
		}
	}
	return false, nil
}

func (r *Repository) importarObjeto(ctx context.Context, calendar string, po calendarical.ParsedObject) error {
	data := po.Master.ICalData
	uid := po.UID
	if data == "" || uid == "" || len(uid) > 4096 || strings.ContainsAny(uid, "\r\n\x00") {
		return event.ErrInvalidEvent
	}
	_, err := r.mutate(ctx, "import_ics_event", struct {
		Calendar string
		UID      string
		Data     string
	}{calendar, uid, data}, func(s *model.Snapshot) (model.Aggregate, error) {
		if _, err := ownCalendar(s, calendar, true); err != nil {
			return model.Aggregate{}, err
		}
		id := uuid.NewString()
		now := time.Now().UTC()
		row, err := newRow(r.scope, "object", id, map[string]any{"calendar_id": calendar, "uid": uid, "resource_name": uuid.NewString() + ".ics", "component": "VEVENT", "data": data, "size_bytes": len(data), "etag": uuid.NewString(), "sequence_no": 0, "created_at": now, "updated_at": now})
		if err != nil {
			return model.Aggregate{}, err
		}
		return r.reproject(model.Aggregate{Version: 1, Scope: r.scope, Kind: "object_state", ID: id, Rows: []model.Row{row}}, data, now)
	})
	return err
}
