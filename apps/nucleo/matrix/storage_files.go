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
	"unicode/utf8"

	"maunium.net/go/mautrix/crypto/attachment"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	fileFanout           = 16
	fileLevels           = 6
	maxFileBlocks        = 1 << 24
	MaxStorageFileBytes  = int64(maxFileBlocks) * storage.MaxCipherBlockBytes
	maxFileMetadataBytes = 8 << 10
	fileKind             = "org.sytue.file.v1"
	filePageKind         = "org.sytue.file.page.v1"
)

var ErrStorageSourceChanged = errors.New("matrix: storage source changed; original must be retained")

// StorageFileRequest contains PRIVATE metadata. Journal and Matrix records are
// encrypted on the client; no name, MIME type, source hash or file key is sent
// to the blob service. Size is the exact immutable source revision being saved.
type StorageFileRequest struct {
	ObjectID   string   `json:"object_id"`
	RevisionID string   `json:"revision_id"`
	SizeBytes  int64    `json:"size_bytes"`
	Metadata   []byte   `json:"metadata"`
	Previous   []string `json:"previous,omitempty"`
}

// A page's authenticated record already binds scope, object and revision.
// Do not repeat those values sixteen times in its private leaf descriptors.
type storageFileBlock struct {
	Blob        storage.BlobReceipt      `json:"blob"`
	File        attachment.EncryptedFile `json:"matrix_file"`
	PlainSHA256 string                   `json:"plain_sha256"`
}

func fileDescriptor(block StorageBlock, plainSHA string) *storageFileBlock {
	return &storageFileBlock{Blob: block.Blob, File: block.File, PlainSHA256: plainSHA}
}

func (b storageFileBlock) at(scope storage.Scope, object, revision string, part int) StorageBlock {
	return StorageBlock{Scope: scope, ObjectID: object, RevisionID: revision, Part: part, Blob: b.Blob, File: b.File}
}

type storageFileLink struct {
	Block *storageFileBlock `json:"block,omitempty"`
	Event *EventRef         `json:"event,omitempty"`
}

type storageFilePage struct {
	Kind  string            `json:"kind"`
	Level int               `json:"level"`
	Index int               `json:"index"`
	Nodes []storageFileLink `json:"nodes"`
}

type storageFile struct {
	Kind    string             `json:"kind"`
	Request StorageFileRequest `json:"request"`
	SHA256  string             `json:"sha256"`
	Root    storageFilePage    `json:"root"`
}

type storageFileIO struct {
	scope   storage.Scope
	gateway *api.Drive
	records storageRecordIO
	journal *offline.Store
	closed  func() bool
}

func (f storageFileIO) guard(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.closed != nil && f.closed() {
		return ErrClosed
	}
	return nil
}

func (r StorageFileRequest) validate(scope storage.Scope) error {
	if !scope.Valid() || r.SizeBytes < 0 || r.SizeBytes > MaxStorageFileBytes || len(r.Metadata) > maxFileMetadataBytes || !utf8.Valid(r.Metadata) || !json.Valid(r.Metadata) {
		return storage.ErrRecord
	}
	return (storage.Record{Version: 1, Scope: scope, ObjectID: r.ObjectID, RevisionID: r.RevisionID, Role: "manifest", Payload: []byte(`{}`), Previous: r.Previous}).Validate(scope)
}

func fileBlocks(size int64) int {
	if size == 0 {
		return 1
	}
	return int(1 + (size-1)/storage.MaxCipherBlockBytes)
}

func fileBlockBytes(size int64, index int) int64 {
	return min(storage.MaxCipherBlockBytes, max(int64(0), size-int64(index)*storage.MaxCipherBlockBytes))
}

func filePageSpan(level int) int {
	span := 1
	for range level + 1 {
		span *= fileFanout
	}
	return span
}

func fileRootLevel(count int) int {
	level := 0
	for count > filePageSpan(level) {
		level++
	}
	return level
}

func digestBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func validFileHash(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && hex.EncodeToString(raw) == value
}

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
	if json.Unmarshal(rec.Payload, &file) != nil || file.Kind != fileKind || file.Request.validate(f.scope) != nil || file.Request.ObjectID != object || file.Request.RevisionID != revision || !validFileHash(file.SHA256) || !slices.Equal(file.Request.Previous, rec.Previous) {
		return file, storage.ErrRecord
	}
	if err := validateFilePage(file.Root, file.Request, fileRootLevel(fileBlocks(file.Request.SizeBytes)), 0); err != nil {
		return file, err
	}
	return file, nil
}

func validateFilePage(page storageFilePage, request StorageFileRequest, level, index int) error {
	count := fileBlocks(request.SizeBytes)
	if level < 0 || level >= fileLevels || index < 0 || index > count || page.Kind != filePageKind || page.Level != level || page.Index != index {
		return storage.ErrRecord
	}
	span := filePageSpan(level)
	if index > (count-1)/span {
		return storage.ErrRecord
	}
	first := index * span
	childSpan := span / fileFanout
	wanted := 1 + (min(span, count-first)-1)/childSpan
	if len(page.Nodes) != wanted {
		return storage.ErrRecord
	}
	for _, node := range page.Nodes {
		if level == 0 {
			if node.Block == nil || node.Event != nil || !validFileHash(node.Block.PlainSHA256) {
				return storage.ErrRecord
			}
		} else if node.Block != nil || node.Event == nil || !validFileEvent(*node.Event) {
			return storage.ErrRecord
		}
	}
	return nil
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
		block := descriptor.at(f.scope, object, revision, index)
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
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return StorageFileRequest{}, err
	}
	file, err := (storageFileIO{scope: scope, records: c, closed: c.closed.Load}).root(ctx, ref, object, revision)
	if err != nil {
		return StorageFileRequest{}, err
	}
	return file.Request, nil
}
