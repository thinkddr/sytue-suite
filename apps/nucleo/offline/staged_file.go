// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package offline

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const stagedBlockBytes = 256 << 10

var ErrStagedFile = errors.New("offline: staged file is missing, changed or incomplete")

// StagedFile describes immutable input in the existing encrypted record store.
// Its owner and namespace must come from the authenticated application, never
// from the descriptor. Completion is written only after exact size and EOF.
type StagedFile struct {
	ID        string `json:"id"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

func (f StagedFile) valid() bool {
	sum, err := hex.DecodeString(f.SHA256)
	return storage.Identifier(f.ID) && f.SizeBytes >= 0 && f.SizeBytes <= storage.MaxFileBytes && err == nil && len(sum) == sha256.Size
}

func (f StagedFile) blocks() int64          { return (f.SizeBytes + stagedBlockBytes - 1) / stagedBlockBytes }
func (f StagedFile) blockID(n int64) string { return fmt.Sprintf("%s_%d", f.ID, n) }

// StageFile makes non-seekable OS inputs resumable without plaintext temporary
// files or an in-memory copy of the complete input. A failed capture is never a
// durable upload intent; callers retain the original until cloud confirmation.
func (s *Store) StageFile(ctx context.Context, owner, namespace string, input io.Reader, size int64) (StagedFile, error) {
	return s.stageFile(ctx, owner, namespace, input, size, true)
}

// StageStream captures an input whose length is not known in advance. It uses
// exactly the same authenticated blocks and completion record as StageFile.
// maxBytes is a hard bound, never an initial allocation. No prefix is published
// as complete after cancellation, overflow, read failure or a failed write.
// As with StageFile, the caller owns input and must make blocked reads cancelable.
func (s *Store) StageStream(ctx context.Context, owner, namespace string, input io.Reader, maxBytes int64) (StagedFile, error) {
	return s.stageFile(ctx, owner, namespace, input, maxBytes, false)
}

func (s *Store) stageFile(ctx context.Context, owner, namespace string, input io.Reader, size int64, exact bool) (StagedFile, error) {
	if !s.Encrypted() {
		return StagedFile{}, ErrEncryptedStore
	}
	if input == nil || size < 0 || size > storage.MaxFileBytes {
		return StagedFile{}, ErrStagedFile
	}
	file := StagedFile{ID: rand.Text(), SizeBytes: size}
	if err := s.captureFile(ctx, owner, namespace, file, input, exact); err != nil {
		return StagedFile{}, err
	}
	var complete StagedFile
	found, err := s.Read(owner, namespace, file.ID, &complete)
	if err != nil || !found || !complete.valid() {
		return StagedFile{}, ErrStagedFile
	}
	return complete, nil
}

func (s *Store) captureFile(ctx context.Context, owner, namespace string, file StagedFile, input io.Reader, exact bool) (err error) {
	var written int64
	defer func() {
		if err != nil {
			for n := int64(0); n < written; n++ {
				_ = s.discardStagedRecord(owner, namespace, file.blockID(n))
			}
		}
	}()
	buffer := make([]byte, stagedBlockBytes)
	defer clear(buffer)
	hash := sha256.New()
	var offset int64
	ended := false
	for offset < file.SizeBytes {
		if err := ctx.Err(); err != nil {
			return err
		}
		wanted := int(min(int64(len(buffer)), file.SizeBytes-offset))
		n, end, err := readStagedBlock(input, buffer[:wanted], exact)
		if err != nil {
			return err
		}
		ended = end
		if n == 0 {
			break
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		blockID := file.blockID(written)
		written++
		if err := s.Write(owner, namespace, blockID, buffer[:n]); err != nil {
			return err
		}
		_, _ = hash.Write(buffer[:n])
		offset += int64(n)
		if ended {
			break
		}
	}
	if !ended {
		if err := stagedEOF(input); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	actualSHA256 := hex.EncodeToString(hash.Sum(nil))
	if file.SHA256 != "" && file.SHA256 != actualSHA256 {
		return ErrStagedFile
	}
	file.SHA256 = actualSHA256
	file.SizeBytes = offset
	return s.Write(owner, namespace, file.ID, file)
}

func readStagedBlock(input io.Reader, buffer []byte, exact bool) (int, bool, error) {
	n, err := io.ReadFull(input, buffer)
	if err == nil {
		return n, false, nil
	}
	if !exact && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
		return n, true, nil
	}
	return n, false, err
}

func stagedEOF(input io.Reader) error {
	var tail [1]byte
	n, err := io.ReadFull(input, tail[:])
	clear(tail[:])
	if n != 0 || !errors.Is(err, io.EOF) {
		return ErrStagedFile
	}
	return nil
}

// OpenStagedFile checks the durable descriptor before serving random reads.
// Every block is authenticated by the existing store's AEAD and scoped path.
func (s *Store) OpenStagedFile(ctx context.Context, owner, namespace string, file StagedFile) (*StagedReader, error) {
	if !s.Encrypted() {
		return nil, ErrEncryptedStore
	}
	if !file.valid() {
		return nil, ErrStagedFile
	}
	var stored StagedFile
	found, err := s.Read(owner, namespace, file.ID, &stored)
	if err != nil {
		return nil, err
	}
	if !found || stored != file {
		return nil, ErrStagedFile
	}
	return &StagedReader{ctx: ctx, store: s, owner: owner, namespace: namespace, file: file, cached: -1}, nil
}

// DeleteStagedFile is idempotent, including after a crash partway through
// cleanup. Applications call it only after the private cloud commit is verified.
func (s *Store) DeleteStagedFile(owner, namespace string, file StagedFile) error {
	if !s.Encrypted() {
		return ErrEncryptedStore
	}
	if !file.valid() {
		return ErrStagedFile
	}
	for n := int64(0); n < file.blocks(); n++ {
		if err := s.discardStagedRecord(owner, namespace, file.blockID(n)); err != nil {
			return err
		}
	}
	if err := s.discardStagedRecord(owner, namespace, file.ID); err != nil {
		return err
	}
	return s.discardStagedRecord(owner, namespace, stagedReservationID(file))
}

// Cleanup does not need to decrypt a corrupt block. The known descriptor and
// authenticated caller fix its hashed path, just as logout can erase ciphertext
// while the OS keychain is locked.
func (s *Store) discardStagedRecord(owner, namespace, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.del(clave(owner, namespace, id))
}

type StagedReader struct {
	mu               sync.Mutex
	ctx              context.Context
	store            *Store
	owner, namespace string
	file             StagedFile
	cached           int64
	buffer           []byte
	closed           bool
}

func (r *StagedReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.buffer)
	r.buffer = nil
	r.closed = true
	return nil
}

func (r *StagedReader) block(n int64) error {
	if r.cached == n {
		return nil
	}
	clear(r.buffer)
	r.buffer = nil
	r.cached = -1
	found, err := r.store.Read(r.owner, r.namespace, r.file.blockID(n), &r.buffer)
	if err != nil {
		return err
	}
	wanted := int(min(int64(stagedBlockBytes), r.file.SizeBytes-n*stagedBlockBytes))
	if !found || len(r.buffer) != wanted {
		clear(r.buffer)
		r.buffer = nil
		return ErrStagedFile
	}
	r.cached = n
	return nil
}

func (r *StagedReader) ReadAt(p []byte, offset int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if offset < 0 {
		return 0, ErrStagedFile
	}
	if len(p) == 0 {
		return 0, nil
	}
	n := 0
	for n < len(p) {
		if err := r.ctx.Err(); err != nil {
			return n, err
		}
		if offset >= r.file.SizeBytes {
			return n, io.EOF
		}
		block := offset / stagedBlockBytes
		if err := r.block(block); err != nil {
			return n, err
		}
		copied := copy(p[n:], r.buffer[offset%stagedBlockBytes:])
		n += copied
		offset += int64(copied)
	}
	return n, nil
}
