// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/filesdk"
)

const (
	fileFanout           = filesdk.Fanout
	fileLevels           = filesdk.Levels
	maxFileBlocks        = filesdk.MaxBlocks
	MaxStorageFileBytes  = storage.MaxFileBytes
	maxFileMetadataBytes = storage.MaxFileMetadataBytes
	fileKind             = filesdk.ManifestKind
	filePageKind         = filesdk.PageKind
)

var (
	ErrStorageSourceChanged      = errors.New("matrix: storage source changed; original must be retained")
	ErrStoragePublicationMissing = errors.New("matrix: storage journal has no verified publication target; original must be retained")
	ErrStorageUploadMissing      = storage.ErrUploadMissing
)

// StorageFileRequest contains PRIVATE metadata. Journal and Matrix records are
// encrypted on the client; no name, MIME type, source hash or file key is sent
// to the blob service. Size is the exact immutable source revision being saved.
type StorageFileRequest = storage.FileRequest

// The shared codec preserves the existing encrypted manifest wire format.
type storageFileBlock = filesdk.Descriptor

func fileDescriptor(block StorageBlock, plainSHA string) *storageFileBlock {
	return filesdk.Describe(block, plainSHA)
}

type (
	storageFileLink = filesdk.Link
	storageFilePage = filesdk.Page
	storageFile     = filesdk.Manifest
)

type storageFileIO struct {
	scope            storage.Scope
	gateway          *api.Drive
	records          storageRecordIO
	journal          *offline.Store
	closed           func() bool
	publicationCheck func() error
	// A migrated immutable source carries an authenticated full-file commitment.
	expectedSourceSHA256 string
}

func (f storageFileIO) guard(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.closed != nil && f.closed() {
		return ErrClosed
	}
	if f.publicationCheck != nil {
		return f.publicationCheck()
	}
	return nil
}

func fileBlocks(size int64) int                  { return filesdk.BlockCount(size) }
func fileBlockBytes(size int64, index int) int64 { return filesdk.BlockBytes(size, index) }
func filePageSpan(level int) int                 { return filesdk.PageSpan(level) }
func fileRootLevel(count int) int                { return filesdk.RootLevel(count) }

func digestBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func validFileHash(value string) bool { return filesdk.ValidHash(value) }

func (f storageFileIO) readRecord(ctx context.Context, ref EventRef, request StorageFileRequest) (storage.Record, error) {
	if err := f.guard(ctx); err != nil {
		return storage.Record{}, err
	}
	rec, err := f.records.ReadStorageRecord(ctx, ref, request.ObjectID, "manifest")
	if err != nil {
		return rec, err
	}
	if rec.Validate(f.scope) != nil || rec.ObjectID != request.ObjectID || rec.RevisionID != request.RevisionID || rec.Role != "manifest" {
		return storage.Record{}, storage.ErrRecord
	}
	return rec, f.guard(ctx)
}

// saveRecord reads the maintained SDK receipt back before recording progress.
// An ambiguous network result is retried with exactly the same private payload
// and transaction; no original or journal is removed on failure.
func (f storageFileIO) saveRecord(ctx context.Context, txn string, request StorageFileRequest, payload []byte, previous []string) (EventRef, error) {
	if err := f.guard(ctx); err != nil {
		return EventRef{}, err
	}
	rec := storage.Record{Version: 1, Scope: f.scope, ObjectID: request.ObjectID, RevisionID: request.RevisionID, Role: "manifest", Payload: payload, Previous: previous}
	ref, err := writeStorageRecordVerified(ctx, f.records, txn, rec)
	if err == nil {
		err = f.guard(ctx)
	}
	return ref, err
}

func (f storageFileIO) root(ctx context.Context, ref EventRef, object, revision string) (storageFile, error) {
	request := StorageFileRequest{ObjectID: object, RevisionID: revision}
	rec, err := f.readRecord(ctx, ref, request)
	if err != nil {
		return storageFile{}, err
	}
	var file storageFile
	if json.Unmarshal(rec.Payload, &file) != nil || file.Validate(f.scope, object, revision) != nil || !slices.Equal(file.Request.Previous, rec.Previous) {
		return file, storage.ErrRecord
	}
	return file, nil
}

