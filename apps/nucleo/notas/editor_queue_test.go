// SPDX-License-Identifier: Apache-2.0
package notas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func editorDraftFixture(revision, body string) EditorDraft {
	return EditorDraft{Version: 2, NoteID: "n1", NotebookID: "c1", Revision: revision, FromInput: true, Input: api.NoteInput{Kind: "text", Content: body}, ExpectedUpdatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
}

func TestEditorQueueAdmitsRAMWithLeaseAndDrainWaitsForDurableSource(t *testing.T) {
	w := localWorkFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	var releases atomic.Int32
	w.acquire = func(context.Context) (func(), error) { return func() { releases.Add(1) }, nil }
	w.editorBeforeWrite = func() { close(entered); <-release }
	receipt, err := w.QueueEditor(t.Context(), "edicion:one", editorDraftFixture("r1", "private"))
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if durable, err := receipt.Status(); durable || err != nil {
		t.Fatal("RAM-only edit declared saved", durable, err)
	}
	if releases.Load() != 0 {
		t.Fatal("pending RAM lost original SH")
	}
	pending, err := w.Pending(t.Context())
	if err != nil || pending.Count != 1 {
		t.Fatal("admitted RAM missing from exact pending count", pending, err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := w.Drain(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal("drain passed unfinished writer", err)
	}
	unblock()
	if err := w.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if durable, err := receipt.Status(); !durable || err != nil {
		t.Fatal("completed WAL did not acknowledge durability", durable, err)
	}
	if releases.Load() != 1 {
		t.Fatal("original lease release was not exact", releases.Load())
	}
	var stored EditorDraft
	if found, err := w.journal.Read(w.Owner(), EspacioBorradores, "edicion:one", &stored); err != nil || !found || stored.EditorHTML() != "private" {
		t.Fatal("writer lost accepted source", found, err)
	}
}

func TestEditorQueueDoesNotAcceptWhenOriginalLeaseIsDenied(t *testing.T) {
	w := localWorkFixture(t)
	w.acquire = func(context.Context) (func(), error) { return nil, errors.New("paused") }
	if receipt, err := w.QueueEditor(t.Context(), "edicion:one", editorDraftFixture("r1", "unaccepted")); receipt != nil || !errors.Is(err, ErrLocalAdmission) {
		t.Fatal(receipt, err)
	}
	if pending, err := w.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("denied edit became accepted RAM", pending, err)
	}
}

func TestEditorQueueRejectsInvalidUTF8BeforeAdmission(t *testing.T) {
	w := localWorkFixture(t)
	for _, draft := range []EditorDraft{
		editorDraftFixture("r1", string([]byte{0xff})),
		{Version: 3, Blocked: true, NoteID: "n1", NotebookID: "c1", Revision: "r2", HTML: string([]byte{0xff})},
	} {
		if receipt, err := w.QueueEditor(t.Context(), "edicion:invalid", draft); receipt != nil || !errors.Is(err, ErrEditorDraft) {
			t.Fatal("invalid UTF-8 was admitted for JSON serialization", receipt, err)
		}
	}
	if pending, err := w.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("invalid content was stored or counted as admitted", pending, err)
	}
}

func TestEditorQueueIOFailureKeepsSourceAndLeaseUntilDrainRetry(t *testing.T) {
	w := localWorkFixture(t)
	var fail atomic.Bool
	fail.Store(true)
	var releases atomic.Int32
	w.acquire = func(context.Context) (func(), error) { return func() { releases.Add(1) }, nil }
	ioError := errors.New("journal write failed")
	w.editorWriter = func(ctx context.Context, id string, d EditorDraft) error {
		if fail.Load() {
			return ioError
		}
		return w.journal.Write(w.Owner(), EspacioBorradores, id, d)
	}
	receipt, err := w.QueueEditor(t.Context(), "edicion:one", editorDraftFixture("r1", "retained"))
	if err != nil {
		t.Fatal(err)
	}
	<-receipt.Done()
	if durable, err := receipt.Status(); durable || !errors.Is(err, ioError) {
		t.Fatal(durable, err)
	}
	if pending, err := w.Pending(t.Context()); err != nil || pending.Count != 1 || releases.Load() != 0 {
		t.Fatal("IO failure erased accepted work or its lease", pending, err, releases.Load())
	}
	if err := w.Drain(t.Context()); !errors.Is(err, ioError) {
		t.Fatal("unknown RAM became permission to close", err)
	}
	fail.Store(false)
	if err := w.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if durable, err := receipt.Status(); !durable || err != nil || releases.Load() != 1 {
		t.Fatal("retry lost source or lease ownership", durable, err, releases.Load())
	}
}

func TestEditorQueueFailureReceiptPrecedesSuccessfulRetry(t *testing.T) {
	w := localWorkFixture(t)
	var fail atomic.Bool
	fail.Store(true)
	ioError := errors.New("journal write failed")
	w.editorWriter = func(ctx context.Context, id string, d EditorDraft) error {
		if fail.Swap(false) {
			return ioError
		}
		return w.journal.Write(w.Owner(), EspacioBorradores, id, d)
	}
	beforeReceipt, releaseReceipt := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(releaseReceipt) })
	t.Cleanup(unblock)
	beforeWrite, releaseWrite := make(chan struct{}), make(chan struct{})
	unblockWrite := sync.OnceFunc(func() { close(releaseWrite) })
	t.Cleanup(unblockWrite)
	w.editorBeforeWrite = func() { close(beforeWrite); <-releaseWrite }
	var first sync.Once
	id := "edicion:one"
	receipt, err := w.QueueEditor(t.Context(), id, editorDraftFixture("r1", "retained"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-beforeWrite:
	case <-time.After(5 * time.Second):
		t.Fatal("first attempt did not start")
	}
	lockHeld := make(chan bool, 1)
	receipt.beforeFinish = func() {
		first.Do(func() {
			acquired := w.writes.TryLock()
			if acquired {
				w.writes.Unlock()
			}
			lockHeld <- !acquired
			close(beforeReceipt)
			<-releaseReceipt
		})
	}
	unblockWrite()
	select {
	case <-beforeReceipt:
	case <-time.After(5 * time.Second):
		t.Fatal("first attempt did not reach receipt notification")
	}
	// A retry must not enter persistence while the failure notification is
	// pending: otherwise the late failure could overwrite its successful status.
	if !<-lockHeld {
		t.Fatal("writer exclusion released before the receipt was completed")
	}
	retry := make(chan error, 1)
	go func() { retry <- w.writeEditors(t.Context(), id) }()
	unblock()
	select {
	case err := <-retry:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retry did not complete")
	}
	// Done reports the first attempt, not a second completion.
	select {
	case <-receipt.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("first attempt did not notify its receipt")
	}
	if durable, err := receipt.Status(); !durable || err != nil {
		t.Fatal("stale failure overwrote successful retry", durable, err)
	}
	if exact, err := receipt.ExactStatus(); !exact || err != nil {
		t.Fatal("retry did not persist the exact admitted source", exact, err)
	}
}

