// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"context"
	"io"
)

// RecordStore is the maintained SDK's authenticated private-record and catalog
// surface. Applications supply domain JSON; keys, encryption, signature checks
// and device trust remain in its implementation. File adapters add their own
// bound gateway/journal methods without introducing those dependencies here.
type RecordStore interface {
	CatalogReader
	PrepareStorage(context.Context, Scope) (Binding, error)
	CheckStoragePublicationTarget(PublicationTarget) error
	PutStorageRecordAt(context.Context, PublicationTarget, string, Record) (Reference, error)
	ReadStorageRecord(context.Context, Reference, string, string) (Record, error)
	PutStorageObjectAt(context.Context, PublicationTarget, string, string, string, ObjectCommit) (ObjectVersion, error)
	StorageObject(context.Context, Reference, string) (ObjectVersion, error)
}

// BoundFiles is an explicitly scoped SDK/gateway/journal capability. Native
// adapters bind the real Drive client and sealed store; browser adapters bind
// StorageRepository and its encrypted journal. It never supplies a dummy HTTP
// gateway, a plaintext cache or a new cipher implementation.
type BoundFiles interface {
	FileScope() Scope
	Prepare(context.Context) error
	Info(context.Context, Reference, string, string) (FileRequest, error)
	Commitment(context.Context, Reference, string, string) (FileRequest, string, error)
	Download(context.Context, Reference, string, string, io.Writer) (FileRequest, error)
	Upload(context.Context, string, PublicationTarget, FileRequest, io.ReaderAt) (Reference, error)
	Acknowledge(context.Context, string, Reference) error
}

// PublicationPolicy is a live authorization callback retained by an SDK
// publication context. It carries no concrete native SDK dependency.
type PublicationPolicy func(context.Context) error

// HistoricalPublicationProjector is implemented by an SDK's explicit historical
// writer. It is called only AFTER that writer granted its own live publication
// policy. It removes only foreign destination intents while preserving the
// original cancellation, lease and source authority; it never grants a writer.
type HistoricalPublicationProjector interface {
	HistoricalStoragePublicationContext(context.Context) (context.Context, error)
}
