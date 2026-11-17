// SPDX-License-Identifier: Apache-2.0
package api

import "context"

// NotesAcknowledgedData retains the exact private mutation until its caller has
// durably acknowledged the source. It is optional for existing data providers;
// an unavailable private capability never falls back to plaintext HTTP.
type NotesAcknowledgedData interface {
	UpdateNoteAcknowledged(context.Context, string, string, NoteInput, func(context.Context, Note) error) (Note, error)
	ResumeNoteAcknowledged(context.Context, string, string, NoteInput, func(context.Context, Note) error) (Note, bool, error)
}

func (n *Notes) UpdateNoteAcknowledged(ctx context.Context, id, notebook string, in NoteInput, acknowledge func(context.Context, Note) error) (Note, error) {
	if err := in.ValidarEnlacesCifrados(); err != nil {
		return Note{}, err
	}
	if n == nil || n.data == nil || acknowledge == nil {
		return Note{}, ErrPrivateNotesOperation
	}
	data, ok := n.data.(NotesAcknowledgedData)
	if !ok {
		return Note{}, ErrPrivateNotesOperation
	}
	return data.UpdateNoteAcknowledged(ctx, id, notebook, in, acknowledge)
}

func (n *Notes) ResumeNoteAcknowledged(ctx context.Context, id, notebook string, in NoteInput, acknowledge func(context.Context, Note) error) (Note, bool, error) {
	if err := in.ValidarEnlacesCifrados(); err != nil {
		return Note{}, false, err
	}
	if n == nil || n.data == nil || acknowledge == nil {
		return Note{}, false, ErrPrivateNotesOperation
	}
	data, ok := n.data.(NotesAcknowledgedData)
	if !ok {
		return Note{}, false, ErrPrivateNotesOperation
	}
	return data.ResumeNoteAcknowledged(ctx, id, notebook, in, acknowledge)
}
