// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"slices"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type pendingFileBlock struct {
	Block       StorageBlock `json:"block"`
	PlainSHA256 string       `json:"plain_sha256"`
}

type storageFileUpload struct {
	Target       storage.PublicationTarget     `json:"target"`
	Version      int                           `json:"v"`
	Scope        storage.Scope                 `json:"scope"`
	Request      StorageFileRequest            `json:"request"`
	Salt         string                        `json:"salt"`
	SourceSHA256 string                        `json:"source_sha256"`
	PrefixSHA256 string                        `json:"prefix_sha256"`
	Next         int                           `json:"next"`
	Frontier     [fileLevels][]storageFileLink `json:"frontier"`
	Pages        [fileLevels]int               `json:"pages"`
	Pending      *pendingFileBlock             `json:"pending,omitempty"`
	File         *storageFile                  `json:"file,omitempty"`
	Published    *EventRef                     `json:"published,omitempty"`
	Verified     bool                          `json:"verified,omitempty"`
}

func (s storageFileUpload) transaction(kind string, level, index int) string {
	return "file_" + digestBytes([]byte(s.Salt+"\x00"+kind+"\x00"+strconv.Itoa(level)+"\x00"+strconv.Itoa(index)))
}

func (f storageFileIO) journalOwner() string {
	return f.scope.Tenant + "\x00" + f.scope.Subject + "\x00" + f.scope.Product
}

func (f storageFileIO) checkpoint(ctx context.Context, job string, state *storageFileUpload) error {
	if err := f.guard(ctx); err != nil {
		return err
	}
	return f.journal.Write(f.journalOwner(), storage.PendingUploadsNamespace, job, state)
}

func (s storageFileUpload) validate(scope storage.Scope, request StorageFileRequest) error {
	if s.Version == 1 {
		return ErrStoragePublicationMissing
	}
	if s.Version != 2 || !s.Target.Valid() || s.Scope != scope || s.Request.Validate(scope) != nil || s.Next < 0 || s.Next > fileBlocks(request.SizeBytes) || !storage.Identifier(s.Salt) || !validFileHash(s.SourceSHA256) || !validFileHash(s.PrefixSHA256) {
		return storage.ErrRecord
	}
	original, _ := json.Marshal(s.Request)
	now, _ := json.Marshal(request)
	if !bytes.Equal(original, now) {
		return ErrStorageSourceChanged
	}
	for level := range fileLevels {
		if len(s.Frontier[level]) > fileFanout || s.Pages[level] < 0 || s.Pages[level] > maxFileBlocks {
			return storage.ErrRecord
		}
	}
	if err := s.validatePending(scope, request); err != nil {
		return err
	}
	if err := s.validatePrepared(); err != nil {
		return err
	}
	return nil
}

func (s storageFileUpload) validatePending(scope storage.Scope, request StorageFileRequest) error {
	if s.Pending != nil {
		b := s.Pending.Block
		if s.Next >= fileBlocks(request.SizeBytes) || !validFileHash(s.Pending.PlainSHA256) || b.Blob.SizeBytes != fileBlockBytes(request.SizeBytes, s.Next) || b.Validate(scope, request.ObjectID, request.RevisionID, s.Next) != nil {
			return storage.ErrRecord
		}
	}
	return nil
}

func (s storageFileUpload) validatePrepared() error {
	request := s.Request
	if s.Verified && s.Published == nil {
		return storage.ErrRecord
	}
	if s.File != nil && (s.Next != fileBlocks(request.SizeBytes) || s.Pending != nil) {
		return storage.ErrRecord
	}
	if s.Published != nil && (s.File == nil || !validFileEvent(*s.Published) || s.Published.RoomID != s.Target.RoomID) {
		return storage.ErrRecord
	}
	if s.File != nil {
		original, _ := json.Marshal(request)
		prepared, _ := json.Marshal(s.File.Request)
		if !bytes.Equal(original, prepared) || s.File.Kind != fileKind || s.File.SHA256 != s.SourceSHA256 {
			return storage.ErrRecord
		}
		return validateFilePage(s.File.Root, request, fileRootLevel(fileBlocks(request.SizeBytes)), 0)
	}
	return nil
}

