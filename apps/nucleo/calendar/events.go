// SPDX-License-Identifier: Apache-2.0
package calendar

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/alarm"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
	calendarical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
)

func inputEvent(in api.EventInput, uid string) (event.Event, error) {
	if in.Start.IsZero() || in.End.Before(in.Start) || len(in.Summary) > 65536 || len(in.Attendees) > 1000 {
		return event.Event{}, event.ErrInvalidEvent
	}
	if in.TZID == "" {
		in.TZID = "UTC"
	}
	if _, err := time.LoadLocation(in.TZID); err != nil {
		return event.Event{}, event.ErrInvalidEvent
	}
	if in.Status == "" {
		in.Status = string(event.StatusConfirmed)
	}
	if in.Classification == "" {
		in.Classification = string(event.ClassPublic)
	}
	if in.Transparency == "" {
		in.Transparency = string(event.TranspOpaque)
	}
	ev := event.Event{UID: uid, Summary: in.Summary, Description: in.Description, Location: in.Location, Organizer: in.Organizer, OrganizerName: event.SanitizeCommonName(in.OrganizerName), DTStart: in.Start.UTC(), DTEnd: in.End.UTC(), TZID: in.TZID, AllDay: in.AllDay, Status: event.Status(in.Status), Classification: event.Classification(in.Classification), Transparency: event.Transparency(in.Transparency), Color: event.SanitizeColor(in.Color), ConferenceURL: event.SanitizeConferenceURL(in.ConferenceURL), RRule: in.RRule, RDate: in.RDate, ExDate: in.ExDate}
	if !event.ValidStatus(ev.Status) || !event.ValidClassification(ev.Classification) || !event.ValidTransparency(ev.Transparency) || ev.Color != in.Color || ev.ConferenceURL != in.ConferenceURL {
		return event.Event{}, event.ErrInvalidEvent
	}
	if ev.RRule != "" {
		if _, err := event.ParseRRule(ev.RRule); err != nil {
			return event.Event{}, err
		}
	}
	if !validReminderInput(in.Reminders) {
		return event.Event{}, event.ErrInvalidEvent
	}
	var err error
	ev.Attendees, err = normalizeAttendees(in.Attendees)
	if err != nil {
		return event.Event{}, err
	}
	return ev, nil
}

func editedData(in api.EventInput, uid, old string, sequence int) (string, error) {
	ev, err := inputEvent(in, uid)
	if err != nil {
		return "", err
	}
	ev.SequenceNo = sequence
	data, err := calendarical.EncodeEvent(ev, "")
	if err != nil {
		return "", err
	}
	if in.Reminders != nil {
		data, err = calendarical.ConRecordatorios(data, *in.Reminders, ev.Summary)
		if err != nil {
			return "", err
		}
	}
	data, err = calendarical.CarryOver(data, old, in.Reminders != nil)
	if err != nil {
		return "", err
	}
	return calendarical.PreserveExtensions(data, old)
}

