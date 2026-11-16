// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"encoding/json"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

var _ api.NotesAcknowledgedData = (*Repository)(nil)

// UpdateNoteAcknowledged uses the shared mutation's authenticated result and
// retains that mutation until the original local draft can acknowledge it. A
// retry neither invents a content match nor weakens its optimistic precondition.
func (r *Repository) UpdateNoteAcknowledged(ctx context.Context, id, notebook string, in api.NoteInput, acknowledge func(context.Context, api.Note) error) (api.Note, error) {
	if id == "" || notebook == "" || acknowledge == nil {
		return api.Note{}, ErrSource
	}
	return r.saveNoteAcknowledged(ctx, id, notebook, in, acknowledge)
}

type noteMutationRequest struct {
	ID, Notebook string
	Input        api.NoteInput
	Expected     *time.Time
}

func (r *Repository) ResumeNoteAcknowledged(ctx context.Context, id, notebook string, in api.NoteInput, acknowledge func(context.Context, api.Note) error) (api.Note, bool, error) {
	if id == "" || notebook == "" || acknowledge == nil {
		return api.Note{}, false, ErrSource
	}
	raw, found, err := r.engine.ResumeAcknowledged(ctx, "update-note", noteMutationRequest{id, notebook, in, in.ExpectedUpdatedAt}, func(ctx context.Context, exact []byte) error {
		var accepted api.Note
		if json.Unmarshal(exact, &accepted) != nil || accepted.ID != id || accepted.NotebookID != notebook {
			return ErrSource
		}
		return acknowledge(ctx, accepted)
	})
	defer clear(raw)
	var result api.Note
	if err == nil && found {
		err = json.Unmarshal(raw, &result)
	}
	return result, found, err
}
