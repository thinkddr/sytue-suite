// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// RetainedFileCatalog authenticates linked files without fetching their bodies.
// The engine supplies it during final validation and on ordinary ready views.
type RetainedFileCatalog interface {
	Catalog
	ValidateRetainedFile(context.Context, RetainedFile) error
}

// RetainedFileContentCatalog permits domain graph validation to read linked
// checkpoint pages before readiness. It uses the same authenticated SDK reader
// as ordinary ready views; it exposes no publication or acknowledgement.
type RetainedFileContentCatalog interface {
	RetainedFileCatalog
	ReadRetainedFile(context.Context, RetainedFile, io.Writer) error
}

func (v catalogView) ValidateRetainedFile(ctx context.Context, file RetainedFile) error {
	if v.requireReady {
		work, done, err := v.r.Start(ctx)
		if err != nil {
			return err
		}
		defer done()
		ctx = work
		if err := v.r.requireReady(); err != nil {
			return err
		}
	}
	return v.r.validateRetainedFile(ctx, file)
}

func (v catalogView) ReadRetainedFile(ctx context.Context, file RetainedFile, out io.Writer) error {
	if v.requireReady {
		work, done, err := v.r.Start(ctx)
		if err != nil {
			return err
		}
		defer done()
		ctx = work
		if err := v.r.requireReady(); err != nil {
			return err
		}
	}
	return v.r.readRetainedFile(ctx, file, out)
}

func (r *Repository) ValidateRetainedFile(parent context.Context, file RetainedFile) error {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	if err := r.requireReady(); err != nil {
		return err
	}
	return r.validateRetainedFile(ctx, file)
}

func (r *Repository) retainedFileRequest(ctx context.Context, file RetainedFile) (storage.ContentPointer, storage.FileRequest, error) {
	if !r.retainedFileShape(file) {
		return storage.ContentPointer{}, storage.FileRequest{}, ErrSource
	}
	if err := r.cfg.ValidateBinary(ctx, file.Row); err != nil {
		return storage.ContentPointer{}, storage.FileRequest{}, err
	}
	r.mu.Lock()
	binding := r.binding
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return storage.ContentPointer{}, storage.FileRequest{}, ErrClosed
	}
	if !binding.Valid() || binding.Scope != r.cfg.Scope || !slices.Contains(binding.Rooms, file.Target.RoomID) {
		return storage.ContentPointer{}, storage.FileRequest{}, ErrSource
	}
	object := r.object(file.Row.Kind, file.Row.ID)
	record, err := r.cfg.Crypto.ReadStorageRecord(ctx, file.Link, object, "content")
	if err != nil {
		return storage.ContentPointer{}, storage.FileRequest{}, err
	}
	link, err := storage.DecodeFileLink(record, r.cfg.Scope)
	if err != nil || record.ObjectID != object || record.RevisionID != file.Revision || link.Content.RevisionID != file.Revision || link.Content.Reference.RoomID != file.Target.RoomID {
		return storage.ContentPointer{}, storage.FileRequest{}, ErrSource
	}
	metadata, _ := json.Marshal(retainedFileHeader{Kind: retainedFileKind, EntityKind: r.cfg.EntityKind, Scope: r.cfg.Scope, Target: file.Target, Row: file.Row})
	return link.Content, storage.FileRequest{ObjectID: object, RevisionID: file.Revision, SizeBytes: file.Row.Size, Metadata: metadata}, nil
}

func sameRetainedFileRequest(scope storage.Scope, expected, actual storage.FileRequest) bool {
	return actual.Validate(scope) == nil && len(actual.Previous) == 0 && actual.ObjectID == expected.ObjectID && actual.RevisionID == expected.RevisionID && actual.SizeBytes == expected.SizeBytes && bytes.Equal(actual.Metadata, expected.Metadata)
}

func (r *Repository) validateRetainedFile(ctx context.Context, file RetainedFile) error {
	pointer, expected, err := r.retainedFileRequest(ctx, file)
	if err != nil {
		return err
	}
	return r.checkRetainedCommitment(ctx, file, pointer, expected)
}

func (r *Repository) checkRetainedCommitment(ctx context.Context, file RetainedFile, pointer storage.ContentPointer, expected storage.FileRequest) error {
	sdk, ok := r.cfg.Crypto.(binaryCommitmentReader)
	if !ok {
		return ErrSource
	}
	actual, digest, err := sdk.StorageFileCommitment(ctx, pointer.Reference, expected.ObjectID, expected.RevisionID)
	if err != nil {
		return err
	}
	if !sameRetainedFileRequest(r.cfg.Scope, expected, actual) || digest != file.Row.SHA256 {
		return ErrSource
	}
	return ctx.Err()
}

// ReadRetainedFile joins the ordinary session gate and authenticates every byte
// through the maintained SDK. Closing/canceling its caller cancels that SDK read.
func (r *Repository) ReadRetainedFile(parent context.Context, file RetainedFile, out io.Writer) error {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	if err := r.requireReady(); err != nil {
		return err
	}
	return r.readRetainedFile(ctx, file, out)
}

func (r *Repository) readRetainedFile(ctx context.Context, file RetainedFile, out io.Writer) error {
	if out == nil {
		return ErrSource
	}
	pointer, expected, err := r.retainedFileRequest(ctx, file)
	if err != nil {
		return err
	}
	if err := r.checkRetainedCommitment(ctx, file, pointer, expected); err != nil {
		return err
	}
	sink := &binarySink{out: out, remaining: file.Row.Size, sum: sha256.New()}
	actual, err := r.cfg.Crypto.DownloadStorageFile(ctx, r.cfg.Gateway, pointer.Reference, expected.ObjectID, expected.RevisionID, sink)
	if err != nil {
		return err
	}
	if !sameRetainedFileRequest(r.cfg.Scope, expected, actual) || sink.remaining != 0 {
		return ErrSource
	}
	if hex.EncodeToString(sink.sum.Sum(nil)) != file.Row.SHA256 {
		return ErrSource
	}
	return ctx.Err()
}

func (r *Repository) retainedFileShape(file RetainedFile) bool {
	return file.Version == 1 && file.Scope == r.cfg.Scope && file.Target.Valid() && file.Link.RoomID == file.Target.RoomID && file.Link.Valid() && storage.Identifier(file.Revision) && file.Row.Validate() == nil && r.binaryKind(file.Row.Kind) && r.cfg.ValidateBinary != nil
}
