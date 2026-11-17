// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	catalogtransfer "git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

type CopyPrivateFile func(context.Context, storage.PublicationTarget, string, storage.Reference, string, string) (storage.Reference, error)

type recordTransfer struct {
	source, target *Repository
	state          SnapshotState
}

// MigratePrivateCatalog accepts only the narrow historical source. Ordinary
// New repositories and every application facade remain isolated-only.
func (r *Repository) MigratePrivateCatalog(parent context.Context, source *HistoricalSource, copyFile CopyPrivateFile) error {
	if source == nil || source.repository == nil || copyFile == nil {
		return ErrSource
	}
	previous := source.repository
	if previous == r || previous.cfg.Scope != r.cfg.Scope || previous.cfg.EntityKind != r.cfg.EntityKind || previous.cfg.ReceiptKind != r.cfg.ReceiptKind {
		return ErrSource
	}
	r.mu.Lock()
	ready := r.ready
	r.mu.Unlock()
	if ready {
		return ErrSource
	}
	frozenPort, ok := previous.cfg.Crypto.(frozenRecordSource)
	if !ok {
		return ErrSource
	}
	oldFiles, ok := previous.cfg.Crypto.(binaryCommitmentReader)
	if !ok {
		return ErrSource
	}
	newFiles, ok := r.cfg.Crypto.(binaryCommitmentReader)
	if !ok {
		return ErrSource
	}
	ctx, done, err := r.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	ctx, sourceDone, err := previous.Start(ctx)
	if err != nil {
		return err
	}
	defer sourceDone()
	return r.cfg.Journal.Exclusive(ctx, r.owner(), r.namespace(), "catalog", func() error {
		destination, err := r.cfg.Crypto.PrepareStorage(ctx, r.cfg.Scope)
		if err != nil {
			return err
		}
		frozen, err := frozenPort.FrozenStorageBindings(ctx)
		if err != nil {
			return err
		}
		rules := recordTransfer{source: previous, target: r}
		err = catalogtransfer.Run(ctx, catalogtransfer.Config{
			Scope: r.cfg.Scope, Owner: r.owner(), Namespace: storage.IdentityJournalNamespace(r.cfg.Product), ReceiptKind: r.cfg.ReceiptKind, Journal: r.cfg.Journal,
			Source: previous.cfg.Crypto, Target: r.cfg.Crypto, FrozenBindings: frozenPort.FrozenStorageBindings, Rules: &rules,
			PutObject: r.cfg.Crypto.PutStorageObjectAt, PutRecord: r.cfg.Crypto.PutStorageRecordAt, CopyFile: copyFile,
			SourceFileCommitment: oldFiles.StorageFileCommitment, TargetFileCommitment: newFiles.StorageFileCommitment,
			VerifyFile: func(ctx context.Context, ref storage.Reference, object, revision string) error {
				_, err := r.cfg.Crypto.DownloadStorageFile(ctx, r.cfg.Gateway, ref, object, revision, io.Discard)
				return err
			},
			AcknowledgeFile: func(ctx context.Context, job string, ref storage.Reference) error {
				return r.cfg.Crypto.AcknowledgeStorageFile(ctx, r.cfg.Journal, job, ref)
			},
			PrepareTarget: r.refresh, MarkReady: func(ctx context.Context) error { return r.openCompleted(ctx, rules.state) },
		}, frozen, destination)
		return transferError(err)
	})
}

func transferError(err error) error {
	if errors.Is(err, catalogtransfer.ErrSource) {
		return ErrSource
	}
	if errors.Is(err, catalogtransfer.ErrNotReady) {
		return ErrNotReady
	}
	return err
}

func (t *recordTransfer) SourceComplete(ctx context.Context) error {
	state, err := t.source.cfg.Source.State(ctx, t.source.cfg.Scope)
	if err != nil {
		return err
	}
	if err := t.source.validateState(state); err != nil {
		return err
	}
	if state.Mode == "legacy" {
		return ErrNotReady
	}
	if err := t.source.verifyCompleted(ctx, state); err != nil {
		return err
	}
	t.state = state
	return nil
}

func (t *recordTransfer) TargetComplete(ctx context.Context) error {
	return t.target.verifyCompleted(ctx, t.state)
}

func (t *recordTransfer) Classify(ctx context.Context, version storage.ObjectVersion, _ catalogtransfer.Lookup) (catalogtransfer.Class, error) {
	var header struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(version.Commit.Metadata, &header) != nil {
		return catalogtransfer.Class{}, ErrSource
	}
	if header.Kind == t.source.cfg.ReceiptKind {
		return catalogtransfer.Class{Order: 2}, t.receipt(ctx, version)
	}
	value, err := t.source.description(version)
	if err != nil {
		return catalogtransfer.Class{}, err
	}
	if version.Commit.State == "deleted" && version.Commit.Content == nil {
		return catalogtransfer.Class{Order: 1}, nil
	}
	if value.Binary {
		_, err := t.source.verifyBinaryCommitment(ctx, version)
		return catalogtransfer.Class{Order: 0}, err
	}
	raw, err := t.source.readJSON(ctx, version)
	clear(raw)
	return catalogtransfer.Class{Order: 1}, err
}

