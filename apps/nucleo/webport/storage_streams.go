// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type ownedSource struct {
	port     Port
	identity Identity
	reader   io.ReaderAt
	size     int64
	ctx      context.Context
	cancel   context.CancelFunc
	active   sync.WaitGroup
	closed   bool
}

// Sources exposes only random scoped, short-lived ReaderAt capabilities. The
// caller retains its original until all in-flight reads have actually joined.
type Sources struct {
	mu    sync.Mutex
	items map[string]*ownedSource
}

func NewSources() *Sources { return &Sources{items: make(map[string]*ownedSource)} }
func (s *Sources) Add(ctx context.Context, port Port, identity Identity, reader io.ReaderAt, size int64) (string, func(), error) {
	if port == nil || identity.Validate() != nil || reader == nil || size < 0 || size > storage.MaxFileBytes {
		return "", nil, ErrRejected
	}
	id, err := newCapabilityID()
	if err != nil {
		return "", nil, err
	}
	life, cancel := context.WithCancel(ctx)
	source := &ownedSource{port: port, identity: identity, reader: reader, size: size, ctx: life, cancel: cancel}
	s.mu.Lock()
	if len(s.items) >= 128 {
		s.mu.Unlock()
		cancel()
		return "", nil, ErrRejected
	}
	if _, exists := s.items[id]; exists {
		s.mu.Unlock()
		cancel()
		return "", nil, ErrRejected
	}
	s.items[id] = source
	s.mu.Unlock()
	var once sync.Once
	return id, func() {
		once.Do(func() {
			s.mu.Lock()
			source.closed = true
			delete(s.items, id)
			source.cancel()
			s.mu.Unlock()
			source.active.Wait()
		})
	}, nil
}

func (s *Sources) Read(ctx context.Context, port Port, identity Identity, id string, offset int64, length int) ([]byte, error) { //nolint:contextcheck // reads run under the source lifetime, cancelled by the caller ctx via AfterFunc
	if !ValidID(id) || length < 0 || length > ChunkBytes || offset < 0 {
		return nil, ErrRejected
	}
	s.mu.Lock()
	source := s.items[id]
	if !source.canRead(port, identity, offset, length) {
		s.mu.Unlock()
		return nil, ErrRejected
	}
	source.active.Add(1)
	s.mu.Unlock()
	defer source.active.Done()
	active, cancel := context.WithCancel(source.ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := source.ctx.Err(); err != nil {
		return nil, err
	}
	if err := RevalidateIdentity(active, port, identity); err != nil { //nolint:contextcheck // active derives from the source lifetime and is cancelled by the caller ctx via AfterFunc
		return nil, err
	}
	out, err := readSourceChunk(source.reader, offset, length)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = source.ctx.Err()
	}
	if err == nil {
		err = RevalidateIdentity(active, port, identity) //nolint:contextcheck // active derives from the source lifetime and is cancelled by the caller ctx via AfterFunc
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			err = source.ctx.Err()
		}
	}
	if err != nil {
		clear(out)
		return nil, err
	}
	return out, nil
}

func (s *ownedSource) canRead(port Port, identity Identity, offset int64, length int) bool {
	return s != nil && !s.closed && s.port.ID() == port.ID() && s.identity == identity && offset <= s.size && int64(length) <= s.size-offset
}

func readSourceChunk(reader io.ReaderAt, offset int64, length int) ([]byte, error) {
	out := make([]byte, length)
	n, err := reader.ReadAt(out, offset)
	if errors.Is(err, io.EOF) && n == length {
		err = nil
	}
	if err == nil && n != length {
		err = io.ErrUnexpectedEOF
	}
	return out, err
}

func (s *Storage) Download(ctx context.Context, ref storage.Reference, object, revision string, out io.Writer) (_ storage.FileRequest, resultErr error) {
	var opened struct {
		Stream  string              `json:"stream"`
		Request storage.FileRequest `json:"request"`
		SHA256  string              `json:"sha256"`
	}
	if !validFile(ref, object, revision) || out == nil {
		return opened.Request, ErrRejected
	}
	if err := s.call(ctx, StorageFileDownload, fileIdentity{ref, object, revision}, &opened); err != nil {
		return opened.Request, err
	}
	if !ValidID(opened.Stream) {
		return opened.Request, ErrRejected
	}
	defer func() {
		resultErr = errors.Join(resultErr, closeHostReplyHandle(ctx, s.port, s.identity.Scope, StorageFileDownload, opened.Stream))
	}()
	if !ValidID(opened.SHA256) || opened.Request.Validate(s.identity.Scope) != nil || opened.Request.ObjectID != object || opened.Request.RevisionID != revision {
		return opened.Request, ErrRejected
	}

	hash := sha256.New()
	for offset := int64(0); offset < opened.Request.SizeBytes; {
		length := int(min(int64(ChunkBytes), opened.Request.SizeBytes-offset))
		if err := s.downloadChunk(ctx, opened.Stream, offset, length, hash, out); err != nil {
			return opened.Request, err
		}
		offset += int64(length)
	}
	sum := hash.Sum(nil)
	defer clear(sum)
	if hex.EncodeToString(sum) != opened.SHA256 {
		return opened.Request, ErrRejected
	}
	current, err := s.port.Current(ctx)
	if err == nil && current != s.identity {
		err = ErrRejected
	}
	return opened.Request, err
}

func (s *Storage) downloadChunk(ctx context.Context, stream string, offset int64, length int, hash, out io.Writer) error {
	input, err := json.Marshal(struct {
		Stream string `json:"stream"`
		Offset int64  `json:"offset"`
		Length int    `json:"length"`
	}{stream, offset, length})
	if err != nil {
		return err
	}
	data, err := s.port.Call(ctx, s.identity.Scope, StorageFileRead, input)
	clear(input)
	if err != nil {
		clear(data)
		return err
	}
	defer clear(data)
	if len(data) != length {
		return io.ErrUnexpectedEOF
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := hash.Write(data); err != nil {
		return err
	}
	n, err := out.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}
