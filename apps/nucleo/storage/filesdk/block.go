// SPDX-License-Identifier: Apache-2.0
package filesdk

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"

	"maunium.net/go/mautrix/crypto/attachment"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Block is PRIVATE manifest data. Its Matrix descriptor contains a key:
// persist it only inside an encrypted Record or the encrypted local cache, never
// in the ciphertext gateway's metadata or a server-side task argument.
type Block struct {
	Scope      storage.Scope            `json:"scope"`
	ObjectID   string                   `json:"object_id"`
	RevisionID string                   `json:"revision_id"`
	Part       int                      `json:"part"`
	Blob       storage.BlobReceipt      `json:"blob"`
	File       attachment.EncryptedFile `json:"matrix_file"`
}

// ReadBlockSource reads exactly one bounded source without truncation.
func ReadBlockSource(source io.Reader, size int64) ([]byte, error) {
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

// PrepareBlock encrypts with the maintained SDK before any gateway IO.
func PrepareBlock(scope storage.Scope, object, revision string, part int, source io.Reader, size int64) (Block, []byte, error) {
	if !scope.Valid() || !storage.Identifier(object) || !storage.Identifier(revision) || part < 0 {
		return Block{}, nil, storage.ErrRecord
	}
	data, err := ReadBlockSource(source, size)
	if err != nil {
		return Block{}, nil, err
	}
	file := attachment.NewEncryptedFile()
	file.EncryptInPlace(data)
	hash, err := base64.RawStdEncoding.Strict().DecodeString(file.Hashes.SHA256)
	if err != nil || len(hash) != 32 {
		clear(data)
		return Block{}, nil, storage.ErrRecord
	}
	block := Block{
		Scope: scope, ObjectID: object, RevisionID: revision, Part: part,
		Blob: storage.BlobReceipt{ID: rand.Text(), Product: scope.Product, SizeBytes: size, SHA256: hex.EncodeToString(hash)},
		File: attachment.EncryptedFile{Key: file.Key, InitVector: file.InitVector, Hashes: file.Hashes, Version: file.Version},
	}
	if err := block.Validate(scope, object, revision, part); err != nil {
		clear(data)
		return Block{}, nil, err
	}
	return block, data, nil
}

// Validate checks the private descriptor against its original file context.
func (b Block) Validate(scope storage.Scope, object, revision string, part int) error {
	if !scope.Valid() || b.File.Version != "v2" || b.Scope != scope || b.ObjectID != object || b.RevisionID != revision || b.Part != part || part < 0 || !storage.Identifier(object) || !storage.Identifier(revision) || !b.Blob.Valid(scope.Product, b.Blob.ID) {
		return storage.ErrRecord
	}
	hash, err := base64.RawStdEncoding.Strict().DecodeString(b.File.Hashes.SHA256)
	if err != nil || len(hash) != 32 || hex.EncodeToString(hash) != b.Blob.SHA256 {
		return storage.ErrRecord
	}
	return b.File.PrepareForDecryption()
}

// DecryptBlockInPlace authenticates the complete bounded block through the
// maintained SDK before a caller can publish plaintext to a viewer. A failed
// context, length or SDK check clears the caller-owned output buffer.
func DecryptBlockInPlace(scope storage.Scope, object, revision string, part int, block Block, ciphertext []byte) error {
	if err := block.Validate(scope, object, revision, part); err != nil {
		clear(ciphertext)
		return err
	}
	if int64(len(ciphertext)) != block.Blob.SizeBytes {
		clear(ciphertext)
		return storage.ErrRecord
	}
	if err := block.File.DecryptInPlace(ciphertext); err != nil {
		clear(ciphertext)
		return err
	}
	return nil
}
