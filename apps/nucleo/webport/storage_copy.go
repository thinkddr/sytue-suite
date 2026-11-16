// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type randomFile struct {
	store  *Storage
	ctx    context.Context
	stream string
	size   int64
}

func (r *randomFile) ReadAt(out []byte, offset int64) (int, error) {
	if offset < 0 || offset > r.size {
		return 0, ErrRejected
	}
	wanted := int64(len(out))
	if wanted > r.size-offset {
		wanted = r.size - offset
	}
	total := 0
	for int64(total) < wanted {
		length := int(min(int64(ChunkBytes), wanted-int64(total)))
		request, err := json.Marshal(struct {
			Stream string `json:"stream"`
			Offset int64  `json:"offset"`
			Length int    `json:"length"`
		}{r.stream, offset + int64(total), length})
		if err != nil {
			return total, err
		}
		raw, err := r.store.port.Call(r.ctx, r.store.identity.Scope, StorageFileRead, request)
		clear(request)
		if err != nil {
			clear(raw)
			return total, err
		}
		if len(raw) != length {
			clear(raw)
			return total, io.ErrUnexpectedEOF
		}
		copy(out[total:], raw)
		clear(raw)
		total += length
	}
	if total < len(out) {
		return total, io.EOF
	}
	return total, nil
}

// Copy preserves the exact source commitment while changing only explicitly
// supplied destination metadata. Source content never becomes a Blob or server
// object in plaintext; the maintained SDK performs encryption on both sides.
func (s *Storage) Copy(ctx context.Context, source storage.Reference, object, revision, job string, target storage.PublicationTarget, request storage.FileRequest) (storage.Reference, error) {
	return s.CopyTo(ctx, s, source, object, revision, job, target, request)
}

// CopyTo reuses the exact source reader and commitment checks across two real
// SDK identities of the same product. No plaintext server object is created.
func (s *Storage) CopyTo(ctx context.Context, destination *Storage, source storage.Reference, object, revision, job string, target storage.PublicationTarget, request storage.FileRequest) (_ storage.Reference, resultErr error) {
	var ref storage.Reference
	crossIdentity, err := s.canCopyTo(destination, source, object, revision, request)
	if err != nil {
		return ref, err
	}
	var opened struct {
		Stream  string              `json:"stream"`
		Request storage.FileRequest `json:"request"`
		SHA256  string              `json:"sha256"`
	}
	if err := s.call(ctx, StorageFileDownload, fileIdentity{source, object, revision}, &opened); err != nil {
		return ref, err
	}
	if !ValidID(opened.Stream) {
		return ref, ErrRejected
	}
	defer func() {
		resultErr = errors.Join(resultErr, closeHostReplyHandle(ctx, s.port, s.identity.Scope, StorageFileDownload, opened.Stream))
	}()
	if crossIdentity && (len(opened.Request.Previous) != 0 || len(request.Previous) != 0) {
		return ref, ErrRejected
	}
	if !ValidID(opened.SHA256) || opened.Request.Validate(s.identity.Scope) != nil || opened.Request.ObjectID != object || opened.Request.RevisionID != revision || opened.Request.SizeBytes != request.SizeBytes {
		return ref, ErrRejected
	}
	ref, err = destination.Upload(ctx, job, target, request, &randomFile{store: s, ctx: ctx, stream: opened.Stream, size: request.SizeBytes})
	if err != nil {
		return ref, err
	}
	got, checksum, err := destination.Commitment(ctx, ref, request.ObjectID, request.RevisionID)
	expected, _ := json.Marshal(request)
	defer clear(expected)
	actual, _ := json.Marshal(got)
	defer clear(actual)
	if err == nil && (checksum != opened.SHA256 || !bytes.Equal(actual, expected)) {
		err = ErrRejected
	}
	return ref, err
}

func (s *Storage) canCopyTo(destination *Storage, source storage.Reference, object, revision string, request storage.FileRequest) (bool, error) {
	crossIdentity := destination != nil && s.identity.Binding.MatrixUserID != destination.identity.Binding.MatrixUserID
	if crossIdentity && (s.StorageIdentityVersion() != storage.LegacyIdentity || destination.StorageIdentityVersion() != storage.IsolatedIdentity) {
		return false, ErrRejected
	}
	if destination == nil || destination.identity.Scope != s.identity.Scope || !validFile(source, object, revision) || request.Validate(s.identity.Scope) != nil {
		return false, ErrRejected
	}
	return crossIdentity, nil
}
