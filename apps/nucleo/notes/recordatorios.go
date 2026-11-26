// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

var _ api.NotesRemindersData = (*Repository)(nil)

// Un recordatorio es la fila `reminder` que exporta el servicio, una por nota y persona.
// Aquí solo se guarda: quien avisa a su hora es el sistema (§5.12, `local/avisos`), que
// lee ListReminders.

// normalizarRecordatorio son las reglas del servicio, compartidas en `notes/model`.
func normalizarRecordatorio(in api.NoteReminderInput) (api.NoteReminderInput, error) {
	var err error
	in.RRule, in.TZID, in.DueAt, err = model.NormalizarRecordatorio(in.RRule, in.TZID, in.DueAt, in.AfterDays)
	return in, err
}

// SetReminder pone (o rearma) mi recordatorio en una nota.
func (r *Repository) SetReminder(parent context.Context, nota string, in api.NoteReminderInput) (api.NoteReminder, error) {
	in, err := normalizarRecordatorio(in)
	if err != nil {
		return api.NoteReminder{}, err
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.NoteReminder{}, err
	}
	defer done()
	note, _, err := r.notaViva(ctx, nota)
	if err != nil {
		return api.NoteReminder{}, err
	}
	ahora := time.Now().UTC()
	if err := r.reescribir(ctx, "set-reminder", "reminder", idCompuesto(nota, r.cfg.Scope.Subject), map[string]any{
		"note_id": nota, "owner_sub": r.cfg.Scope.Subject, "due_at": in.DueAt, "dtstart": in.DueAt,
		"all_day": in.AllDay, "rrule": in.RRule, "after_days": in.AfterDays, "tzid": in.TZID,
		"fired_at": nil, "deleted_at": nil, "created_at": ahora,
	}, true); err != nil {
		return api.NoteReminder{}, err
	}
	return api.NoteReminder{NoteID: nota, NotebookID: note.NotebookID, DueAt: in.DueAt, AllDay: in.AllDay, RRule: in.RRule, AfterDays: in.AfterDays, TZID: in.TZID}, nil
}

// DeleteReminder quita mi recordatorio de una nota. Sin recordatorio no hace nada.
func (r *Repository) DeleteReminder(parent context.Context, nota string) error {
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	id := idCompuesto(nota, r.cfg.Scope.Subject)
	var actual struct {
		NoteID string `json:"note_id"`
	}
	ok, err := r.filaViva(ctx, "reminder", id, &actual)
	if err != nil || !ok {
		return err
	}
	return r.reescribir(ctx, "delete-reminder", "reminder", id, borrado(time.Now().UTC()), false)
}