func (t *recordTransfer) receipt(ctx context.Context, version storage.ObjectVersion) error {
	var receipt snapshotReceipt
	if json.Unmarshal(version.Commit.Metadata, &receipt) != nil || receipt.Kind != t.source.cfg.ReceiptKind || !storage.MigrationEpoch(receipt.Epoch) || receipt.Rows < 0 || receipt.Blobs < 0 || receipt.Blobs > receipt.Rows || len(receipt.Inventory) != 64 || version.ObjectID != t.source.cfg.Product+"_snapshot_"+hashBytes([]byte(receipt.Epoch)) || version.Commit.State != "live" {
		return ErrSource
	}
	if _, err := hex.DecodeString(receipt.Inventory); err != nil {
		return ErrSource
	}
	pointer := version.Commit.Content
	if pointer == nil || pointer.Kind != "record" {
		return ErrSource
	}
	record, err := t.source.cfg.Crypto.ReadStorageRecord(ctx, pointer.Reference, version.ObjectID, "content")
	if err != nil {
		return err
	}
	defer clear(record.Payload)
	if record.Validate(t.source.cfg.Scope) != nil || record.RevisionID != pointer.RevisionID || record.ObjectID != version.ObjectID || record.Role != "content" || !bytes.Equal(record.Payload, version.Commit.Metadata) {
		return ErrSource
	}
	return nil
}

func (t *recordTransfer) Rewrite(_ context.Context, _ storage.ObjectVersion, record storage.Record, _ catalogtransfer.Resolve) ([]byte, error) {
	var header struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(record.Payload, &header) != nil || (header.Kind != t.source.cfg.EntityKind && header.Kind != t.source.cfg.ReceiptKind) {
		return nil, ErrSource
	}
	// The inner JSON is base64 bytes: retain unknown fields and large integers.
	return bytes.Clone(record.Payload), nil
}

func (t *recordTransfer) CheckPending(ctx context.Context, source storage.FrozenBinding, destination storage.Binding) error {
	ids, err := t.source.cfg.Journal.RecordIDs(t.source.owner(), t.source.namespace())
	if err != nil {
		return err
	}
	if len(ids) != 0 {
		return ErrSource
	}
	ids, err = t.source.cfg.Journal.RecordIDs(t.source.owner(), storage.PendingUploadsNamespace)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		var work struct {
			Target storage.PublicationTarget `json:"target"`
		}
		found, err := t.source.cfg.Journal.Read(t.source.owner(), storage.PendingUploadsNamespace, id, &work)
		if err != nil {
			return err
		}
		if !found || !work.Target.Valid() || slices.Contains(source.Binding.Rooms, work.Target.RoomID) || !slices.Contains(destination.Rooms, work.Target.RoomID) {
			return ErrSource
		}
	}
	return nil
}

func (r *Repository) PrivateMigrationReceipt(parent context.Context) (storage.Reference, error) {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return storage.Reference{}, err
	}
	defer done()
	if r.cfg.Crypto.StorageIdentityVersion() != storage.IsolatedIdentity {
		return storage.Reference{}, ErrSource
	}
	if err := r.requireReady(); err != nil {
		return storage.Reference{}, err
	}
	state, err := r.cfg.Source.State(ctx, r.cfg.Scope)
	if err != nil {
		return storage.Reference{}, err
	}
	if err := r.validateState(state); err != nil {
		return storage.Reference{}, err
	}
	if state.Mode == "legacy" {
		return storage.Reference{}, ErrNotReady
	}
	if err := r.verifyCompleted(ctx, state); err != nil {
		return storage.Reference{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	versions := r.heads[r.cfg.Product+"_snapshot_"+hashBytes([]byte(state.Epoch))]
	if len(versions) != 1 {
		return storage.Reference{}, ErrConflict
	}
	return versions[0].Reference, nil
}

func (r *Repository) AcknowledgeIdentityCutover(parent context.Context, state storage.IdentityCutoverState) error {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	if state.Validate(r.cfg.Scope) != nil || state.Phase != "isolated" || state.Receipt == nil {
		return ErrSource
	}
	return r.cfg.Journal.Exclusive(ctx, r.owner(), r.namespace(), "catalog", func() error {
		receipt, err := r.PrivateMigrationReceipt(ctx)
		if err != nil {
			return err
		}
		return transferError(catalogtransfer.RetireAcknowledged(ctx, r.cfg.Journal, r.owner(), storage.IdentityJournalNamespace(r.cfg.Product), storage.IdentityHistoryNamespace(r.cfg.Product), r.cfg.Scope, state, receipt))
	})
}
