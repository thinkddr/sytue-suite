// SPDX-License-Identifier: Apache-2.0
package notas

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestEditorDeliveryRetainsOriginalBeforeSDKAndAcknowledgesOnlyExactSource(t *testing.T) {
	w := localWorkFixture(t)
	id := "edicion:delivery"
	first := editorDraftFixture("r1", "first")
	if err := w.journal.Write(w.Owner(), EspacioBorradores, id, first); err != nil {
		t.Fatal(err)
	}
	delivery, err := w.BeginEditorDelivery(t.Context(), id, first)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.AcceptedAt.IsZero() == false {
		t.Fatal("begin invented acceptance")
	}
	if err := w.FinishEditorDelivery(t.Context(), delivery); !errors.Is(err, ErrEditorDraft) {
		t.Fatal("unaccepted request was erased", err)
	}
	newer := editorDraftFixture("r2", "newer")
	if err := w.journal.Write(w.Owner(), EspacioBorradores, id, newer); err != nil {
		t.Fatal(err)
	}
	updated := api.Note{ID: "n1", NotebookID: "c1", UpdatedAt: first.ExpectedUpdatedAt.Add(time.Second)}
	if err := w.MarkEditorDelivery(t.Context(), delivery, updated); err != nil {
		t.Fatal(err)
	}
	var still EditorDraft
	if found, err := w.journal.Read(w.Owner(), EspacioBorradores, id, &still); err != nil || !found || still.Revision != "r2" || !still.ExpectedUpdatedAt.Equal(first.ExpectedUpdatedAt) {
		t.Fatal("callback changed the source before records cleanup succeeded", found, err)
	}
	retained, err := w.EditorDeliveries(t.Context(), id)
	if err != nil || len(retained) != 1 || retained[0].Draft.Revision != "r1" || retained[0].Draft.EditorHTML() != "first" || retained[0].AcceptedAt.IsZero() {
		t.Fatal("original request or acceptance proof lost", err)
	}
	if err := w.FinishEditorDelivery(t.Context(), delivery); err != nil {
		t.Fatal(err)
	}
	if found, err := w.journal.Read(w.Owner(), EspacioBorradores, id, &still); err != nil || !found || still.Revision != "r2" || still.EditorHTML() != "newer" || !still.ExpectedUpdatedAt.Equal(updated.UpdatedAt) {
		t.Fatal("finishing v1 erased or failed to rebase v2", found, err)
	}
	if list, err := w.EditorDeliveries(t.Context(), id); err != nil || len(list) != 0 {
		t.Fatal("completed receipt remained", err)
	}
	if err := w.FinishEditorDelivery(t.Context(), delivery); err != nil {
		t.Fatal("cleanup was not idempotent", err)
	}
}

func TestEditorDeliverySurvivesFreshControllerAndRejectsChangedRequestOrReceipt(t *testing.T) {
	w := localWorkFixture(t)
	original := editorDraftFixture("r1", "accepted exactly")
	id := "edicion:stable"
	if err := w.journal.Write(w.Owner(), EspacioBorradores, id, original); err != nil {
		t.Fatal(err)
	}
	delivery, err := w.BeginEditorDelivery(t.Context(), id, original)
	if err != nil {
		t.Fatal(err)
	}
	altered := original
	altered.Input.Content = "different"
	if _, err := w.BeginEditorDelivery(t.Context(), id, altered); !errors.Is(err, ErrEditorDraft) {
		t.Fatal("same revision accepted different source", err)
	}
	if err := w.MarkEditorDelivery(t.Context(), delivery, api.Note{ID: "other", NotebookID: "c1", UpdatedAt: time.Now()}); !errors.Is(err, ErrEditorDraft) {
		t.Fatal("foreign acceptance was stored", err)
	}
	accepted := api.Note{ID: "n1", NotebookID: "c1", UpdatedAt: original.ExpectedUpdatedAt.Add(time.Second)}
	if err := w.MarkEditorDelivery(t.Context(), delivery, accepted); err != nil {
		t.Fatal(err)
	}
	changed := accepted
	changed.UpdatedAt = changed.UpdatedAt.Add(time.Second)
	if err := w.MarkEditorDelivery(t.Context(), delivery, changed); !errors.Is(err, ErrEditorDraft) {
		t.Fatal("a different accepted revision replaced the proof", err)
	}
	// Reopen the actual encrypted store through a new controller without any RAM receipt.
	fresh, err := NewLocalWork(t.Context(), w.scope, w.journal, w.acquire)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Close)
	list, err := fresh.EditorDeliveries(t.Context(), id)
	if err != nil || len(list) != 1 || !list[0].AcceptedAt.Equal(accepted.UpdatedAt) {
		t.Fatal("fresh controller lost exact durable acceptance", err)
	}
	if err := fresh.FinishEditorDelivery(t.Context(), list[0]); err != nil {
		t.Fatal(err)
	}
	if pending, err := fresh.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("successful recovery left invisible or duplicate source work", pending, err)
	}
}

func TestEditorDeliveryFailedReceiptAndCleanupKeepDurableRecoverySource(t *testing.T) {
	w := localWorkFixture(t)
	draft := editorDraftFixture("r1", "retained across failures")
	id := "edicion:failure"
	if err := w.journal.Write(w.Owner(), EspacioBorradores, id, draft); err != nil {
		t.Fatal(err)
	}
	delivery, err := w.BeginEditorDelivery(t.Context(), id, draft)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	accepted := api.Note{ID: "n1", NotebookID: "c1", UpdatedAt: draft.ExpectedUpdatedAt.Add(time.Second)}
	if err := w.MarkEditorDelivery(canceled, delivery, accepted); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	list, err := w.EditorDeliveries(t.Context(), id)
	if err != nil || len(list) != 1 || !list[0].AcceptedAt.IsZero() {
		t.Fatal("failed receipt became proof of acceptance", err)
	}
	if err := w.MarkEditorDelivery(t.Context(), delivery, accepted); err != nil {
		t.Fatal(err)
	}
	if err := w.FinishEditorDelivery(canceled, delivery); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var current EditorDraft
	if found, err := w.journal.Read(w.Owner(), EspacioBorradores, id, &current); err != nil || !found || current.Revision != "r1" {
		t.Fatal("failed cleanup erased original source", found, err)
	}
	if pending, err := w.Pending(t.Context()); err != nil || pending.Count != 2 {
		t.Fatal("recovery records were lost or invisible", pending, err)
	}
	if err := w.FinishEditorDelivery(t.Context(), delivery); err != nil {
		t.Fatal(err)
	}
	if pending, err := w.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal(pending, err)
	}
}

func TestEditorDeliveryCannotStartAfterPresenterRetirement(t *testing.T) {
	w := localWorkFixture(t)
	w.Stop()
	if _, err := w.BeginEditorDelivery(t.Context(), "edicion:old", editorDraftFixture("r1", "unadmitted")); !errors.Is(err, ErrLocalAdmission) {
		t.Fatal("retired presenter started another delivery", err)
	}
	if pending, err := w.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("retired source acquired new work", pending, err)
	}
}

func TestEditorDeliveryRequiresOriginalSessionLeaseBeforeIntent(t *testing.T) {
	w := localWorkFixture(t)
	rejected := errors.New("original session was revoked")
	w.acquire = func(context.Context) (func(), error) { return nil, rejected }
	if _, err := w.BeginEditorDelivery(t.Context(), "edicion:revoked", editorDraftFixture("r1", "not admitted")); !errors.Is(err, rejected) {
		t.Fatal("delivery bypassed the original session witness", err)
	}
	if pending, err := w.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("revoked session wrote an intent", pending, err)
	}
}
