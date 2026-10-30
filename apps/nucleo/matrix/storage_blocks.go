// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"sync"

	"maunium.net/go/mautrix/crypto/attachment"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// StorageBlock is PRIVATE manifest data. Its Matrix descriptor contains a key:
// persist it only inside an encrypted Record or the encrypted local cache, never
// in the ciphertext gateway's metadata or a server-side task argument.
type StorageBlock struct {
	Scope      storage.Scope            `json:"scope"`
	ObjectID   string                   `json:"object_id"`
	RevisionID string                   `json:"revision_id"`
	Part       int                      `json:"part"`
	Blob       storage.BlobReceipt      `json:"blob"`
	File       attachment.EncryptedFile `json:"matrix_file"`
}

func (c *Client) storageBlockScope(ctx context.Context) (storage.Scope, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	scope := c.storageScope()
	if !storage.Product(scope.Product) {
		return storage.Scope{}, ErrIdentity
	}
	if err := c.requireEncrypted(ctx, c.cfg.Bootstrap.StorageRoomID); err != nil {
		return storage.Scope{}, err
	}
	return scope, nil
}

// UploadStorageBlock encrypts through the maintained Matrix file codec before
// calling the suite gateway. Memory is bounded and no source bytes are cached by
// this adapter. A caller keeps its original until the encrypted manifest is read
// back and verified; an upload receipt alone does not complete a migration.
func (c *Client) UploadStorageBlock(ctx context.Context, gateway *api.Drive, object, revision string, part int, source io.Reader, size int64) (StorageBlock, error) {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return StorageBlock{}, err
	}
	block, err := uploadStorageBlock(ctx, gateway, scope, object, revision, part, source, size)
	if err != nil {
		return StorageBlock{}, err
	}
	if c.closed.Load() {
		return StorageBlock{}, ErrClosed
	}
	return block, nil
}

func uploadStorageBlock(ctx context.Context, gateway *api.Drive, scope storage.Scope, object, revision string, part int, source io.Reader, size int64) (StorageBlock, error) {
	block, data, err := prepareStorageBlock(scope, object, revision, part, source, size)
	if err != nil {
		return StorageBlock{}, err
	}
	defer clear(data)
	if err := putPreparedStorageBlock(ctx, gateway, block, data); err != nil {
		return StorageBlock{}, err
	}
	return block, nil
}

func storageBytes(source io.Reader, size int64) ([]byte, error) {
	if source == nil || size < 0 || size > storage.MaxCipherBlockBytes {
		return nil, storage.ErrRecord
	}
	data, err := io.ReadAll(io.LimitReader(source, size+1))
	if err != nil || int64(len(data)) != size {
		clear(data)
		return nil, errors.Join(storage.ErrRecord, err)
	}
	return data, nil
}

func prepareStorageBlock(scope storage.Scope, object, revision string, part int, source io.Reader, size int64) (StorageBlock, []byte, error) {
	if !storage.Identifier(object) || !storage.Identifier(revision) || part < 0 {
		return StorageBlock{}, nil, storage.ErrRecord
	}
	data, err := storageBytes(source, size)
	if err != nil {
		return StorageBlock{}, nil, err
	}
	file := attachment.NewEncryptedFile()
	file.EncryptInPlace(data)
	hash, err := base64.RawStdEncoding.Strict().DecodeString(file.Hashes.SHA256)
	if err != nil || len(hash) != 32 {
		clear(data)
		return StorageBlock{}, nil, storage.ErrRecord
	}
	block := StorageBlock{
		Scope: scope, ObjectID: object, RevisionID: revision, Part: part,
		Blob: storage.BlobReceipt{ID: rand.Text(), Product: scope.Product, SizeBytes: size, SHA256: hex.EncodeToString(hash)},
		File: attachment.EncryptedFile{Key: file.Key, InitVector: file.InitVector, Hashes: file.Hashes, Version: file.Version},
	}
	if err := block.validate(scope, object, revision, part); err != nil {
		clear(data)
		return StorageBlock{}, nil, err
	}
	return block, data, nil
}

func putPreparedStorageBlock(ctx context.Context, gateway *api.Drive, block StorageBlock, data []byte) error {
	if gateway == nil || int64(len(data)) != block.Blob.SizeBytes {
		return storage.ErrRecord
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != block.Blob.SHA256 {
		return storage.ErrRecord
	}
	reader := &storageCipherReader{body: bytes.NewReader(data)}
	defer func() { _ = reader.Close() }()
	var body io.Reader = reader
	if len(data) == 0 {
		body = http.NoBody
	}
	receipt, err := gateway.PutCipherBlob(ctx, block.Scope.Product, block.Blob.ID, body, int64(len(data)))
	if err != nil {
		return err
	}
	if receipt != block.Blob {
		return storage.ErrRecord
	}
	return nil
}

// net/http may close a request body asynchronously after an early response. The
// uploader clears its ciphertext buffer on return, so close our reader first and
// serialize against active/future reads rather than racing the transport.
type storageCipherReader struct {
	mu   sync.Mutex
	body *bytes.Reader
}

func (r *storageCipherReader) Read(out []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.body == nil {
		return 0, io.EOF
	}
	return r.body.Read(out)
}

func (r *storageCipherReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.body = nil
	return nil
}

func (b StorageBlock) validate(scope storage.Scope, object, revision string, part int) error {
	if !scope.Valid() || b.File.Version != "v2" || b.Scope != scope || b.ObjectID != object || b.RevisionID != revision || b.Part != part || part < 0 || !storage.Identifier(object) || !storage.Identifier(revision) || !b.Blob.Valid(scope.Product, b.Blob.ID) {
		return storage.ErrRecord
	}
	hash, err := base64.RawStdEncoding.Strict().DecodeString(b.File.Hashes.SHA256)
	if err != nil || len(hash) != 32 || hex.EncodeToString(hash) != b.Blob.SHA256 {
		return storage.ErrRecord
	}
	return b.File.PrepareForDecryption()
}

// DownloadStorageBlock authenticates the entire bounded ciphertext block before
// returning plaintext. Substitution across object, revision, scope or position is
// rejected before making a request; no unauthenticated bytes reach a viewer.
func (c *Client) DownloadStorageBlock(ctx context.Context, gateway *api.Drive, block StorageBlock, object, revision string, part int) ([]byte, error) {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return nil, err
	}
	data, err := downloadStorageBlock(ctx, gateway, scope, block, object, revision, part)
	if err != nil {
		return nil, err
	}
	if c.closed.Load() {
		clear(data)
		return nil, ErrClosed
	}
	return data, nil
}

func downloadStorageBlock(ctx context.Context, gateway *api.Drive, scope storage.Scope, block StorageBlock, object, revision string, part int) ([]byte, error) {
	if gateway == nil {
		return nil, storage.ErrRecord
	}
	if validationErr := block.validate(scope, object, revision, part); validationErr != nil {
		return nil, validationErr
	}
	body, err := gateway.ReadCipherBlob(ctx, scope.Product, block.Blob.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(io.LimitReader(body, block.Blob.SizeBytes+1))
	if err != nil {
		clear(data)
		return nil, err
	}
	if int64(len(data)) != block.Blob.SizeBytes {
		clear(data)
		return nil, ErrIdentity
	}
	if decryptErr := block.File.DecryptInPlace(data); decryptErr != nil {
		clear(data)
		return nil, decryptErr
	}
	return data, nil
}