func TestEditorQueueACKRebasesRevisionStillOnlyInRAM(t *testing.T) {
	w := localWorkFixture(t)
	v1 := editorDraftFixture("r1", "first")
	v2 := editorDraftFixture("r2", "second")
	id := "edicion:one"
	if err := w.journal.Write(w.Owner(), EspacioBorradores, id, v1); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	w.editorBeforeWrite = func() { close(entered); <-release }
	receipt, err := w.QueueEditor(t.Context(), id, v2)
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	updated := api.Note{ID: "n1", NotebookID: "c1", UpdatedAt: v1.ExpectedUpdatedAt.Add(time.Second)}
	if err := w.AcknowledgeEditor(t.Context(), id, v1, updated); err != nil {
		t.Fatal(err)
	}
	unblock()
	if err := w.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	<-receipt.Done()
	if covered, err := receipt.Status(); !covered || err != nil {
		t.Fatal("rebased revision should remain covered", covered, err)
	}
	if exact, err := receipt.ExactStatus(); exact || !errors.Is(err, ErrEditorSuperseded) {
		t.Fatal("a rebase must not claim the original source was written byte-for-byte", exact, err)
	}
	var stored EditorDraft
	found, err := w.journal.Read(w.Owner(), EspacioBorradores, id, &stored)
	if err != nil || !found || stored.Revision != "r2" || stored.EditorHTML() != "second" || !stored.ExpectedUpdatedAt.Equal(updated.UpdatedAt) {
		t.Fatal("ACK lost or failed to rebase the newer accepted source", stored.Revision, found, err)
	}
}

