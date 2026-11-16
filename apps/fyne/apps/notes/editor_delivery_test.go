// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
)

type editorAcknowledgedData struct {
	privatePresenterData
	resume func(context.Context, string, string, api.NoteInput, func(context.Context, api.Note) error) (api.Note, bool, error)
}

func (d editorAcknowledgedData) UpdateNoteAcknowledged(context.Context, string, string, api.NoteInput, func(context.Context, api.Note) error) (api.Note, error) {
	panic("unexpected new mutation outside the admitted sender")
}

func (d editorAcknowledgedData) ResumeNoteAcknowledged(ctx context.Context, id, book string, in api.NoteInput, ack func(context.Context, api.Note) error) (api.Note, bool, error) {
	return d.resume(ctx, id, book, in, ack)
}

func TestPrivateEditorReplayRetainsNewerDraftThroughCallbackAndAPIFailure(t *testing.T) {
	p := draftRestoreScreen(t, true, nil)
	work := p.vida().work
	id := "edicion:replay"
	first := borradorEdicion{Version: 2, NoteID: "n1", NotebookID: "c1", Revision: "r1", FromInput: true, Input: api.NoteInput{Content: "first"}, ExpectedUpdatedAt: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)}
	newer := first.Clone()
	newer.Revision = "r2"
	newer.Input.Content = "newer"
	accepted := api.Note{ID: first.NoteID, NotebookID: first.NotebookID, UpdatedAt: first.ExpectedUpdatedAt.Add(time.Second)}
	if err := work.Write(t.Context(), notas.EspacioBorradores, id, first); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("records cleanup failed after local receipt")
	if err := p.enviarEdicionConfirmada(t.Context(), id, func(ctx context.Context, sent borradorEdicion, ack func(context.Context, api.Note) error) (api.Note, error) {
		if sent.Revision != "r1" {
			t.Fatal("sender did not retain original revision")
		}
		if err := work.Write(ctx, notas.EspacioBorradores, id, newer); err != nil {
			t.Fatal(err)
		}
		if err := ack(ctx, accepted); err != nil {
			t.Fatal(err)
		}
		return accepted, failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	retained, found, err := work.DurableEditor(t.Context(), id)
	if err != nil || !found || retained.Revision != "r2" || !retained.ExpectedUpdatedAt.Equal(first.ExpectedUpdatedAt) {
		t.Fatal("callback failure erased or prematurely rebased newer typing", found, err)
	}
	resumes := 0
	data := editorAcknowledgedData{privatePresenterData: privatePresenterData{scope: work.PrivateStorageScope()}, resume: func(ctx context.Context, note, book string, in api.NoteInput, ack func(context.Context, api.Note) error) (api.Note, bool, error) {
		resumes++
		if note != "n1" || book != "c1" || in.Content != "first" || in.ExpectedUpdatedAt == nil || !in.ExpectedUpdatedAt.Equal(first.ExpectedUpdatedAt) {
			t.Fatal("resume changed the exact original request")
		}
		return accepted, true, ack(ctx, accepted)
	}}
	p.svc, err = api.NewNotes("", nil, nil).WithPrivateData(data)
	if err != nil {
		t.Fatal(err)
	}
	sends := 0
	if err := p.enviarEdicionConfirmada(t.Context(), id, func(ctx context.Context, sent borradorEdicion, ack func(context.Context, api.Note) error) (api.Note, error) {
		sends++
		if sent.Revision != "r2" || sent.Input.Content != "newer" || sent.Input.ExpectedUpdatedAt == nil || !sent.Input.ExpectedUpdatedAt.Equal(accepted.UpdatedAt) {
			t.Fatal("newer request was sent before exact receipt replay")
		}
		next := accepted
		next.UpdatedAt = next.UpdatedAt.Add(time.Second)
		return next, ack(ctx, next)
	}); err != nil {
		t.Fatal(err)
	}
	if resumes != 1 || sends != 1 {
		t.Fatal("original request was republished instead of resumed", resumes, sends)
	}
	if pending, err := work.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("successful replay left hidden local work", pending, err)
	}
}

func TestPrivateEditorRequiresVerifiedCallbackBeforeSourceCleanup(t *testing.T) {
	p := draftRestoreScreen(t, true, nil)
	work := p.vida().work
	draft := borradorEdicion{Version: 2, NoteID: "n1", NotebookID: "c1", Revision: "r1", FromInput: true, Input: api.NoteInput{Content: "retained"}}
	id := "edicion:no-callback"
	if err := work.Write(t.Context(), notas.EspacioBorradores, id, draft); err != nil {
		t.Fatal(err)
	}
	if err := p.enviarEdicionConfirmada(t.Context(), id, func(context.Context, borradorEdicion, func(context.Context, api.Note) error) (api.Note, error) {
		return api.Note{ID: "n1", NotebookID: "c1", UpdatedAt: time.Now()}, nil
	}); !errors.Is(err, api.ErrPrivateNotesOperation) {
		t.Fatal("ordinary success erased an unverified source", err)
	}
	if pending, err := work.Pending(t.Context()); err != nil || pending.Count != 2 {
		t.Fatal("unverified request or original source disappeared", pending, err)
	}
}

func TestPrivateUnsafeLegacyDeliveryKeepsBothSources(t *testing.T) {
	p := draftRestoreScreen(t, true, nil)
	work := p.vida().work
	id := "edicion:unsafe"
	draft := borradorEdicion{Version: 1, NoteID: "n1", NotebookID: "c1", Revision: "r1", HTML: "pending", Input: api.NoteInput{CipherVersion: 2, ContentCipher: "encv1:synthetic", Links: []api.NoteLink{{TargetKind: "docs_document", TargetID: "private"}}}}
	if err := work.Write(t.Context(), notas.EspacioBorradores, id, draft); err != nil {
		t.Fatal(err)
	}
	if _, err := work.BeginEditorDelivery(t.Context(), id, draft); err != nil {
		t.Fatal(err)
	}
	called := false
	err := p.enviarEdicionConfirmada(t.Context(), id, func(context.Context, borradorEdicion, func(context.Context, api.Note) error) (api.Note, error) {
		called = true
		return api.Note{}, nil
	})
	if !errors.Is(err, api.ErrDatosCifradosNotas) || called {
		t.Fatal("unsafe delivery was sent", err)
	}
	if pending, err := work.Pending(t.Context()); err != nil || pending.Count != 2 {
		t.Fatal("unsafe delivery lost its source or journal", pending, err)
	}
}