func validateFilePage(page storageFilePage, request StorageFileRequest, level, index int) error {
	return filesdk.ValidatePage(page, request, level, index)
}

func validFileEvent(ref EventRef) bool {
	return ref.Valid()
}

func (f storageFileIO) walk(ctx context.Context, file storageFile, page storageFilePage, level, index int, write func(int, storageFileBlock) error) error {
	if err := f.guard(ctx); err != nil {
		return err
	}
	if err := validateFilePage(page, file.Request, level, index); err != nil {
		return err
	}
	for j, link := range page.Nodes {
		next := index*fileFanout + j
		if level == 0 {
			if err := write(next, *link.Block); err != nil {
				return err
			}
		} else {
			rec, err := f.readRecord(ctx, *link.Event, file.Request)
			if err != nil {
				return err
			}
			var child storageFilePage
			if json.Unmarshal(rec.Payload, &child) != nil {
				return storage.ErrRecord
			}
			if err := f.walk(ctx, file, child, level-1, next, write); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f storageFileIO) download(ctx context.Context, ref EventRef, object, revision string, out io.Writer) (StorageFileRequest, error) {
	if out == nil || f.gateway == nil || f.records == nil {
		return StorageFileRequest{}, storage.ErrRecord
	}
	file, err := f.root(ctx, ref, object, revision)
	if err != nil {
		return StorageFileRequest{}, err
	}
	hash := sha256.New()
	err = f.walk(ctx, file, file.Root, file.Root.Level, 0, func(index int, descriptor storageFileBlock) error {
		block := descriptor.At(f.scope, object, revision, index)
		if block.Blob.SizeBytes != fileBlockBytes(file.Request.SizeBytes, index) {
			return storage.ErrRecord
		}
		data, err := downloadStorageBlock(ctx, f.gateway, f.scope, block, object, revision, index)
		if err != nil {
			return err
		}
		defer clear(data)
		if digestBytes(data) != descriptor.PlainSHA256 {
			return ErrIdentity
		}
		if err := f.guard(ctx); err != nil {
			return err
		}
		_, err = io.Copy(io.MultiWriter(out, hash), bytes.NewReader(data))
		return err
	})
	if err != nil {
		return StorageFileRequest{}, err
	}
	if hex.EncodeToString(hash.Sum(nil)) != file.SHA256 {
		return StorageFileRequest{}, ErrIdentity
	}
	return file.Request, f.guard(ctx)
}

// DownloadStorageFile authenticates every page, position and block, then checks
// the private whole-file hash. The destination must be a client-owned staging
// sink: publish it only after success; a failed download can contain an earlier
// authenticated prefix. No plaintext temporary is created by this adapter.
func (c *Client) DownloadStorageFile(ctx context.Context, gateway *api.Drive, ref EventRef, object, revision string, out io.Writer) (StorageFileRequest, error) {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return StorageFileRequest{}, err
	}
	return (storageFileIO{scope: scope, gateway: gateway, records: c, closed: c.closed.Load}).download(ctx, ref, object, revision, out)
}

// StorageFileInfo reads authenticated private metadata without downloading file
// bytes. The returned metadata must stay on the authorized device.
func (c *Client) StorageFileInfo(ctx context.Context, ref EventRef, object, revision string) (StorageFileRequest, error) {
	request, _, err := c.StorageFileCommitment(ctx, ref, object, revision)
	return request, err
}

// StorageFileCommitment returns authenticated private metadata and its complete
// plaintext digest without fetching media. It confirms a manifest's commitment,
// not current provider availability. Copies must retain shared ciphertext while
// any surviving file/version still references it; no original is retired here.
func (c *Client) StorageFileCommitment(ctx context.Context, ref EventRef, object, revision string) (StorageFileRequest, string, error) {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return StorageFileRequest{}, "", err
	}
	file, err := (storageFileIO{scope: scope, records: c, closed: c.closed.Load}).root(ctx, ref, object, revision)
	if err != nil {
		return StorageFileRequest{}, "", err
	}
	return file.Request, file.SHA256, nil
}
