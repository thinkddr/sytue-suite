// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// MigrationStorage keeps the historical reader read-only. Its separate writer
// can be used only with the original Go source publication context. A frozen
// resume may omit the writer; that absence never falls back to ordinary writes.
type MigrationStorage struct {
	*Storage
	writer *Storage
}

func NewMigrationStorage(reader, writer *Storage) (*MigrationStorage, error) {
	if reader == nil || reader.StorageIdentityVersion() != storage.LegacyIdentity {
		return nil, ErrRejected
	}
	if writer != nil && (writer.StorageIdentityVersion() != storage.LegacyIdentity || reader.identity != writer.identity || reader.port.ID() == writer.port.ID()) {
		return nil, ErrRejected
	}
	return &MigrationStorage{Storage: reader, writer: writer}, nil
}

func (c *MigrationStorage) RequireAuthorizedStoragePublication(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, policy storage.PublicationPolicy) (context.Context, error) {
	if c.writer == nil || policy == nil || scope != c.identity.Scope {
		return nil, ErrRejected
	}
	// Preparing a reader never touches the writer. Minting a source capability
	// checks both real SDK bindings, including the original target/device.
	before, err := c.StorageBindings(ctx)
	if err != nil {
		return nil, err
	}
	current, err := c.writer.PrepareStorage(ctx, scope)
	if err != nil {
		return nil, err
	}
	if !before.Same(current) || target != before.Target {
		return nil, ErrRejected
	}
	return c.writer.RequireAuthorizedStoragePublication(ctx, scope, target, policy)
}

func (c *MigrationStorage) publication(ctx context.Context, target storage.PublicationTarget) error {
	proof, ok := ctx.Value(publicationContext{}).(publicationProof)
	if c.writer == nil || !ok || proof.owner != c.writer || proof.policy == nil || proof.scope != c.identity.Scope || proof.target != target {
		return ErrRejected
	}
	return ctx.Err()
}

func (c *MigrationStorage) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (storage.Reference, error) {
	if err := c.publication(ctx, target); err != nil {
		return storage.Reference{}, err
	}
	return c.writer.PutStorageRecordAt(ctx, target, transaction, record)
}

func (c *MigrationStorage) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	if err := c.publication(ctx, target); err != nil {
		return storage.ObjectVersion{}, err
	}
	return c.writer.PutStorageObjectAt(ctx, target, transaction, object, revision, commit)
}

func (c *MigrationStorage) Upload(ctx context.Context, job string, target storage.PublicationTarget, request storage.FileRequest, source io.ReaderAt) (storage.Reference, error) {
	if err := c.publication(ctx, target); err != nil {
		return storage.Reference{}, err
	}
	return c.writer.Upload(ctx, job, target, request, source)
}

func (c *MigrationStorage) Acknowledge(ctx context.Context, job string, reference storage.Reference) error {
	if c.writer == nil {
		return ErrRejected
	}
	return c.writer.Acknowledge(ctx, job, reference)
}

var (
	_ storage.RecordStore = (*MigrationStorage)(nil)
	_ storage.BoundFiles  = (*MigrationStorage)(nil)
)

// HistoricalStoragePublicationContext never turns the reader into a writer.
// Both original opaque ports must retain the same authenticated SDK binding
// before and after revalidating the already-authorized source publication.
func (c *MigrationStorage) HistoricalStoragePublicationContext(ctx context.Context) (context.Context, error) {
	if c == nil || c.writer == nil {
		return nil, ErrRejected
	}
	proof, ok := ctx.Value(publicationContext{}).(publicationProof)
	if !ok || c.publication(ctx, proof.target) != nil {
		return nil, ErrRejected
	}
	before, err := c.StorageBindings(ctx)
	if err != nil {
		return nil, err
	}
	bound, err := c.writer.HistoricalStoragePublicationContext(ctx)
	if err != nil {
		return nil, err
	}
	writer, err := c.writer.StorageBindings(bound)
	if err != nil {
		return nil, err
	}
	reader, err := c.StorageBindings(bound)
	if err != nil {
		return nil, err
	}
	if !before.Same(writer) || !before.Same(reader) || c.writer.CheckStoragePublicationTarget(proof.target) != nil {
		return nil, ErrRejected
	}
	return bound, nil
}

var _ storage.HistoricalPublicationProjector = (*MigrationStorage)(nil)
