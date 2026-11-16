// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateNotesAcknowledgedProbe struct {
	*privateNotesProbe
	resumed bool
}

func (p *privateNotesAcknowledgedProbe) UpdateNoteAcknowledged(ctx context.Context, id, book string, _ NoteInput, ack func(context.Context, Note) error) (Note, error) {
	n := Note{ID: id, NotebookID: book, UpdatedAt: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)}
	return n, ack(ctx, n)
}

func (p *privateNotesAcknowledgedProbe) ResumeNoteAcknowledged(ctx context.Context, id, book string, in NoteInput, ack func(context.Context, Note) error) (Note, bool, error) {
	p.resumed = true
	n, err := p.UpdateNoteAcknowledged(ctx, id, book, in, ack)
	return n, true, err
}

func TestPrivateNotesAcknowledgmentCapabilityNeverFallsBackToHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	base := NewNotes(server.URL, nil, server.Client())
	plain, err := base.WithPrivateData(&privateNotesProbe{scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}})
	if err != nil {
		t.Fatal(err)
	}
	callback := func(context.Context, Note) error { return nil }
	if _, err := plain.UpdateNoteAcknowledged(t.Context(), "n1", "b1", NoteInput{}, callback); !errors.Is(err, ErrPrivateNotesOperation) {
		t.Fatal("missing capability used ordinary update", err)
	}
	if _, _, err := plain.ResumeNoteAcknowledged(t.Context(), "n1", "b1", NoteInput{}, callback); !errors.Is(err, ErrPrivateNotesOperation) {
		t.Fatal("missing resume capability fell back", err)
	}
	if _, err := base.UpdateNoteAcknowledged(t.Context(), "n1", "b1", NoteInput{}, callback); !errors.Is(err, ErrPrivateNotesOperation) {
		t.Fatal("legacy client accepted a private acknowledgment", err)
	}
	if requests.Load() != 0 {
		t.Fatal("private acknowledgment leaked a request to HTTP", requests.Load())
	}
}

func TestPrivateNotesAcknowledgmentPassesExactResultAndCallbackFailure(t *testing.T) {
	probe := &privateNotesAcknowledgedProbe{privateNotesProbe: &privateNotesProbe{scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}}}
	client, err := NewNotes("", nil, nil).WithPrivateData(probe)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("local source acknowledgment failed")
	calls := 0
	callback := func(_ context.Context, n Note) error {
		calls++
		if n.ID != "n1" || n.NotebookID != "b1" || n.UpdatedAt.IsZero() {
			t.Fatal("acknowledgment result changed")
		}
		return failure
	}
	if _, err := client.UpdateNoteAcknowledged(t.Context(), "n1", "b1", NoteInput{}, callback); !errors.Is(err, failure) {
		t.Fatal("callback error was hidden", err)
	}
	if _, found, err := client.ResumeNoteAcknowledged(t.Context(), "n1", "b1", NoteInput{}, callback); !found || !probe.resumed || !errors.Is(err, failure) || calls != 2 {
		t.Fatal(found, err, calls)
	}
}
