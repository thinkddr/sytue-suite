// SPDX-License-Identifier: Apache-2.0
package notas

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

var (
	ErrEditorDraft   = errors.New("notes: an editor draft cannot be authenticated")
	ErrEditorPending = errors.New("notes: a newer accepted editor revision is not durable")
	// ErrEditorSuperseded means a later source replaced this admission before
	// it was written. Its receipt is not evidence that these bytes were saved.
	ErrEditorSuperseded = errors.New("notes: editor revision was superseded before persistence")
)

// DraftReceipt belongs to an admitted revision. Durable remains false after an
// IO failure; a later Drain/retry can make it durable. Status reports coverage
// by a persisted revision, which may be a newer coalesced or rebased source.
// ExactStatus distinguishes that case. Neither replaces an authenticated
// readback before a transition. Done signals an attempted write.
type DraftReceipt struct {
	Revision     string
	mu           sync.Mutex
	done         chan struct{}
	once         sync.Once
	durable      bool
	err          error
	exact        bool
	exactErr     error
	beforeFinish func() // Test hook; installed before the first write starts.
}

func (r *DraftReceipt) Done() <-chan struct{} { return r.done }

func (r *DraftReceipt) Status() (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.durable, r.err
}

// ExactStatus reports whether the exact admitted source was written. A later
// revision or metadata rebase may cover it without writing those same bytes.
func (r *DraftReceipt) ExactStatus() (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exact, r.exactErr
}

func (r *DraftReceipt) finish(durable, exact bool, err error) {
	if r.beforeFinish != nil {
		r.beforeFinish()
	}
	r.mu.Lock()
	r.durable = r.durable || durable
	r.err = err
	r.exact = r.exact || exact
	if durable && !exact {
		r.exactErr = ErrEditorSuperseded
	} else {
		r.exactErr = err
	}
	r.mu.Unlock()
	r.once.Do(func() { close(r.done) })
}

type (
	draftAdmission struct {
		sequence uint64
		receipt  *DraftReceipt
	}
	editorSlot struct {
		latest     EditorDraft
		sequence   uint64
		order      uint64
		admissions []draftAdmission
		release    func()
		running    bool
	}
)

// QueueEditor acquires the original session's SH before accepting the immutable
// source and starting its tracked writer. The existing lease remains held while
// any newer accepted revision is only in RAM. Serialization and fsync stay off UI.
func (w *LocalWork) QueueEditor(ctx context.Context, id string, value EditorDraft) (*DraftReceipt, error) {
	if !value.Valid() || !strings.HasPrefix(id, "edicion:") || len(id) == len("edicion:") {
		return nil, ErrEditorDraft
	}
	_, done, err := w.Start()
	if err != nil {
		return nil, errors.Join(ErrLocalAdmission, err)
	}
	release, err := w.acquire(ctx)
	if err == nil && release == nil {
		err = offline.ErrWorkLease
	}
	if err != nil {
		done()
		return nil, errors.Join(ErrLocalAdmission, err)
	}
	owned := value.Clone()
	receipt := &DraftReceipt{Revision: owned.Revision, done: make(chan struct{})}
	w.queueMu.Lock()
	slot := w.editors[id]
	if slot != nil && (slot.latest.NoteID != owned.NoteID || slot.latest.NotebookID != owned.NotebookID) {
		w.queueMu.Unlock()
		release()
		done()
		return nil, errors.Join(ErrLocalAdmission, ErrEditorDraft)
	}
	extraRelease := false
	if slot == nil {
		slot = &editorSlot{release: release}
		w.editors[id] = slot
	} else {
		extraRelease = true
	}
	slot.sequence++
	w.editorOrder++
	slot.order = w.editorOrder
	slot.latest = owned
	slot.admissions = append(slot.admissions, draftAdmission{sequence: slot.sequence, receipt: receipt})
	start := !slot.running
	if start {
		slot.running = true
	}
	w.queueMu.Unlock()
	if extraRelease {
		release()
	}
	if start {
		go func() {
			defer done()
			if w.editorBeforeWrite != nil {
				w.editorBeforeWrite()
			}
			writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			_ = w.writeEditors(writeCtx, id)
		}()
	} else {
		done()
	}
	return receipt, nil
}

// writeEditors shares the ACK exclusion, but never holds queueMu during IO.
// An ACK changes sequence even when revision is unchanged, so an older write
// cannot acknowledge a RAM revision whose CAS timestamp has since been rebased.
func (w *LocalWork) writeEditors(ctx context.Context, id string) error {
	for {
		w.writes.Lock()
		w.queueMu.Lock()
		slot := w.editors[id]
		if slot == nil {
			w.queueMu.Unlock()
			w.writes.Unlock()
			return nil
		}
		snapshot, sequence := slot.latest, slot.sequence
		w.queueMu.Unlock()
		err := w.persistEditor(ctx, id, snapshot)
		receipts, release := w.completeEditorWrite(id, sequence, err)
		// Complete receipts before releasing the writer exclusion: otherwise a
		// successful retry could be followed by a stale failure notification.
		for _, admission := range receipts {
			admission.receipt.finish(err == nil, err == nil && admission.sequence == sequence, err)
		}
		w.writes.Unlock()
		if release != nil {
			release()
		}
		if err != nil {
			return err
		}
	}
}

