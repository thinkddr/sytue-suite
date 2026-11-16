// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Source is the bounded retained-source adapter shared by browser product
// engines. The immutable port owns credentials and the only network routes.
type Source struct {
	owner     *Storage
	provision bool
}

func NewSource(owner *Storage, provision bool) (*Source, error) {
	if owner == nil || !owner.identity.Scope.Valid() || provision && owner.identity.Scope.Product != "mail" {
		return nil, ErrRejected
	}
	return &Source{owner: owner, provision: provision}, nil
}

func (s *Source) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	var state storage.AppMigrationState
	if scope != s.owner.identity.Scope {
		return state, ErrRejected
	}
	err := s.owner.call(ctx, SourceCapabilities, nil, &state)
	if err == nil {
		err = state.Validate(scope)
	}
	return state, err
}

func (s *Source) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.AppMigrationState, error) {
	var state storage.AppMigrationState
	if scope != s.owner.identity.Scope || !storage.MigrationEpoch(epoch) {
		return state, ErrRejected
	}
	err := s.owner.call(ctx, SourceBegin, struct {
		Epoch     string `json:"epoch"`
		Provision bool   `json:"provision"`
	}{epoch, s.provision}, &state)
	if err == nil {
		err = state.Validate(scope)
	}
	if err == nil && (state.Mode != "frozen" || state.Epoch != epoch) {
		err = ErrRejected
	}
	return state, err
}

func (s *Source) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	var page storage.StreamSourcePage
	if scope != s.owner.identity.Scope || !storage.MigrationEpoch(epoch) {
		return page, ErrRejected
	}
	encoded, err := EncodeSourcePageRequest(scope, epoch, after)
	if err != nil {
		return page, err
	}
	defer clear(encoded)
	err = s.owner.call(ctx, SourcePage, json.RawMessage(encoded), &page)
	if err == nil {
		err = page.Validate(scope, epoch, after)
	}
	return page, err
}

func (s *Source) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	if scope != s.owner.identity.Scope || !storage.MigrationEpoch(epoch) || row.Validate() != nil {
		return nil, ErrRejected
	}
	var opened struct {
		Stream string                  `json:"stream"`
		Row    storage.StreamSourceRow `json:"row"`
	}
	if err := s.owner.call(ctx, SourceOpen, struct {
		Epoch string                  `json:"epoch"`
		Row   storage.StreamSourceRow `json:"row"`
	}{epoch, row}, &opened); err != nil {
		return nil, err
	}
	if !ValidID(opened.Stream) {
		return nil, ErrRejected
	}
	if opened.Row != row {
		return nil, errors.Join(ErrRejected, closeHostReplyHandle(ctx, s.owner.port, s.owner.identity.Scope, SourceOpen, opened.Stream))
	}
	life, cancel := context.WithCancel(ctx)
	reader := &sourceReader{owner: s.owner, ctx: life, cancel: cancel, stream: opened.Stream, row: row, hash: sha256.New()}
	reader.stop = context.AfterFunc(ctx, func() { _ = reader.Close() })
	return reader, nil
}

type sourceReader struct {
	owner            *Storage
	ctx              context.Context
	cancel           context.CancelFunc
	stop             func() bool
	stream           string
	row              storage.StreamSourceRow
	hash             hash.Hash
	mu               sync.Mutex
	offset           int64
	closed, verified bool
	closeErr         error
}

func (r *sourceReader) Read(out []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if err := r.ctx.Err(); err != nil {
		return 0, errors.Join(err, r.closeLocked())
	}
	if len(out) == 0 {
		return 0, nil
	}
	if r.verified {
		return 0, io.EOF
	}
	length := int(min(int64(len(out)), int64(ChunkBytes), r.row.Size-r.offset))
	raw, err := r.owner.callOwned(r.ctx, SourceRead, struct {
		Stream string `json:"stream"`
		Offset int64  `json:"offset"`
		Length int    `json:"length"`
	}{r.stream, r.offset, length})
	defer clear(raw)
	if err != nil {
		return 0, errors.Join(err, r.closeLocked())
	}
	if len(raw) != length {
		return 0, errors.Join(io.ErrUnexpectedEOF, r.closeLocked())
	}
	if length == 0 {
		if hex.EncodeToString(r.hash.Sum(nil)) != r.row.SHA256 {
			return 0, errors.Join(ErrRejected, r.closeLocked())
		}
		r.verified = true
		return 0, io.EOF
	}
	if _, err = r.hash.Write(raw); err != nil {
		return 0, errors.Join(err, r.closeLocked())
	}
	copy(out, raw)
	r.offset += int64(length)
	return length, nil
}

func (r *sourceReader) Close() error {
	r.cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeLocked()
}

func (r *sourceReader) closeLocked() error {
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	r.cancel()
	if r.stop != nil {
		r.stop()
	}
	r.closeErr = closeHostReplyHandle(r.ctx, r.owner.port, r.owner.identity.Scope, SourceOpen, r.stream)
	return r.closeErr
}

// EncodeSourcePageRequest preserves the native source cursor codec in Go. The
// host receives an already validated opaque query, not a second TS parser.
func EncodeSourcePageRequest(scope storage.Scope, epoch string, after storage.LegacySourceCursor) ([]byte, error) {
	if !scope.Valid() || !storage.MigrationEpoch(epoch) {
		return nil, ErrRejected
	}
	cursor, err := storage.EncodeSourceCursor(after)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Epoch string `json:"epoch"`
		After string `json:"after"`
	}{epoch, cursor})
}

// WriteTo keeps actual cross-worker reads at the bounded source block size.
// Hide both optional io fast paths to retain this buffer without recursion.
func (r *sourceReader) WriteTo(out io.Writer) (int64, error) {
	if out == nil {
		return 0, ErrRejected
	}
	buffer := make([]byte, ChunkBytes)
	defer clear(buffer)
	return io.CopyBuffer(struct{ io.Writer }{out}, struct{ io.Reader }{r}, buffer)
}
