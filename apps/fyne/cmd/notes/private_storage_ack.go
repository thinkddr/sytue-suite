// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func (r *notepadsRepository) UpdateNoteAcknowledged(ctx context.Context, id, notebook string, in api.NoteInput, acknowledge func(context.Context, api.Note) error) (api.Note, error) {
	data, ok := r.notesRepository.(api.NotesAcknowledgedData)
	if !ok {
		return api.Note{}, api.ErrPrivateNotesOperation
	}
	return data.UpdateNoteAcknowledged(ctx, id, notebook, in, acknowledge)
}

func (r *notepadsRepository) ResumeNoteAcknowledged(ctx context.Context, id, notebook string, in api.NoteInput, acknowledge func(context.Context, api.Note) error) (api.Note, bool, error) {
	data, ok := r.notesRepository.(api.NotesAcknowledgedData)
	if !ok {
		return api.Note{}, false, api.ErrPrivateNotesOperation
	}
	return data.ResumeNoteAcknowledged(ctx, id, notebook, in, acknowledge)
}