func (f storageFileIO) hashSource(ctx context.Context, source io.ReaderAt, size int64, h hash.Hash) error {
	buffer := make([]byte, 64<<10)
	defer clear(buffer)
	for offset := int64(0); offset < size; {
		if err := f.guard(ctx); err != nil {
			return err
		}
		wanted := int(min(int64(len(buffer)), size-offset))
		n, err := source.ReadAt(buffer[:wanted], offset)
		if n != wanted {
			return ErrStorageSourceChanged
		}
		if err != nil && err != io.EOF {
			return err
		}
		_, _ = h.Write(buffer[:n])
		offset += int64(n)
	}
	return nil
}

func (f storageFileIO) exactSource(ctx context.Context, source io.ReaderAt, size int64) error {
	if err := f.guard(ctx); err != nil {
		return err
	}
	var tail [1]byte
	n, err := source.ReadAt(tail[:], size)
	if n != 0 {
		return ErrStorageSourceChanged
	}
	if err != io.EOF {
		return storage.ErrRecord
	}
	return nil
}

func (f storageFileIO) uploadState(ctx context.Context, job string, request StorageFileRequest, source io.ReaderAt, publication storagePublicationIO) (storageFileUpload, error) {
	var state storageFileUpload
	found, err := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, job, &state)
	if err != nil {
		return state, err
	}
	if found {
		if f.expectedSourceSHA256 != "" && state.SourceSHA256 != f.expectedSourceSHA256 {
			return state, ErrStorageSourceChanged
		}
		return state, state.validate(f.scope, request)
	}
	target, targetErr := publication.StoragePublicationTarget()
	if targetErr != nil {
		return state, targetErr
	}
	h := sha256.New()
	if err := f.hashSource(ctx, source, request.SizeBytes, h); err != nil {
		return state, err
	}
	if err := f.exactSource(ctx, source, request.SizeBytes); err != nil {
		return state, err
	}
	if f.expectedSourceSHA256 != "" && hex.EncodeToString(h.Sum(nil)) != f.expectedSourceSHA256 {
		return state, ErrStorageSourceChanged
	}
	state = storageFileUpload{Version: 2, Target: target, Scope: f.scope, Request: request, Salt: rand.Text(), SourceSHA256: hex.EncodeToString(h.Sum(nil)), PrefixSHA256: digestBytes(nil)}
	err = f.checkpoint(ctx, job, &state)
	return state, err
}

func (f storageFileIO) flush(ctx context.Context, job string, state *storageFileUpload, level int) error {
	if level >= fileLevels-1 || len(state.Frontier[level]) == 0 {
		return storage.ErrRecord
	}
	// Make space first, retaining the current page until its parent can accept
	// the receipt. Every checkpoint remains a complete recoverable frontier.
	if len(state.Frontier[level+1]) == fileFanout {
		if err := f.flush(ctx, job, state, level+1); err != nil {
			return err
		}
	}
	page := storageFilePage{Kind: filePageKind, Level: level, Index: state.Pages[level], Nodes: state.Frontier[level]}
	payload, err := json.Marshal(page)
	if err != nil {
		return err
	}
	ref, err := f.saveRecord(ctx, state.transaction("page", level, page.Index), state.Request, payload, nil)
	if err != nil {
		return err
	}
	state.Pages[level]++
	state.Frontier[level] = nil
	state.Frontier[level+1] = append(state.Frontier[level+1], storageFileLink{Event: &ref})
	return f.checkpoint(ctx, job, state)
}

func restoreFileBlock(pending pendingFileBlock, source io.Reader, size int64) ([]byte, error) {
	data, err := storageBytes(source, size)
	if err != nil {
		return nil, err
	}
	// Reusing an AES-CTR key/IV is permissible only for the exact same plaintext
	// retry. Never encrypt changed input with a descriptor retained in a journal.
	if digestBytes(data) != pending.PlainSHA256 {
		clear(data)
		return nil, ErrStorageSourceChanged
	}
	file := pending.Block.File
	if err := file.PrepareForDecryption(); err != nil {
		clear(data)
		return nil, err
	}
	file.EncryptInPlace(data)
	if digestBytes(data) != pending.Block.Blob.SHA256 {
		clear(data)
		return nil, ErrIdentity
	}
	return data, nil
}

