// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// NoteReminder es MI recordatorio en una nota (`domain.Reminder` de services/notes). DueAt
// es el instante; TZID, la zona en que el servicio calcula la repetición. Repite con una
// RRULE sin DTSTART o, «flexible», AfterDays días después de completar la tarea.
type NoteReminder struct {
	NoteID     string    `json:"note_id"`
	NotebookID string    `json:"notebook_id"`
	DueAt      time.Time `json:"due_at"`
	AllDay     bool      `json:"all_day"`
	RRule      string    `json:"rrule,omitempty"`
	AfterDays  int       `json:"after_days,omitempty"`
	TZID       string    `json:"tzid"`
}

// Repite dice si vuelve a sonar.
func (r NoteReminder) Repite() bool { return r.RRule != "" || r.AfterDays > 0 }

// NoteReminderInput es lo que se manda al ponerlo (`domain.ReminderInput`).
type NoteReminderInput struct {
	DueAt     time.Time `json:"due_at"`
	AllDay    bool      `json:"all_day"`
	RRule     string    `json:"rrule"`
	AfterDays int       `json:"after_days"`
	TZID      string    `json:"tzid"`
}

// ListReminders son mis recordatorios pendientes: el indicador de cada fila.
func (n *Notes) ListReminders(ctx context.Context) ([]NoteReminder, error) {
	r, err := JSON[struct {
		Reminders []NoteReminder `json:"reminders"`
	}](ctx, n.c, Request{Path: "/reminders"})
	return r.Reminders, err
}

// SetReminder pone (o rearma) mi recordatorio en una nota.
func (n *Notes) SetReminder(ctx context.Context, nota string, in NoteReminderInput) (NoteReminder, error) {
	cuerpo, err := json.Marshal(in)
	if err != nil {
		return NoteReminder{}, err
	}
	return JSON[NoteReminder](ctx, n.c, Request{
		Method: http.MethodPut, Path: "/notes/" + url.PathEscape(nota) + "/reminder", Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// DeleteReminder quita mi recordatorio de una nota.
func (n *Notes) DeleteReminder(ctx context.Context, nota string) error {
	return n.sinCuerpo(ctx, http.MethodDelete, "/notes/"+url.PathEscape(nota)+"/reminder", nil)
}