func TestEditorQueueCoalescesWithoutReleasingLastProtectionOrBorrowingInput(t *testing.T) {
	w := localWorkFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	w.editorBeforeWrite = func() { close(entered); <-release }
	var releases, writes atomic.Int32
	w.acquire = func(context.Context) (func(), error) { return func() { releases.Add(1) }, nil }
	w.editorWriter = func(ctx context.Context, id string, d EditorDraft) error {
		writes.Add(1)
		return w.journal.Write(w.Owner(), EspacioBorradores, id, d)
	}
	first, err := w.QueueEditor(t.Context(), "edicion:one", editorDraftFixture("r1", "first"))
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	secondDraft := editorDraftFixture("r2", "latest")
	secondDraft.Input.Data = json.RawMessage(`{"marker":"owned"}`)
	x := 7
	secondDraft.Input.X = &x
	second, err := w.QueueEditor(t.Context(), "edicion:one", secondDraft)
	if err != nil {
		t.Fatal(err)
	}
	x = 99
	copy(secondDraft.Input.Data, `{"marker":"wrong"}`)
	if releases.Load() != 1 {
		t.Fatal("coalescing lost the original pending lease", releases.Load())
	}
	if pending, err := w.Pending(t.Context()); err != nil || pending.Count != 1 {
		t.Fatal("same draft counted twice", pending, err)
	}
	unblock()
	if err := w.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if durable, err := first.Status(); !durable || err != nil {
		t.Fatal("coalesced source was not covered by the later persisted edit", durable, err)
	}
	if exact, err := first.ExactStatus(); exact || !errors.Is(err, ErrEditorSuperseded) {
		t.Fatal("coalesced source was incorrectly reported as exactly persisted", exact, err)
	}
	if durable, err := second.Status(); !durable || err != nil {
		t.Fatal(durable, err)
	}
	if exact, err := second.ExactStatus(); !exact || err != nil {
		t.Fatal("latest admitted source was not written exactly", exact, err)
	}
	var stored EditorDraft
	if found, err := w.journal.Read(w.Owner(), EspacioBorradores, "edicion:one", &stored); err != nil || !found || stored.Revision != "r2" || stored.EditorHTML() != "latest" || stored.Input.X == nil || *stored.Input.X != 7 || !bytes.Contains(stored.Input.Data, []byte("owned")) {
		t.Fatal("writer borrowed mutable input or lost latest version", found, err)
	}
	if writes.Load() != 1 || releases.Load() != 2 {
		t.Fatal("coalescing wrote redundant versions or lost lease cleanup", writes.Load(), releases.Load())
	}
}

func TestEditorQueueReportsExactForEachSourceActuallyWritten(t *testing.T) {
	w := localWorkFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	var writes atomic.Int32
	written := make(chan EditorDraft, 2)
	w.editorWriter = func(ctx context.Context, id string, d EditorDraft) error {
		if writes.Add(1) == 1 {
			close(entered)
			<-release
		}
		if err := w.journal.Write(w.Owner(), EspacioBorradores, id, d); err != nil {
			return err
		}
		var actual EditorDraft
		found, err := w.journal.Read(w.Owner(), EspacioBorradores, id, &actual)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("editor source missing after write")
		}
		select {
		case written <- actual:
		default:
			return errors.New("unexpected extra editor write")
		}
		return nil
	}
	id := "edicion:one"
	first, err := w.QueueEditor(t.Context(), id, editorDraftFixture("r1", "first"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first source did not reach persistence")
	}
	second, err := w.QueueEditor(t.Context(), id, editorDraftFixture("r2", "second"))
	if err != nil {
		t.Fatal(err)
	}
	unblock()
	drainCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := w.Drain(drainCtx); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range []*DraftReceipt{first, second} {
		if exact, err := receipt.ExactStatus(); !exact || err != nil {
			t.Fatal("a source actually written was not reported as exact", receipt.Revision, exact, err)
		}
	}
	if writes.Load() != 2 {
		t.Fatal("expected each source to be written once", writes.Load())
	}
	firstWritten, secondWritten := <-written, <-written
	if firstWritten.Revision != "r1" || firstWritten.EditorHTML() != "first" || secondWritten.Revision != "r2" || secondWritten.EditorHTML() != "second" {
		t.Fatal("journal readback did not match the two admitted sources")
	}
	var stored EditorDraft
	if found, err := w.journal.Read(w.Owner(), EspacioBorradores, id, &stored); err != nil || !found || stored.Revision != "r2" || stored.EditorHTML() != "second" {
		t.Fatal("latest source was not retained", found, err)
	}
}