func (f storageFileIO) uploadLeaf(ctx context.Context, job string, state *storageFileUpload, source io.ReaderAt, h hash.Hash) error {
	if len(state.Frontier[0]) == fileFanout {
		if err := f.flush(ctx, job, state, 0); err != nil {
			return err
		}
	}
	size := fileBlockBytes(state.Request.SizeBytes, state.Next)
	input := io.TeeReader(io.NewSectionReader(source, int64(state.Next)*storage.MaxCipherBlockBytes, size), h)
	var data []byte
	var err error
	if state.Pending == nil {
		// A separate small hash binds the pending ciphertext to the source chunk
		// across interruptions; only digests (never hash buffers) enter the journal.
		plainHash := sha256.New()
		block, ciphertext, prepareErr := prepareStorageBlock(f.scope, state.Request.ObjectID, state.Request.RevisionID, state.Next, io.TeeReader(input, plainHash), size)
		if prepareErr != nil {
			return prepareErr
		}
		data = ciphertext
		state.Pending = &pendingFileBlock{Block: block, PlainSHA256: hex.EncodeToString(plainHash.Sum(nil))}
		err = f.checkpoint(ctx, job, state)
	} else {
		data, err = restoreFileBlock(*state.Pending, input, size)
	}
	defer clear(data)
	if err != nil {
		return err
	}
	if err := f.guard(ctx); err != nil {
		return err
	}
	if err := putPreparedStorageBlock(ctx, f.gateway, state.Pending.Block, data); err != nil {
		return err
	}
	block := state.Pending.Block
	state.Frontier[0] = append(state.Frontier[0], storageFileLink{Block: fileDescriptor(block, state.Pending.PlainSHA256)})
	state.Pending = nil
	state.Next++
	state.PrefixSHA256 = hex.EncodeToString(h.Sum(nil))
	return f.checkpoint(ctx, job, state)
}

func (f storageFileIO) prepareFile(ctx context.Context, job string, state *storageFileUpload, source io.ReaderAt) error {
	if state.File != nil {
		h := sha256.New()
		if err := f.hashSource(ctx, source, state.Request.SizeBytes, h); err != nil {
			return err
		}
		if hex.EncodeToString(h.Sum(nil)) != state.SourceSHA256 {
			return ErrStorageSourceChanged
		}
		return f.exactSource(ctx, source, state.Request.SizeBytes)
	}
	h := sha256.New()
	offset := min(int64(state.Next)*storage.MaxCipherBlockBytes, state.Request.SizeBytes)
	if err := f.hashSource(ctx, source, offset, h); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != state.PrefixSHA256 {
		return ErrStorageSourceChanged
	}
	for state.Next < fileBlocks(state.Request.SizeBytes) {
		if err := f.guard(ctx); err != nil {
			return err
		}
		if err := f.uploadLeaf(ctx, job, state, source, h); err != nil {
			return err
		}
	}
	if err := f.exactSource(ctx, source, state.Request.SizeBytes); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != state.SourceSHA256 {
		return ErrStorageSourceChanged
	}
	rootLevel := fileRootLevel(fileBlocks(state.Request.SizeBytes))
	for level := 0; level < rootLevel; level++ {
		if len(state.Frontier[level]) != 0 {
			if err := f.flush(ctx, job, state, level); err != nil {
				return err
			}
		}
	}
	root := storageFilePage{Kind: filePageKind, Level: rootLevel, Index: 0, Nodes: state.Frontier[rootLevel]}
	if err := validateFilePage(root, state.Request, rootLevel, 0); err != nil {
		return err
	}
	state.File = &storageFile{Kind: fileKind, Request: state.Request, SHA256: state.SourceSHA256, Root: root}
	return f.checkpoint(ctx, job, state)
}

