// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	catalogtransfer "git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

const identityMigrationNamespace = "drive-identity-migration-v1"

// CopyPrivateFile binds the maintained SDK to both verified clients and journals.
type (
	CopyPrivateFile func(context.Context, storage.PublicationTarget, string, storage.Reference, string, string) (storage.Reference, error)
	frozenCrypto    interface {
		FrozenStorageBindings(context.Context) (storage.FrozenBinding, error)
	}
	identityMigration = catalogtransfer.Plan
	identityMapping   = catalogtransfer.Mapping
	// Domain policy owns Drive metadata, references and the retained SQL receipt.
	// Transfer mechanics and durable mappings live in storage/transfer.
	identityTransfer struct {
		target, source *Repository
		state          identityMigration
		legacy         storage.LegacyMigrationState
	}
)

func (r *Repository) MigratePrivateCatalog(parent context.Context, source *Repository, copyFile CopyPrivateFile) error {
	if source == nil || source == r || source.cfg.Scope != r.cfg.Scope || copyFile == nil {
		return ErrSource
	}
	r.mu.Lock()
	alreadyReady := r.ready
	r.mu.Unlock()
	if alreadyReady {
		return ErrSource
	}
	proof, ok := source.cfg.Crypto.(frozenCrypto)
	if !ok {
		return ErrSource
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	ctx, sourceDone, err := source.start(ctx)
	if err != nil {
		return err
	}
	defer sourceDone()
	return r.cfg.Journal.Exclusive(ctx, r.owner(), migrationNamespace, "initialize", func() error {
		if err := source.requireReady(); err != nil {
			return err
		}
		destination, err := r.cfg.Crypto.PrepareStorage(ctx, r.cfg.Scope)
		if err != nil {
			return err
		}
		frozen, err := proof.FrozenStorageBindings(ctx)
		if err != nil {
			return err
		}
		if !destination.Valid() || !frozen.Valid() || destination.Scope != r.cfg.Scope || frozen.Binding.Scope != r.cfg.Scope {
			return ErrSource
		}
		for _, room := range destination.Rooms {
			if slices.Contains(frozen.Binding.Rooms, room) {
				return ErrSource
			}
		}
		policy := identityTransfer{target: r, source: source}
		err = catalogtransfer.Run(ctx, catalogtransfer.Config{
			Scope: r.cfg.Scope, Owner: r.owner(), Namespace: identityMigrationNamespace, ReceiptKind: migrationKind, Journal: r.cfg.Journal,
			Source: source.cfg.Crypto, Target: r.cfg.Crypto, FrozenBindings: proof.FrozenStorageBindings, Rules: &policy,
			PutObject: r.putObject, PutRecord: r.putRecord, CopyFile: copyFile,
			SourceFileCommitment: source.cfg.Crypto.StorageFileCommitment, TargetFileCommitment: r.cfg.Crypto.StorageFileCommitment,
			VerifyFile: func(ctx context.Context, ref storage.Reference, object, revision string) error {
				_, err := r.cfg.Crypto.DownloadStorageFile(ctx, r.cfg.Gateway, ref, object, revision, io.Discard)
				return err
			},
			AcknowledgeFile: func(ctx context.Context, job string, ref storage.Reference) error {
				return r.cfg.Crypto.AcknowledgeStorageFile(ctx, r.cfg.Journal, job, ref)
			},
			PrepareTarget: r.refresh, MarkReady: r.markReady,
		}, frozen, destination)
		if errors.Is(err, catalogtransfer.ErrSource) {
			return ErrSource
		}
		return err
	})
}

func (t *identityTransfer) transaction(ref storage.Reference, role string) string {
	return catalogtransfer.Transaction(t.state, ref, role)
}

func (t *identityTransfer) Classify(ctx context.Context, version storage.ObjectVersion, lookup catalogtransfer.Lookup) (catalogtransfer.Class, error) {
	var header struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(version.Commit.Metadata, &header) != nil {
		return catalogtransfer.Class{}, ErrSource
	}
	switch header.Kind {
	case bodyKind:
		return catalogtransfer.Class{Order: 0}, t.checkBody(ctx, version)
	case itemKind:
		return catalogtransfer.Class{Order: 1}, t.checkItem(ctx, version, lookup)
	case migrationKind:
		return catalogtransfer.Class{Order: 2}, t.checkCompletion(ctx, version)
	default:
		return catalogtransfer.Class{}, ErrSource
	}
}

func (t *identityTransfer) Rewrite(_ context.Context, _ storage.ObjectVersion, record storage.Record, resolve catalogtransfer.Resolve) ([]byte, error) {
	var header struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(record.Payload, &header) != nil {
		return nil, ErrSource
	}
	if header.Kind == entryKind {
		return rewriteTransferEntry(record.Payload, resolve)
	}
	if header.Kind != migrationKind {
		return nil, ErrSource
	}
	return bytes.Clone(record.Payload), nil
}

func (t *identityTransfer) SourceComplete(ctx context.Context) error {
	state, err := t.source.cfg.Source.PrivateStorageState(ctx, t.source.cfg.Scope)
	if err != nil {
		return err
	}
	if state.Validate(t.source.cfg.Scope) != nil || state.Mode == "legacy" {
		return ErrSource
	}
	if err := t.source.refresh(ctx); err != nil {
		return err
	}
	complete, err := t.source.completed(ctx, state)
	if err != nil {
		return err
	}
	if !complete {
		return ErrSource
	}
	t.legacy = state
	return nil
}

func (t *identityTransfer) TargetComplete(ctx context.Context) error {
	if err := t.target.refresh(ctx); err != nil {
		return err
	}
	complete, err := t.target.completed(ctx, t.legacy)
	if err != nil {
		return err
	}
	if !complete {
		return ErrSource
	}
	return nil
}

func (t *identityTransfer) CheckPending(_ context.Context, source storage.FrozenBinding, destination storage.Binding) error {
	return t.sourcePending(t.legacy, source, destination)
}

func rewriteTransferEntry(payload []byte, resolve catalogtransfer.Resolve) ([]byte, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil {
		return nil, ErrSource
	}
	for _, field := range []string{"metadata", "binary"} {
		raw, ok := fields[field]
		if !ok || bytes.Equal(raw, []byte("null")) {
			continue
		}
		var pointer bodyPointer
		var parts map[string]json.RawMessage
		if json.Unmarshal(raw, &pointer) != nil || json.Unmarshal(raw, &parts) != nil {
			return nil, ErrSource
		}
		next, err := resolve(pointer.Reference, pointer.ObjectID)
		if err != nil {
			return nil, err
		}
		parts["reference"], err = json.Marshal(next)
		if err != nil {
			return nil, err
		}
		fields[field], err = json.Marshal(parts)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}

func (t *identityTransfer) checkItem(ctx context.Context, version storage.ObjectVersion, lookup catalogtransfer.Lookup) error {
	item, err := decodeSummary(version)
	if err != nil {
		return err
	}
	if version.Commit.State == "deleted" {
		return nil
	}
	_, data, raw, err := t.source.readEntry(ctx, version)
	clear(raw)
	defer clear(data.Raw)
	if err != nil {
		return err
	}
	for _, pair := range []struct {
		pointer *bodyPointer
		purpose string
	}{{data.Metadata, "metadata"}, {data.Binary, "binary"}} {
		if pair.pointer == nil {
			continue
		}
		if _, found := lookup(pair.pointer.Reference); !found {
			return ErrSource
		}
		if _, _, err := t.source.bodyVersion(ctx, data.observation, *pair.pointer, item, pair.purpose); err != nil {
			return err
		}
	}
	return nil
}

func (t *identityTransfer) checkBody(ctx context.Context, version storage.ObjectVersion) error {
	var body bodySummary
	if json.Unmarshal(version.Commit.Metadata, &body) != nil || !validTransferBody(body, version.ObjectID) {
		return ErrSource
	}
	if version.Commit.State == "deleted" {
		return nil
	}
	pointer := version.Commit.Content
	if pointer == nil || pointer.Kind != "file" {
		return ErrSource
	}
	request, sum, err := t.source.cfg.Crypto.StorageFileCommitment(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return err
	}
	if request.Validate(t.source.cfg.Scope) != nil || len(request.Previous) != 0 || request.ObjectID != version.ObjectID || request.RevisionID != pointer.RevisionID || request.SizeBytes != body.SizeBytes || sum != body.SHA256 {
		return ErrSource
	}
	return nil
}

func (t *identityTransfer) checkCompletion(ctx context.Context, version storage.ObjectVersion) error {
	var value completion
	if json.Unmarshal(version.Commit.Metadata, &value) != nil || !storage.MigrationEpoch(value.Epoch) || !validHash(value.InventorySHA256) || value.Rows < 0 || value.Blobs < 0 || value.Blobs > value.Rows || version.ObjectID != "drive_migration_"+digest([]byte(value.Epoch)) || version.Commit.State != "live" {
		return ErrSource
	}
	content, err := t.source.readContent(ctx, version)
	defer clear(content.Payload)
	if err != nil {
		return err
	}
	if !bytes.Equal(content.Payload, version.Commit.Metadata) {
		return ErrSource
	}
	return nil
}

func (t *identityTransfer) sourcePending(state storage.LegacyMigrationState, source storage.FrozenBinding, destination storage.Binding) error {
	for _, namespace := range []string{migrationNamespace, storage.PendingUploadsNamespace} {
		records, err := t.source.cfg.Journal.List(t.source.owner(), namespace)
		if err != nil {
			return err
		}
		for _, record := range records {
			if err := t.checkPending(record.ID, record.Value, namespace, state, source, destination); err != nil {
				for _, value := range records {
					clear(value.Value)
				}
				return err
			}
		}
		for _, value := range records {
			clear(value.Value)
		}
	}
	return nil
}

func (t *identityTransfer) checkPending(key string, raw []byte, namespace string, state storage.LegacyMigrationState, source storage.FrozenBinding, destination storage.Binding) error {
	if namespace == migrationNamespace && strings.HasPrefix(key, "begin_") {
		var begin beginWork
		if json.Unmarshal(raw, &begin) != nil || begin.Scope != t.target.cfg.Scope || begin.Epoch != state.Epoch || !begin.Target.Valid() || !slices.Contains(source.Binding.Rooms, begin.Target.RoomID) {
			return ErrSource
		}
		return nil
	}
	var work struct {
		Target storage.PublicationTarget `json:"target"`
	}
	if json.Unmarshal(raw, &work) != nil || !work.Target.Valid() || slices.Contains(source.Binding.Rooms, work.Target.RoomID) || !slices.Contains(destination.Rooms, work.Target.RoomID) {
		return ErrSource
	}
	return nil
}

func validTransferBody(body bodySummary, object string) bool {
	return sourceKind(body.SourceKind) && storage.SourceCursorValid(storage.LegacySourceCursor{Kind: body.SourceKind, ID: body.SourceID}) && (body.Purpose == "binary" || body.Purpose == "metadata") && (body.Epoch == "" || storage.MigrationEpoch(body.Epoch)) && validHash(body.SHA256) && body.SizeBytes >= 0 && body.SizeBytes <= storage.MaxFileBytes && object == bodyID(body.SourceKind, body.SourceID, body.Purpose)
}
