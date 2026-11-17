// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// openAndReconcile retires only local receipts whose complete immutable baseline
// was just authenticated. It never resumes a publication, reads SQL bodies or
// chooses between branches. The original device can reconcile after a handoff.
func (r *Repository) openAndReconcile(ctx context.Context, state SnapshotState) error {
	if err := r.openCompleted(ctx, state); err != nil {
		return err
	}
	if err := r.reconcileSnapshotJournal(ctx, state); err != nil {
		r.mu.Lock()
		r.ready = false
		r.mu.Unlock()
		return err
	}
	return nil
}

func (r *Repository) reconcileSnapshotJournal(ctx context.Context, state SnapshotState) error {
	ids, err := r.cfg.Journal.RecordIDs(r.owner(), r.namespace())
	if err != nil {
		return err
	}
	var control *snapshotWork
	var completed []string
	for _, id := range ids {
		if id == "snapshot" {
			var work snapshotWork
			found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), id, &work)
			if err != nil {
				return err
			}
			if !found || work.Version != 1 || work.Scope != r.cfg.Scope || work.Epoch != state.Epoch || !work.Target.Valid() {
				return ErrSource
			}
			control = &work
			continue
		}
		if !strings.HasPrefix(id, "import_") {
			continue
		}
		var work publication
		found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), id, &work)
		if err != nil {
			return err
		}
		if !found {
			return ErrSource
		}
		if err := r.reconcileSnapshotPublication(ctx, id, state, work); err != nil {
			return err
		}
		completed = append(completed, id)
	}
	if control == nil && len(completed) > 0 {
		return ErrSource
	}
	for _, key := range completed {
		if err := r.cfg.Journal.Delete(r.owner(), r.namespace(), key); err != nil {
			return err
		}
	}
	if control != nil {
		return r.cfg.Journal.Delete(r.owner(), r.namespace(), "snapshot")
	}
	return nil
}

func (r *Repository) reconcileSnapshotPublication(ctx context.Context, key string, state SnapshotState, work publication) error {
	if err := r.validateSnapshotWork(key, state, work); err != nil {
		return err
	}
	obj := r.object(work.Kind, work.ID)
	r.mu.Lock()
	var versions []storage.ObjectVersion
	for _, version := range r.byObject[obj] {
		if version.RevisionID == work.Revision+"_index" {
			versions = append(versions, cloneVersion(version))
		}
	}
	r.mu.Unlock()
	if len(versions) != 1 {
		return ErrConflict
	}
	actual := versions[0]
	pointer := actual.Commit.Content
	if pointer == nil || actual.Reference.RoomID != work.Target.RoomID || pointer.RevisionID != work.Revision || (work.Content != nil && *work.Content != pointer.Reference) || (work.Index != nil && *work.Index != actual.Reference) {
		return ErrSource
	}
	work.Content = &pointer.Reference
	want, _ := json.Marshal(r.publicationCommit(work))
	got, _ := json.Marshal(actual.Commit)
	if !bytes.Equal(want, got) {
		return ErrSource
	}
	if work.Binary {
		return r.reconcileBinarySnapshotPublication(ctx, key, state, work, actual)
	}
	raw, err := r.readJSON(ctx, actual)
	if err != nil {
		return err
	}
	defer clear(raw)
	baseline := entity{Epoch: state.Epoch, Baseline: work.Baseline, Seed: true}
	if err := work.validate(r.cfg.Scope, work.Kind, work.ID, work.State, raw, &baseline); err != nil {
		return err
	}
	if work.File {
		if err := r.acknowledgeReconciledFile(ctx, key+"_file", actual); err != nil && !errors.Is(err, storage.ErrUploadMissing) {
			return err
		}
	}
	return nil
}

func (r *Repository) validateSnapshotWork(key string, state SnapshotState, work publication) error {
	expected := "import_" + hashBytes([]byte(state.Epoch+"\x00"+work.Kind+"\x00"+work.ID))
	if key != expected || work.Version != 1 || work.Scope != r.cfg.Scope || !work.Seed || work.Epoch != state.Epoch || work.Baseline != work.Input || len(work.Input) != 64 || !storage.Identifier(work.Revision) || !work.Target.Valid() {
		return ErrSource
	}
	return nil
}

func (r *Repository) reconcileBinarySnapshotPublication(ctx context.Context, key string, state SnapshotState, work publication, actual storage.ObjectVersion) error {
	row, err := r.readBinary(ctx, actual, io.Discard)
	if err != nil {
		return err
	}
	if err := r.validateBinaryWork(ctx, work, state, row); err != nil {
		return err
	}
	return r.acknowledgeBinary(ctx, key, work, actual, true)
}