func TestEditorQueueRetainsRealSessionLockWhileAcceptedRAMIsNotDurable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("real remote Linux session lock; Apple and Binder are separate probes")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	identity := auth.IdentitySnapshot{Issuer: "https://id.example.test", Client: "notes", Subject: "alice", Organization: "acme"}
	storeID := "sesion-dddddddddddddddddddddddddddddddd"
	if err := ajustes.PonerAccesoSesionCon(t.Context(), storeID, identity.IdentidadSesion(), true); err != nil {
		t.Fatal(err)
	}
	access, err := ajustes.LeerAccesoSesionCon(t.Context(), storeID)
	if err != nil {
		t.Fatal(err)
	}
	witness := auth.TestigoTrabajo{Almacen: storeID, Acceso: access}
	w := localWorkFixture(t)
	w.acquire = witness.Compartido
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	w.editorBeforeWrite = func() { close(entered); <-release }
	receipt, err := w.QueueEditor(t.Context(), "edicion:session", editorDraftFixture("r1", "protected RAM"))
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	blocked, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if release, err := witness.Exclusivo(blocked); err == nil {
		release()
		t.Fatal("another app obtained EX while admitted draft was only RAM")
	}
	unblock()
	if err := w.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if durable, err := receipt.Status(); !durable || err != nil {
		t.Fatal(durable, err)
	}
	releaseExclusive, err := witness.Exclusivo(t.Context())
	if err != nil {
		t.Fatal("durability left shared lease held", err)
	}
	releaseExclusive()
}

func TestEditorQueueRejectsForeignNoteAndACKWithoutChangingAcceptedSource(t *testing.T) {
	w := localWorkFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	w.editorBeforeWrite = func() { close(entered); <-release }
	original := editorDraftFixture("r1", "original")
	receipt, err := w.QueueEditor(t.Context(), "edicion:one", original)
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	foreign := original
	foreign.NoteID = "foreign"
	if receipt, err := w.QueueEditor(t.Context(), "edicion:one", foreign); receipt != nil || !errors.Is(err, ErrLocalAdmission) {
		t.Fatal("foreign source replaced admitted note", receipt, err)
	}
	if err := w.AcknowledgeEditor(t.Context(), "edicion:one", original, api.Note{ID: "foreign", NotebookID: "c1"}); err == nil {
		t.Fatal("foreign ACK accepted")
	}
	unblock()
	if err := w.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if durable, err := receipt.Status(); !durable || err != nil {
		t.Fatal(durable, err)
	}
	var stored EditorDraft
	if found, err := w.journal.Read(w.Owner(), EspacioBorradores, "edicion:one", &stored); err != nil || !found || stored.NoteID != "n1" || stored.EditorHTML() != "original" {
		t.Fatal("foreign call changed accepted source", found, err)
	}
}

func TestEditorQueueRecoveryKeepsNewestRAMRevisionAndRejectsOldWALCandidate(t *testing.T) {
	w := localWorkFixture(t)
	id := "edicion:one"
	v1 := editorDraftFixture("r1", "durable old")
	if err := w.journal.Write(w.Owner(), EspacioBorradores, id, v1); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	w.editorBeforeWrite = func() { close(entered); <-release }
	v2 := editorDraftFixture("r2", "latest RAM")
	v2.Input.Data = json.RawMessage(`{"owned":true}`)
	receipt, err := w.QueueEditor(t.Context(), id, v2)
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	gotID, got, gotReceipt, found := w.PendingEditor("n1", "c1")
	if !found || gotID != id || got.Revision != "r2" || got.EditorHTML() != v2.EditorHTML() || gotReceipt != receipt {
		t.Fatal("reopened source lost its accepted identity", gotID, found)
	}
	copy(got.Input.Data, `{"owned":null}`)
	_, again, _, _ := w.PendingEditor("n1", "c1")
	if !bytes.Equal(again.Input.Data, v2.Input.Data) {
		t.Fatal("reopened editor borrowed writer memory")
	}
	if _, found, err := w.DurableEditor(t.Context(), id); found || !errors.Is(err, ErrEditorPending) {
		t.Fatal("old WAL became a send candidate while newer RAM was accepted", found, err)
	}
	if _, _, _, found := w.PendingEditor("n1", "foreign"); found {
		t.Fatal("another notebook received the pending source")
	}
	unblock()
	if err := w.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	latest, found, err := w.DurableEditor(t.Context(), id)
	if err != nil || !found || latest.Revision != "r2" {
		t.Fatal("durable handoff lost the accepted source", found, err)
	}
}