func (r *Repository) CreateEvent(ctx context.Context, calendar string, in api.EventInput) (api.Event, error) {
	if in.Send {
		return api.Event{}, ErrPrivateIngress
	}
	change, err := r.mutate(ctx, "create_event", struct {
		Calendar string
		Input    api.EventInput
	}{calendar, in}, func(s *model.Snapshot) (model.Aggregate, error) {
		if _, err := ownCalendar(s, calendar, true); err != nil {
			return model.Aggregate{}, err
		}
		uid := in.UID
		if uid == "" {
			uid = uuid.NewString() + "@sytue"
		}
		if len(uid) > 4096 || strings.ContainsAny(uid, "\r\n\x00") {
			return model.Aggregate{}, event.ErrInvalidEvent
		}
		data, err := editedData(in, uid, "", 0)
		if err != nil {
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
	if err != nil {
		return api.Event{}, err
	}
	return aggregateEvent(change, "")
}

func (r *Repository) UpdateEvent(ctx context.Context, id string, in api.EventInput) (api.Event, error) {
	if in.Send {
		return api.Event{}, ErrPrivateIngress
	}
	change, err := r.mutate(ctx, "update_event", struct {
		ID    string
		Input api.EventInput
	}{id, in}, func(s *model.Snapshot) (model.Aggregate, error) {
		ev, err := findEvent(s, id)
		if err != nil {
			return model.Aggregate{}, err
		}
		if ev.RecurrenceID != nil {
			return model.Aggregate{}, event.ErrInvalidEvent
		}
		if _, err := ownCalendar(s, ev.CalendarID.String(), true); err != nil {
			return model.Aggregate{}, err
		}
		a, err := s.Edit("object_state", ev.ObjectID.String())
		if err != nil {
			return a, err
		}
		var old string
		for _, row := range a.Rows {
			if row.Kind == "object" {
				old, err = objectData(row)
				if err != nil {
					return a, err
				}
			}
		}
		data, err := editedData(in, ev.UID, old, ev.SequenceNo+1)
		if err != nil {
			return a, err
		}
		return r.reproject(a, data, time.Now().UTC())
	})
	if err != nil {
		return api.Event{}, err
	}
	return aggregateEvent(change, id)
}

func aggregateEvent(a model.Aggregate, id string) (api.Event, error) {
	var ev event.Event
	for _, row := range a.Rows {
		if row.Kind != "event" || rowDeleted(row) {
			continue
		}
		var candidate event.Event
		if json.Unmarshal(row.JSON, &candidate) != nil {
			return api.Event{}, model.ErrSnapshot
		}
		if id != "" && row.ID == id || id == "" && candidate.RecurrenceID == nil {
			ev = candidate
			break
		}
	}
	if ev.ID == uuid.Nil {
		return api.Event{}, model.ErrSnapshot
	}
	ev.Attendees = nil
	for _, row := range a.Rows {
		if row.Kind != "attendee" || rowDeleted(row) {
			continue
		}
		var value struct {
			event.Attendee
			Event string `json:"event_id"`
		}
		if json.Unmarshal(row.JSON, &value) != nil {
			return api.Event{}, model.ErrSnapshot
		}
		if value.Event == ev.ID.String() {
			ev.Attendees = append(ev.Attendees, value.Attendee)
		}
	}
	result, err := eventView(ev)
	if err != nil {
		return result, err
	}
	for _, row := range a.Rows {
		if row.Kind == "object" {
			data, err := objectData(row)
			if err != nil {
				return result, err
			}
			minutes := calendarical.Recordatorios(data)
			result.Reminders = &minutes
		}
	}
	return result, nil
}

func recurrenceKey(id *time.Time) string {
	if id == nil {
		return "master"
	}
	return id.UTC().Format(time.RFC3339Nano)
}

func eventFields(in event.Input, object string, now time.Time) map[string]any {
	return map[string]any{"object_id": object, "calendar_id": in.CalendarID.String(), "uid": in.UID, "summary": in.Summary, "description": in.Description, "location": in.Location, "organizer": in.Organizer, "organizer_name": in.OrganizerName, "dtstart": in.DTStart, "dtend": in.DTEnd, "tzid": in.TZID, "all_day": in.AllDay, "status": in.Status, "classification": in.Classification, "transp": in.Transparency, "color": in.Color, "conference_url": in.ConferenceURL, "rrule": in.RRule, "rdate": in.RDate, "exdate": in.ExDate, "recurrence_end": in.RecurrenceEnd, "recurrence_id": in.RecurrenceID, "sequence_no": in.SequenceNo, "updated_at": now, "deleted_at": nil}
}

// reproject keeps historical rows, IDs and unknown columns. Removed overrides,
// attendees and pending reminders become hidden; external invitation/outbox
// references therefore retain their original, valid event target.
func (r *Repository) reproject(a model.Aggregate, data string, now time.Time) (model.Aggregate, error) {
	var object event.CalendarObject
	objectIndex := -1
	for i, row := range a.Rows {
		if row.Kind == "object" {
			if json.Unmarshal(row.JSON, &object) != nil {
				return a, model.ErrSnapshot
			}
			objectIndex = i
		}
	}
	if objectIndex < 0 {
		return a, model.ErrSnapshot
	}
	parsed, err := calendarical.DecodeCalendarObject(data, object.CalendarID.String())
	if err != nil || parsed.UID != object.UID {
		return a, event.ErrInvalidEvent
	}
	parsed.Master.CalendarID = object.CalendarID
	a.Rows[objectIndex], err = patch(a.Rows[objectIndex], map[string]any{"data": data, "size_bytes": len(data), "etag": uuid.NewString(), "updated_at": now, "sequence_no": parsed.Master.SequenceNo})
	if err != nil {
		return a, err
	}
	existing := map[string]int{}
	for i, row := range a.Rows {
		if row.Kind != "event" || rowDeleted(row) {
			continue
		}
		var ev event.Event
		if json.Unmarshal(row.JSON, &ev) != nil {
			return a, model.ErrSnapshot
		}
		existing[recurrenceKey(ev.RecurrenceID)] = i
		a.Rows[i], err = patch(row, map[string]any{"deleted_at": now})
		if err != nil {
			return a, err
		}
	}
	for _, in := range append([]event.Input{parsed.Master}, parsed.Overrides...) {
		in.CalendarID = object.CalendarID
		fields := eventFields(in, a.ID, now)
		id := uuid.NewString()
		if i, ok := existing[recurrenceKey(in.RecurrenceID)]; ok {
			id = a.Rows[i].ID
			a.Rows[i], err = patch(a.Rows[i], fields)
		} else {
			var row model.Row
			fields["created_at"] = now
			row, err = newRow(r.scope, "event", id, fields)
			a.Rows = append(a.Rows, row)
		}
		if err != nil {
			return a, err
		}
		a, err = r.projectAttendees(a, id, in.Attendees, now)
		if err != nil {
			return a, err
		}
	}
	return r.projectReminders(a, parsed.Master, data, now)
}

func (r *Repository) projectAttendees(a model.Aggregate, eventID string, list []event.Attendee, now time.Time) (model.Aggregate, error) {
	existing := map[string]int{}
	for i, row := range a.Rows {
		if row.Kind != "attendee" || rowDeleted(row) {
			continue
		}
		var value struct {
			event.Attendee
			Event string `json:"event_id"`
		}
		if json.Unmarshal(row.JSON, &value) != nil {
			return a, model.ErrSnapshot
		}
		if value.Event != eventID {
			continue
		}
		existing[strings.ToLower(value.Email)] = i
		var err error
		a.Rows[i], err = patch(row, map[string]any{"deleted_at": now})
		if err != nil {
			return a, err
		}
	}
	for _, person := range list {
		fields := map[string]any{"event_id": eventID, "email": person.Email, "common_name": person.CommonName, "role": person.Role, "partstat": person.PartStat, "cutype": person.CUType, "rsvp": person.RSVP, "deleted_at": nil}
		var err error
		if i, ok := existing[strings.ToLower(person.Email)]; ok {
			a.Rows[i], err = patch(a.Rows[i], fields)
		} else {
			var row model.Row
			row, err = newRow(r.scope, "attendee", uuid.NewString(), fields)
			a.Rows = append(a.Rows, row)
		}
		if err != nil {
			return a, err
		}
	}
	return a, nil
}

func (r *Repository) projectReminders(a model.Aggregate, in event.Input, data string, now time.Time) (model.Aggregate, error) {
	for i, row := range a.Rows {
		if row.Kind != "reminder" || rowDeleted(row) {
			continue
		}
		var state struct {
			Fired     *time.Time `json:"fired_at"`
			Dismissed *time.Time `json:"dismissed_at"`
		}
		if json.Unmarshal(row.JSON, &state) != nil {
			return a, model.ErrSnapshot
		}
		if state.Fired == nil && state.Dismissed == nil {
			var err error
			a.Rows[i], err = patch(row, map[string]any{"deleted_at": now})
			if err != nil {
				return a, err
			}
		}
	}
	if in.Status == event.StatusCancelled {
		return a, nil
	}
	triggers, err := alarm.Triggers(data, in.DTStart, in.DTEnd)
	if err != nil {
		return a, err
	}
	masterID := ""
	for _, row := range a.Rows {
		if row.Kind == "event" && !rowDeleted(row) {
			var ev event.Event
			if json.Unmarshal(row.JSON, &ev) != nil {
				return a, model.ErrSnapshot
			}
			if ev.RecurrenceID == nil {
				masterID = row.ID
			}
		}
	}
	if masterID == "" {
		return a, event.ErrInvalidEvent
	}
	for _, trigger := range triggers {
		if !trigger.At.After(now) {
			continue
		}
		row, err := newRow(r.scope, "reminder", uuid.NewString(), map[string]any{"owner_sub": r.scope.Subject, "calendar_id": in.CalendarID.String(), "object_id": a.ID, "event_id": masterID, "uid": in.UID, "summary": in.Summary, "dtstart": in.DTStart, "trigger_at": trigger.At.UTC(), "action": trigger.Action, "created_at": now})
		if err != nil {
			return a, err
		}
		a.Rows = append(a.Rows, row)
	}
	return a, nil
}

func (r *Repository) DeleteEvent(ctx context.Context, id string) error {
	_, err := r.mutate(ctx, "delete_event", id, func(s *model.Snapshot) (model.Aggregate, error) {
		ev, err := findEvent(s, id)
		if err != nil {
			return model.Aggregate{}, err
		}
		if _, err := ownCalendar(s, ev.CalendarID.String(), true); err != nil {
			return model.Aggregate{}, err
		}
		a, err := s.Edit("object_state", ev.ObjectID.String())
		if err != nil {
			return a, err
		}
		now := time.Now().UTC()
		a.DeletedAt = &now
		for i, row := range a.Rows {
			if row.Kind == "object" || row.Kind == "event" {
				a.Rows[i], err = patch(row, map[string]any{"deleted_at": now, "updated_at": now})
				if err != nil {
					return a, err
				}
			}
		}
		return a, nil
	})
	return err
}

func (r *Repository) AmendRecurrence(ctx context.Context, calendar, uid string, occurrence time.Time, mode string) error {
	if occurrence.IsZero() || mode != api.RecorteExcluir && mode != api.RecorteHasta {
		return invalid()
	}
	_, err := r.mutate(ctx, "amend_recurrence", struct {
		Calendar, UID string
		Occurrence    time.Time
		Mode          string
	}{calendar, uid, occurrence, mode}, func(s *model.Snapshot) (model.Aggregate, error) {
		if _, err := ownCalendar(s, calendar, true); err != nil {
			return model.Aggregate{}, err
		}
		for _, row := range s.Rows() {
			if row.Kind != "object" || s.Deleted(row) {
				continue
			}
			var value event.CalendarObject
			if json.Unmarshal(row.JSON, &value) != nil {
				return model.Aggregate{}, model.ErrSnapshot
			}
			if value.CalendarID.String() != calendar || value.UID != uid {
				continue
			}
			a, err := s.Edit("object_state", row.ID)
			if err != nil {
				return a, err
			}
			data, err := objectData(row)
			if err != nil {
				return a, err
			}
			data, err = calendarical.AmendMasterRecurrence(data, occurrence, calendarical.AmendMode(mode))
			if err != nil {
				return a, err
			}
			return r.reproject(a, data, time.Now().UTC())
		}
		return model.Aggregate{}, missing()
	})
	return err
}

func validReminderInput(minutes *[]int) bool {
	if minutes != nil {
		if len(*minutes) > 100 {
			return false
		}
		for _, m := range *minutes {
			if m < 0 || m > 525600 {
				return false
			}
		}
	}

	return true
}

func normalizeAttendees(list []api.Attendee) ([]event.Attendee, error) {
	result := make([]event.Attendee, 0, len(list))
	seen := map[string]bool{}
	for _, a := range list {
		email := strings.TrimSpace(a.Email)
		if email == "" || strings.ContainsAny(email, "\r\n\x00") || seen[strings.ToLower(email)] {
			return nil, event.ErrInvalidEvent
		}
		seen[strings.ToLower(email)] = true
		if a.PartStat == "" {
			a.PartStat = string(event.PartNeedsAction)
		}
		if a.Role == "" {
			a.Role = string(event.RoleRequired)
		}
		if a.CUType == "" {
			a.CUType = "INDIVIDUAL"
		}
		if !event.ValidPartStat(event.ParticipationStatus(a.PartStat)) {
			return nil, event.ErrInvalidEvent
		}
		result = append(result, event.Attendee{Email: email, CommonName: a.CommonName, Role: event.AttendeeRole(a.Role), PartStat: event.ParticipationStatus(a.PartStat), CUType: a.CUType, RSVP: a.RSVP})
	}
	return result, nil
}
