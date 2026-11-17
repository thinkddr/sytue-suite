// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// NotesData is the initial personal data surface behind the existing native
// Notepads screen. Remaining methods are rejected by the facade until their
// private implementation is added; they never fall back to plaintext HTTP.
type NotesData interface {
	PrivateStorageScope() storage.Scope
	ListNotebooks(context.Context) ([]Notebook, error)
	GetNotebook(context.Context, string) (Notebook, error)
	CreateNotebook(context.Context, NotebookInput) (Notebook, error)
	UpdateNotebook(context.Context, string, NotebookInput) (Notebook, error)
	DeleteNotebook(context.Context, string) error
	ListNotes(context.Context, string) ([]Note, error)
	CreateNote(context.Context, string, NoteInput) (Note, error)
	UpdateNote(context.Context, string, string, NoteInput) (Note, error)
	DeleteNote(context.Context, string) error
	RestoreNote(context.Context, string) error
	ListTrash(context.Context, int) ([]TrashedNote, error)
	Search(context.Context, string, int, bool) ([]NoteHit, error)
}

var ErrPrivateNotesOperation = errors.New("notes: operation unavailable through private data")

// WithPrivateData returns a separate facade without inheriting the original
// client's session or transport. Use the original only for retained migration
// sources and non-content account controls. Scope is immutable for its lifetime.
func (n *Notes) WithPrivateData(data NotesData) (*Notes, error) {
	if n == nil || n.c == nil || data == nil || n.data != nil {
		return nil, ErrPrivateNotesOperation
	}
	scope := data.PrivateStorageScope()
	if !scope.Valid() || scope.Product != "notes" {
		return nil, ErrPrivateNotesOperation
	}
	return &Notes{c: privateDataClient(n.c, ErrPrivateNotesOperation), data: data}, nil
}

// NotesViewsData reads auxiliary personal views from the same authenticated
// private history. Implementations without these views remain fail-closed.
type NotesViewsData interface {
	PinnedNotes(context.Context) ([]NoteHit, error)
	ListTags(context.Context) ([]Tag, error)
	LinkedNotes(context.Context, string, string, int) ([]Note, error)
	Hashtags(context.Context) ([]Hashtag, error)
	ListReminders(context.Context) ([]NoteReminder, error)
}