func (w *LocalWork) completeEditorWrite(id string, sequence uint64, err error) ([]draftAdmission, func()) {
	w.queueMu.Lock()
	defer w.queueMu.Unlock()
	slot := w.editors[id]
	if slot == nil {
		return nil, nil
	}
	if err != nil {
		slot.running = false
		return append([]draftAdmission(nil), slot.admissions...), nil
	}
	var receipts []draftAdmission
	kept := slot.admissions[:0]
	for _, admission := range slot.admissions {
		if admission.sequence <= sequence {
			receipts = append(receipts, admission)
		} else {
			kept = append(kept, admission)
		}
	}
	slot.admissions = kept
	var release func()
	if slot.sequence == sequence {
		release = slot.release
		delete(w.editors, id)
	}
	return receipts, release
}

func (w *LocalWork) persistEditor(ctx context.Context, id string, draft EditorDraft) error {
	if w.editorWriter != nil {
		return w.editorWriter(ctx, id, draft)
	}
	return w.journal.Exclusive(ctx, w.Owner(), EspacioBorradores, id, func() error { return w.journal.Write(w.Owner(), EspacioBorradores, id, draft) })
}

func (w *LocalWork) flushEditors(ctx context.Context) error {
	for {
		w.queueMu.Lock()
		id := ""
		for key := range w.editors {
			id = key
			break
		}
		w.queueMu.Unlock()
		if id == "" {
			return ctx.Err()
		}
		if err := w.writeEditors(ctx, id); err != nil {
			return err
		}
	}
}

// AcknowledgeEditor rebases both queued RAM and its durable source under the
// writer exclusion. It never deletes a later revision or another note's draft.
func (w *LocalWork) AcknowledgeEditor(ctx context.Context, id string, sent EditorDraft, updated api.Note) error {
	if !sent.Valid() || updated.ID != sent.NoteID || updated.NotebookID != sent.NotebookID {
		return ErrEditorDraft
	}
	w.writes.Lock()
	defer w.writes.Unlock()
	return w.acknowledgeEditorLocked(ctx, id, sent, updated)
}

func (w *LocalWork) acknowledgeEditorLocked(ctx context.Context, id string, sent EditorDraft, updated api.Note) error {
	w.queueMu.Lock()
	if slot := w.editors[id]; slot != nil {
		current := slot.latest
		if current.NoteID != sent.NoteID || current.NotebookID != sent.NotebookID {
			w.queueMu.Unlock()
			return ErrEditorDraft
		}
		if current.Revision != sent.Revision && current.ExpectedUpdatedAt.Equal(sent.ExpectedUpdatedAt) && !updated.UpdatedAt.IsZero() {
			current.ExpectedUpdatedAt = updated.UpdatedAt
			slot.latest = current
			slot.sequence++
		}
	}
	w.queueMu.Unlock()
	return w.journal.Exclusive(ctx, w.Owner(), EspacioBorradores, id, func() error {
		var current EditorDraft
		found, err := w.journal.Read(w.Owner(), EspacioBorradores, id, &current)
		if err != nil || !found {
			return err
		}
		if !current.Valid() || current.NoteID != sent.NoteID || current.NotebookID != sent.NotebookID {
			return ErrEditorDraft
		}
		if current.Revision == sent.Revision {
			return w.journal.Delete(w.Owner(), EspacioBorradores, id)
		}
		if current.ExpectedUpdatedAt.Equal(sent.ExpectedUpdatedAt) && !updated.UpdatedAt.IsZero() {
			current.ExpectedUpdatedAt = updated.UpdatedAt
			return w.journal.Write(w.Owner(), EspacioBorradores, id, current)
		}
		return nil
	})
}

// PendingEditor returns the newest accepted source before consulting older WAL.
// Reopening keeps its original revision and durability receipt. Mutable fields
// are copied so the reopened editor cannot change the admitted writer's source.
func (w *LocalWork) PendingEditor(note, notebook string) (string, EditorDraft, *DraftReceipt, bool) {
	w.queueMu.Lock()
	defer w.queueMu.Unlock()
	var latest *editorSlot
	id := ""
	for key, slot := range w.editors {
		if slot.latest.NoteID == note && slot.latest.NotebookID == notebook && (latest == nil || slot.order > latest.order) {
			latest = slot
			id = key
		}
	}
	if latest == nil {
		return "", EditorDraft{}, nil, false
	}
	return id, latest.latest.Clone(), latest.admissions[len(latest.admissions)-1].receipt, true
}

func (w *LocalWork) EditorPending(id string) bool {
	w.queueMu.Lock()
	defer w.queueMu.Unlock()
	return w.editors[id] != nil
}

// DurableEditor serializes candidate selection with the writer/ACK handoff.
// An older receipt cannot authorize publication of WAL while a newer source is
// accepted in RAM. A subsequent edit may still arrive during a network send;
// AcknowledgeEditor preserves and rebases that newer revision.
func (w *LocalWork) DurableEditor(ctx context.Context, id string) (EditorDraft, bool, error) {
	if err := ctx.Err(); err != nil {
		return EditorDraft{}, false, err
	}
	w.writes.Lock()
	defer w.writes.Unlock()
	if w.EditorPending(id) {
		return EditorDraft{}, false, ErrEditorPending
	}
	var draft EditorDraft
	found, err := w.journal.Read(w.Owner(), EspacioBorradores, id, &draft)
	if err == nil && found && !draft.Valid() {
		err = ErrEditorDraft
	}
	return draft, found, err
}
