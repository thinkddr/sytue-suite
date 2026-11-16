// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateNotesProbe struct {
	scope storage.Scope
	calls []string
	cause error
	input NoteInput
}

func (p *privateNotesProbe) PrivateStorageScope() storage.Scope { return p.scope }
func (p *privateNotesProbe) call(name string) error {
	p.calls = append(p.calls, name)
	return p.cause
}

func (p *privateNotesProbe) ListNotebooks(context.Context) ([]Notebook, error) {
	return []Notebook{{ID: "private", Title: "private notebook"}}, p.call("books")
}

func (p *privateNotesProbe) GetNotebook(context.Context, string) (Notebook, error) {
	return Notebook{}, p.call("book")
}

func (p *privateNotesProbe) CreateNotebook(context.Context, NotebookInput) (Notebook, error) {
	return Notebook{}, p.call("create-book")
}

func (p *privateNotesProbe) UpdateNotebook(context.Context, string, NotebookInput) (Notebook, error) {
	return Notebook{}, p.call("update-book")
}

func (p *privateNotesProbe) DeleteNotebook(context.Context, string) error {
	return p.call("delete-book")
}

func (p *privateNotesProbe) ListNotes(context.Context, string) ([]Note, error) {
	return nil, p.call("notes")
}

func (p *privateNotesProbe) CreateNote(_ context.Context, _ string, input NoteInput) (Note, error) {
	p.input = input
	return Note{}, p.call("create-note")
}

func (p *privateNotesProbe) UpdateNote(_ context.Context, _, _ string, input NoteInput) (Note, error) {
	p.input = input
	return Note{}, p.call("update-note")
}

func (p *privateNotesProbe) DeleteNote(context.Context, string) error { return p.call("delete-note") }

func (p *privateNotesProbe) RestoreNote(context.Context, string) error { return p.call("restore-note") }

func (p *privateNotesProbe) ListTrash(context.Context, int) ([]TrashedNote, error) {
	return nil, p.call("trash")
}

func (p *privateNotesProbe) Search(context.Context, string, int, bool) ([]NoteHit, error) {
	return nil, p.call("search")
}

func TestPrivateNotesFacadeDispatchesCoreDataWithoutTheOriginalTransport(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	original := NewNotes(server.URL, nil, server.Client())
	probe := &privateNotesProbe{scope: storage.Scope{Subject: "alice", Tenant: "acme", Product: "notes"}, cause: errors.New("private probe")}
	private, err := original.WithPrivateData(probe)
	if err != nil {
		t.Fatal(err)
	}
	checks := []func() error{
		func() error { _, e := private.ListNotebooks(t.Context()); return e },
		func() error { _, e := private.GetNotebook(t.Context(), "book"); return e },
		func() error { _, e := private.CreateNotebook(t.Context(), NotebookInput{}); return e },
		func() error { _, e := private.UpdateNotebook(t.Context(), "book", NotebookInput{}); return e },
		func() error { return private.DeleteNotebook(t.Context(), "book") },
		func() error { _, e := private.ListNotes(t.Context(), "book"); return e },
		func() error { _, e := private.ListNotesForEncryption(t.Context(), "book"); return e },
		func() error {
			_, e := private.CreateNote(t.Context(), "book", NoteInput{Title: "private title", Data: []byte(`{"precise":9007199254740993}`)})
			return e
		},
		func() error {
			_, e := private.UpdateNote(t.Context(), "note", "book", NoteInput{Title: "private title", Data: []byte(`{"precise":9007199254740993}`)})
			return e
		},
		func() error { return private.DeleteNote(t.Context(), "note") },
		func() error { return private.RestoreNote(t.Context(), "note") },
		func() error { _, e := private.ListTrash(t.Context(), 100); return e },
		func() error { _, e := private.Search(t.Context(), "private query", 100, false); return e },
	}
	for i, check := range checks {
		if err := check(); !errors.Is(err, probe.cause) {
			t.Fatal("private data was not selected", i, err)
		}
	}
	if len(probe.calls) != len(checks) || requests.Load() != 0 || probe.input.Title != "private title" || string(probe.input.Data) != `{"precise":9007199254740993}` {
		t.Fatal("data changed or reached old transport")
	}
	if _, err := original.ListNotebooks(t.Context()); err == nil || requests.Load() != 1 {
		t.Fatal("original client was replaced", err)
	}
}

func TestPrivateNotesFacadeDeniesRemainingAndFutureHTTPRoutes(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	probe := &privateNotesProbe{scope: storage.Scope{Subject: "alice", Tenant: "acme", Product: "notes"}}
	private, err := NewNotes(server.URL, nil, server.Client()).WithPrivateData(probe)
	if err != nil {
		t.Fatal(err)
	}
	checks := []func() error{
		func() error { _, e := private.Compartidas(t.Context()); return e },
		func() error { _, e := private.ListTags(t.Context()); return e },
		func() error { _, e := private.ListReminders(t.Context()); return e },
		func() error {
			_, e := private.DailyNotebook(t.Context(), "2026-10-30", "private date", false)
			return e
		},
		func() error {
			body, _, e := private.ExportNotebook(t.Context(), "book", "", "html")
			if body != nil {
				_ = body.Close()
			}
			return e
		},
		func() error { return private.PurgeNote(t.Context(), "note") },
		func() error { return private.Peticiones("book").Pedir(t.Context()) },
		func() error {
			response, e := private.c.Do(t.Context(), Request{Method: http.MethodPost, Path: "/future-content-route", Body: []byte("private")})
			if response != nil {
				_ = response.Body.Close()
			}
			return e
		},
	}
	for i, check := range checks {
		if err := check(); !errors.Is(err, ErrPrivateNotesOperation) {
			t.Fatal("unsupported operation did not fail closed", i, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("unsupported operation reached service")
	}
	for _, scope := range []storage.Scope{{}, {Subject: "alice", Tenant: "acme", Product: "mail"}} {
		if _, err := NewNotes(server.URL, nil, nil).WithPrivateData(&privateNotesProbe{scope: scope}); !errors.Is(err, ErrPrivateNotesOperation) {
			t.Fatal("foreign scope accepted", err)
		}
	}
	if _, err := private.WithPrivateData(probe); !errors.Is(err, ErrPrivateNotesOperation) {
		t.Fatal("private facade wrapped twice", err)
	}
}
