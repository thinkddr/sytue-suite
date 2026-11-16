// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"errors"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/filesdk"
)

// acknowledgeReconciledFile runs inside the caller's original work lease and
// catalog exclusion after the publication has been validated against a complete
// snapshot. Only an SDK-minted complete-batch proof can retire a prepared file.
func (r *Repository) acknowledgeReconciledFile(ctx context.Context, job string, actual storage.ObjectVersion) error {
	r.mu.Lock()
	if r.closed || !r.binding.Same(r.catalog.Binding) {
		r.mu.Unlock()
		return ErrSource
	}
	version, found, err := storage.AcceptedAtomicObject(r.catalog, r.cfg.Scope, actual.ObjectID, actual.RevisionID, &actual.Reference)
	if err != nil || !found || !version.Commit.MatchesExact(actual.Commit) {
		r.mu.Unlock()
		return ErrSource
	}
	entry := r.catalog.Entries[actual.Reference]
	entry.Record.Payload = slices.Clone(entry.Record.Payload)
	entry.Record.Previous = slices.Clone(entry.Record.Previous)
	binding := r.catalog.Binding
	binding.Rooms = slices.Clone(binding.Rooms)
	prepared, _, preparedErr := storage.DecodeAtomicPrepared(entry, r.cfg.Scope)
	root := r.catalog.Batches[prepared.BatchID]
	r.mu.Unlock()
	defer clear(entry.Record.Payload)

	if preparedErr != nil {
		if _, err := storage.DecodeObject(entry, r.cfg.Scope); err != nil {
			return ErrSource
		}
		return r.cfg.Crypto.AcknowledgeStorageFile(ctx, r.cfg.Journal, job, actual.Reference)
	}
	if !root.Valid() {
		return ErrSource
	}
	sdk, ok := r.cfg.Crypto.(filesdk.SDK)
	if !ok {
		return ErrSource
	}
	files, err := filesdk.New(r.cfg.Scope, sdk, r.cfg.Gateway, r.cfg.Journal)
	if err != nil {
		return err
	}
	current, err := sdk.StorageBindings(ctx)
	if err != nil {
		return err
	}
	if !binding.Same(current) {
		return ErrSource
	}
	batch, err := storage.VerifyAtomicBatch(ctx, sdk, r.cfg.Scope, root)
	if err != nil {
		return err
	}
	record, err := sdk.ReadStorageRecord(ctx, actual.Reference, actual.ObjectID, "index")
	if err != nil {
		return err
	}
	defer clear(record.Payload)
	if !entry.Record.MatchesExact(record) {
		return ErrSource
	}
	current, err = sdk.StorageBindings(ctx)
	if err != nil {
		return err
	}
	if !binding.Same(current) {
		return ErrSource
	}
	completion, err := batch.FileCompletion(ctx, files, actual.Reference)
	if err != nil {
		return err
	}
	err = files.AcknowledgeAtomic(ctx, job, completion)
	if errors.Is(err, storage.ErrUploadMissing) {
		// Unlike the legacy path, absence of an atomic upload WAL does not prove
		// that this member was durably acknowledged. Preserve staged input.
		return ErrSource
	}
	return err
}
