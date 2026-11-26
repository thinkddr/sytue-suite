// SPDX-License-Identifier: Apache-2.0
package calendar

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
	calendarical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
)

func missing() error {
	return &api.Error{Status: http.StatusNotFound, Body: "private_calendar_resource_not_found"}
}

func invalid() error {
	return &api.Error{Status: http.StatusBadRequest, Body: "private_calendar_input_invalid"}
}

func ownCalendar(s *model.Snapshot, id string, writable bool) (model.Row, error) {
	row, ok := s.Row("calendar", id)
	if !ok || s.Deleted(row) {
		return model.Row{}, missing()
	}
	var value event.Calendar
	if json.Unmarshal(row.JSON, &value) != nil {
		return model.Row{}, model.ErrSnapshot
	}
	if value.OwnerSub != s.Scope().Subject {
		return model.Row{}, missing()
	}
	if writable && value.ReadOnly {
		return model.Row{}, event.ErrReadOnly
	}
	return row, nil
}

func calendarView(row model.Row) (api.Calendar, error) {
	var result api.Calendar
	if json.Unmarshal(row.JSON, &result) != nil {
		return result, model.ErrSnapshot
	}
	return result, nil
}

func (r *Repository) List(ctx context.Context) ([]api.Calendar, error) {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	result := []api.Calendar{}
	for _, row := range s.Rows() {
		if row.Kind != "calendar" || s.Deleted(row) {
			continue
		}
		if _, err := ownCalendar(s, row.ID, false); err != nil {
			continue
		}
		v, err := calendarView(row)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	slices.SortFunc(result, func(a, b api.Calendar) int {
		if a.IsDefault != b.IsDefault {
			if a.IsDefault {
				return -1
			}
			return 1
		}
		if n := strings.Compare(a.Name, b.Name); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	return result, nil
}

func events(s *model.Snapshot, calendar string) ([]event.Event, error) {
	result := []event.Event{}
	index := map[string]int{}
	for _, row := range s.Rows() {
		if row.Kind != "event" || s.Deleted(row) {
			continue
		}
		var ev event.Event
		if json.Unmarshal(row.JSON, &ev) != nil {
			return nil, model.ErrSnapshot
		}
		if calendar != "" && ev.CalendarID.String() != calendar {
			continue
		}
		if _, err := ownCalendar(s, ev.CalendarID.String(), false); err != nil {
			continue
		}
		ev.Attendees = nil
		index[row.ID] = len(result)
		result = append(result, ev)
	}
	for _, row := range s.Rows() {
		if row.Kind != "attendee" || s.Deleted(row) {
			continue
		}
		var a struct {
			event.Attendee
			Event string `json:"event_id"`
		}
		if json.Unmarshal(row.JSON, &a) != nil {
			return nil, model.ErrSnapshot
		}
		if i, ok := index[a.Event]; ok {
			result[i].Attendees = append(result[i].Attendees, a.Attendee)
		}
	}
	slices.SortFunc(result, func(a, b event.Event) int {
		if n := a.DTStart.Compare(b.DTStart); n != 0 {
			return n
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	return result, nil
}

func eventView(ev event.Event) (api.Event, error) {
	raw, err := json.Marshal(ev)
	if err != nil {
		return api.Event{}, err
	}
	defer clear(raw)
	var v api.Event
	if json.Unmarshal(raw, &v) != nil {
		return v, model.ErrSnapshot
	}
	return v, nil
}

func findEvent(s *model.Snapshot, id string) (event.Event, error) {
	all, err := events(s, "")
	if err != nil {
		return event.Event{}, err
	}
	for _, ev := range all {
		if ev.ID.String() == id {
			return ev, nil
		}
	}
	return event.Event{}, missing()
}

func objectData(row model.Row) (string, error) {
	var value struct {
		Data string `json:"data"`
	}
	if json.Unmarshal(row.JSON, &value) != nil || value.Data == "" {
		return "", model.ErrSnapshot
	}
	return value.Data, nil
}

func (r *Repository) GetEvent(ctx context.Context, id string) (api.Event, error) {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return api.Event{}, err
	}
	ev, err := findEvent(s, id)
	if err != nil {
		return api.Event{}, err
	}
	result, err := eventView(ev)
	if err != nil {
		return result, err
	}
	row, ok := s.Row("object", ev.ObjectID.String())
	if !ok {
		return result, model.ErrSnapshot
	}
	data, err := objectData(row)
	if err != nil {
		return result, err
	}
	reminders := calendarical.Recordatorios(data)
	result.Reminders = &reminders
	return result, nil
}

// EventICS es el recurso iCalendar del evento tal cual se guardó: «Descargar .ics».
func (r *Repository) EventICS(ctx context.Context, id string) ([]byte, error) {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	ev, err := findEvent(s, id)
	if err != nil {
		return nil, err
	}
	row, ok := s.Row("object", ev.ObjectID.String())
	if !ok {
		return nil, model.ErrSnapshot
	}
	data, err := objectData(row)
	if err != nil {
		return nil, err
	}
	return []byte(data), nil
}

func validWindow(from, to time.Time) bool {
	return !from.IsZero() && to.After(from) && to.Sub(from) <= 366*24*time.Hour
}

func (r *Repository) Instances(ctx context.Context, calendar string, from, to time.Time) ([]api.Instance, error) {
	if !validWindow(from, to) {
		return nil, event.ErrInvalidRange
	}
	s, err := r.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := ownCalendar(s, calendar, false); err != nil {
		return nil, err
	}
	all, err := events(s, calendar)
	if err != nil {
		return nil, err
	}
	result := []api.Instance{}
	for _, instance := range event.ExpandEvents(all, from, to) {
		v, err := eventView(instance.Event)
		if err != nil {
			return nil, err
		}
		result = append(result, api.Instance{Event: v, Start: instance.Start, End: instance.End})
	}
	return result, nil
}

func (r *Repository) SearchEvents(ctx context.Context, text string, limit int) ([]api.Event, error) {
	page, err := r.SearchEventsPage(ctx, text, limit, "")
	return page.Events, err
}

func (r *Repository) SearchEventsPage(ctx context.Context, text string, limit int, cursor string) (api.Busqueda, error) {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return api.Busqueda{}, err
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
	all, err := events(s, "")
	if err != nil {
		return api.Busqueda{}, err
	}
	matches := matchingEvents(all, text)

	sum := sha256.Sum256([]byte(r.scope.Tenant + "\x00" + r.scope.Subject + "\x00" + text))
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
				start = i + 1
				found = true
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

func (r *Repository) ExportICS(ctx context.Context, calendar string) ([]byte, error) {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	row, err := ownCalendar(s, calendar, false)
	if err != nil {
		return nil, err
	}
	v, err := calendarView(row)
	if err != nil {
		return nil, err
	}
	objects := []string{}
	for _, row := range s.Rows() {
		if row.Kind != "object" || s.Deleted(row) {
			continue
		}
		var rel struct {
			Calendar string `json:"calendar_id"`
		}
		if json.Unmarshal(row.JSON, &rel) != nil {
			return nil, model.ErrSnapshot
		}
		if rel.Calendar != calendar {
			continue
		}
		data, err := objectData(row)
		if err != nil {
			return nil, err
		}
		objects = append(objects, data)
	}
	return []byte(event.UnirICS(v.Title(), objects)), nil
}

func (r *Repository) SuggestSlots(ctx context.Context, from, to time.Time, minutes int, zone, start, end string, weekdays bool) ([]api.Hueco, error) {
	if !validWindow(from, to) || minutes <= 0 || minutes > 1440 {
		return nil, event.ErrInvalidRange
	}
	if zone == "" {
		zone = "UTC"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, invalid()
	}

	ws, err := parseClock(start)
	if err != nil {
		return nil, err
	}
	we, err := parseClock(end)
	if err != nil || we <= ws {
		return nil, invalid()
	}
	s, err := r.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	all, err := events(s, "")
	if err != nil {
		return nil, err
	}
	// UID is unique per calendar, so expand independently to keep overrides in
	// calendars with the same UID from suppressing each other's occurrences.
	byCalendar := map[string][]event.Event{}
	for _, ev := range all {
		id := ev.CalendarID.String()
		byCalendar[id] = append(byCalendar[id], ev)
	}
	busy := []event.FreeBusyPeriod{}
	for _, list := range byCalendar {
		busy = append(busy, event.FreeBusy(event.ExpandEvents(list, from, to), from, to)...)
	}
	slots := event.SuggestFreeSlots(busy, from, to, loc, event.WorkingHours{StartMin: ws, EndMin: we, WeekdaysOnly: weekdays}, time.Duration(minutes)*time.Minute, 50)
	result := make([]api.Hueco, len(slots))
	for i, v := range slots {
		result[i] = api.Hueco{Start: v.Start, End: v.End}
	}
	return result, nil
}

func matchingEvents(all []event.Event, text string) []event.Event {
	matches := []event.Event{}
	for _, ev := range all {
		if ev.RecurrenceID != nil {
			continue
		}
		terms := ev.Summary + " " + ev.Description + " " + ev.Location + " " + ev.Organizer
		for _, a := range ev.Attendees {
			terms += " " + a.Email + " " + a.CommonName
		}
		if strings.Contains(buscar.Plegar(terms), text) {
			matches = append(matches, ev)
		}
	}
	return matches
}

func parseClock(value string) (int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, invalid()
	}
	h, e := strconv.Atoi(parts[0])
	if e != nil || h < 0 || h > 23 {
		return 0, invalid()
	}
	m, e := strconv.Atoi(parts[1])
	if e != nil || m < 0 || m > 59 {
		return 0, invalid()
	}
	return h*60 + m, nil
}
