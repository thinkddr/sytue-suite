// SPDX-License-Identifier: Apache-2.0
package calendar

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
)

func (r *Repository) CreateCalendar(ctx context.Context, in api.CalendarInput) (api.Calendar, error) {
	if !event.ValidName(in.Name) {
		return api.Calendar{}, event.ErrInvalidName
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return api.Calendar{}, invalid()
	}
	if in.Color == "" {
		in.Color = "#6B3E9E"
	}
	if event.SanitizeColor(in.Color) != in.Color {
		return api.Calendar{}, invalid()
	}
	change, err := r.mutate(ctx, "create_calendar", in, func(_ *model.Snapshot) (model.Aggregate, error) {
		id := uuid.NewString()
		now := time.Now().UTC()
		row, err := newRow(r.scope, "calendar", id, map[string]any{"owner_sub": r.scope.Subject, "name": in.Name, "display_name": in.DisplayName, "color": in.Color, "timezone": in.Timezone, "components": []string{"VEVENT"}, "ctag": uuid.NewString(), "sync_token": 1, "created_at": now, "updated_at": now, "is_default": false, "read_only": false})
		return model.Aggregate{Version: 1, Scope: r.scope, Kind: "calendar_state", ID: id, Rows: []model.Row{row}}, err
	})
	if err != nil {
		return api.Calendar{}, err
	}
	return calendarView(change.Rows[0])
}

func (r *Repository) UpdateCalendar(ctx context.Context, id string, in api.CalendarInput) (api.Calendar, error) {
	if in.Timezone != "" {
		if _, err := time.LoadLocation(in.Timezone); err != nil {
			return api.Calendar{}, invalid()
		}
	}
	if in.Name != "" && !event.ValidName(in.Name) || in.Color != "" && event.SanitizeColor(in.Color) != in.Color {
		return api.Calendar{}, invalid()
	}
	change, err := r.mutate(ctx, "update_calendar", struct {
		ID    string
		Input api.CalendarInput
	}{id, in}, func(s *model.Snapshot) (model.Aggregate, error) {
		row, err := ownCalendar(s, id, false)
		if err != nil {
			return model.Aggregate{}, err
		}
		var current event.Calendar
		if json.Unmarshal(row.JSON, &current) != nil {
			return model.Aggregate{}, model.ErrSnapshot
		}
		fields := map[string]any{"updated_at": time.Now().UTC(), "ctag": uuid.NewString(), "sync_token": current.SyncToken + 1}
		for key, value := range map[string]string{"name": in.Name, "display_name": in.DisplayName, "color": in.Color, "timezone": in.Timezone} {
			if value != "" {
				fields[key] = value
			}
		}
		row, err = patch(row, fields)
		return model.Aggregate{Version: 1, Scope: r.scope, Kind: "calendar_state", ID: id, Rows: []model.Row{row}}, err
	})
	if err != nil {
		return api.Calendar{}, err
	}
	return calendarView(change.Rows[0])
}

func (r *Repository) DeleteCalendar(ctx context.Context, id string) error {
	_, err := r.mutate(ctx, "delete_calendar", id, func(s *model.Snapshot) (model.Aggregate, error) {
		row, err := ownCalendar(s, id, false)
		if err != nil {
			return model.Aggregate{}, err
		}
		var current event.Calendar
		if json.Unmarshal(row.JSON, &current) != nil {
			return model.Aggregate{}, model.ErrSnapshot
		}
		if current.IsDefault {
			return model.Aggregate{}, event.ErrDefaultImmutable
		}
		now := time.Now().UTC()
		row, err = patch(row, map[string]any{"deleted_at": now})
		return model.Aggregate{Version: 1, Scope: r.scope, Kind: "calendar_state", ID: id, Rows: []model.Row{row}, DeletedAt: &now}, err
	})
	return err
}