func (f storageFileIO) upload(ctx context.Context, job string, request StorageFileRequest, source io.ReaderAt) (EventRef, error) {
	if f.gateway == nil || f.records == nil || !f.journal.Encrypted() || source == nil || !storage.Identifier(job) || request.Validate(f.scope) != nil || f.expectedSourceSHA256 != "" && !validFileHash(f.expectedSourceSHA256) {
		return EventRef{}, storage.ErrRecord
	}
	request.Metadata = bytes.Clone(request.Metadata)
	request.Previous = slices.Clone(request.Previous)
	publication, ok := f.records.(storagePublicationIO)
	if !ok {
		return EventRef{}, storage.ErrRecord
	}
	var result EventRef
	err := f.journal.Exclusive(ctx, f.journalOwner(), storage.PendingUploadsNamespace, job, func() error {
		state, err := f.uploadState(ctx, job, request, source, publication)
		if err != nil {
			return err
		}
		if err := publication.CheckStoragePublicationTarget(state.Target); err != nil {
			return err
		}
		f.records = pinnedStorageRecords{storageRecordIO: f.records, publication: publication, target: state.Target}
		f.publicationCheck = func() error { return publication.CheckStoragePublicationTarget(state.Target) }
		if err := f.prepareFile(ctx, job, &state, source); err != nil {
			return err
		}
		if state.Published == nil {
			payload, marshalErr := json.Marshal(state.File)
			if marshalErr != nil {
				return marshalErr
			}
			ref, writeErr := f.saveRecord(ctx, state.transaction("root", 0, 0), request, payload, request.Previous)
			if writeErr != nil {
				return writeErr
			}
			state.Published = &ref
			if err := f.checkpoint(ctx, job, &state); err != nil {
				return err
			}
		}
		result = *state.Published
		// A verified file is not yet an application commit. Retain its receipt so
		// a crash before catalog publication cannot create a second file on retry.
		state.Verified = false
		if err := f.checkpoint(ctx, job, &state); err != nil {
			return err
		}
		if _, err = f.download(ctx, result, request.ObjectID, request.RevisionID, io.Discard); err != nil {
			return err
		}
		state.Verified = true
		if err := f.checkpoint(ctx, job, &state); err != nil {
			return err
		}
		return f.guard(ctx)
	})
	return result, err
}

// UploadStorageFile resumes through the existing encrypted offline store. It
// writes each descriptor before network I/O, retains pending work on every error,
// and verifies the complete encrypted replacement. Originals are never removed.
// The completed journal remains until AcknowledgeStorageFile verifies the app's
// immutable catalog commit points to this exact file and content revision.
// Use an immutable ReaderAt revision; changed inputs fail rather than silently
// reusing keys. Logout must cancel/wait for its device jobs before clearing cache.
func (c *Client) UploadStorageFile(ctx context.Context, gateway *api.Drive, journal *offline.Store, job string, request StorageFileRequest, source io.ReaderAt) (EventRef, error) {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return EventRef{}, err
	}
	return (storageFileIO{scope: scope, gateway: gateway, records: c, journal: journal, closed: c.closed.Load}).upload(ctx, job, request, source)
}

// storageFileTarget overrides only publication routing; the maintained uploader
// still owns its encrypted journal, chunk keys, verification and acknowledgment.
type storageFileTarget struct {
	storageRecordIO
	storagePublicationIO
	target storage.PublicationTarget
}

func (s storageFileTarget) StoragePublicationTarget() (storage.PublicationTarget, error) {
	return s.target, s.CheckStoragePublicationTarget(s.target)
}

func (s storageFileTarget) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if target != s.target {
		return ErrIdentity
	}
	return s.storagePublicationIO.CheckStoragePublicationTarget(target)
}

// UploadStorageFileAt pins even the first publication to an application's
// durable intent. A room rotation cannot redirect its initial manifest, and an
// existing job at another endpoint is rejected before reading source bytes or
// calling the blob provider. UploadStorageFile retains its historical behavior.
func (c *Client) UploadStorageFileAt(ctx context.Context, gateway *api.Drive, journal *offline.Store, job string, target storage.PublicationTarget, request StorageFileRequest, source io.ReaderAt) (EventRef, error) {
	if err := c.CheckStoragePublicationTarget(target); err != nil {
		return EventRef{}, err
	}
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return EventRef{}, err
	}
	records := storageFileTarget{storageRecordIO: c, storagePublicationIO: c, target: target}
	return (storageFileIO{scope: scope, gateway: gateway, records: records, journal: journal, closed: c.closed.Load}).upload(ctx, job, request, source)
}
