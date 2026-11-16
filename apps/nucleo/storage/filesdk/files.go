// SPDX-License-Identifier: Apache-2.0
// Package filesdk binds the maintained native SDK's existing encrypted file
// implementation to its real scoped gateway and journal.
package filesdk

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type SDK interface {
	storage.RecordStore
	StorageFileInfo(context.Context, storage.Reference, string, string) (storage.FileRequest, error)
	StorageFileCommitment(context.Context, storage.Reference, string, string) (storage.FileRequest, string, error)
	DownloadStorageFile(context.Context, *api.Drive, storage.Reference, string, string, io.Writer) (storage.FileRequest, error)
	UploadStorageFileAt(context.Context, *api.Drive, *offline.Store, string, storage.PublicationTarget, storage.FileRequest, io.ReaderAt) (storage.Reference, error)
	AcknowledgeStorageFile(context.Context, *offline.Store, string, storage.Reference) error
}
type Files struct {
	scope   storage.Scope
	sdk     SDK
	gateway *api.Drive
	journal *offline.Store
}

func New(scope storage.Scope, sdk SDK, gateway *api.Drive, journal *offline.Store) (*Files, error) {
	if !scope.Valid() || sdk == nil || gateway == nil || journal == nil || !journal.Encrypted() {
		return nil, storage.ErrRecord
	}
	return &Files{scope, sdk, gateway, journal}, nil
}
func (f *Files) FileScope() storage.Scope { return f.scope }
func (f *Files) Prepare(ctx context.Context) error {
	binding, err := f.sdk.StorageBindings(ctx)
	if err != nil {
		return err
	}
	if !binding.Valid() || binding.Scope != f.scope {
		return storage.ErrCatalogSource
	}
	return nil
}

func (f *Files) Info(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, error) {
	return f.sdk.StorageFileInfo(ctx, ref, object, revision)
}

func (f *Files) Commitment(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, string, error) {
	return f.sdk.StorageFileCommitment(ctx, ref, object, revision)
}

func (f *Files) Download(ctx context.Context, ref storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	return f.sdk.DownloadStorageFile(ctx, f.gateway, ref, object, revision, out)
}

func (f *Files) Upload(ctx context.Context, job string, target storage.PublicationTarget, request storage.FileRequest, source io.ReaderAt) (storage.Reference, error) {
	return f.sdk.UploadStorageFileAt(ctx, f.gateway, f.journal, job, target, request, source)
}

func (f *Files) Acknowledge(ctx context.Context, job string, ref storage.Reference) error {
	return f.sdk.AcknowledgeStorageFile(ctx, f.journal, job, ref)
}

var _ storage.BoundFiles = (*Files)(nil)

// AtomicFileAcknowledger is optional until a scoped adapter supports the new
// complete-batch proof. Existing BoundFiles callers keep their exact contract.
type AtomicFileAcknowledger interface {
	AcknowledgeAtomicStorageFile(context.Context, *offline.Store, string, storage.VerifiedAtomicFileCompletion) error
}

func (f *Files) AcknowledgeAtomic(ctx context.Context, job string, completion storage.VerifiedAtomicFileCompletion) error {
	if !completion.MatchesScope(f.scope) {
		return storage.ErrRecord
	}
	sdk, ok := f.sdk.(AtomicFileAcknowledger)
	if !ok {
		return storage.ErrRecord
	}
	return sdk.AcknowledgeAtomicStorageFile(ctx, f.journal, job, completion)
}
