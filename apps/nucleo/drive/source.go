// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// sourceReader adapts a frozen HTTP source to the maintained SDK's ReaderAt.
// Each sequential pass uses one stream. Rewinding opens a fresh stream instead
// of staging the entire original on a phone or persisting plaintext to disk.
type sourceReader struct {
	source          Source
	scope           storage.Scope
	epoch, kind, id string
	size            int64
	checksum        string
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.Mutex
	body            io.ReadCloser
	offset          int64
	sum             hash.Hash
	verified        bool
	closed          bool
}

func openSource(ctx context.Context, source Source, scope storage.Scope, epoch, kind, id string, size int64, checksum string) (*sourceReader, error) {
	if source == nil || !scope.Valid() || scope.Product != "drive" || !storage.MigrationEpoch(epoch) || (kind != "file" && kind != "version") || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) || size < 0 || size > storage.MaxFileBytes {
		return nil, ErrSource
	}
	if checksum != "" {
		decoded, err := hex.DecodeString(checksum)
		if err != nil || len(decoded) != sha256.Size {
			return nil, ErrSource
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	return &sourceReader{source: source, scope: scope, epoch: epoch, kind: kind, id: id, size: size, checksum: strings.ToLower(checksum), ctx: ctx, cancel: cancel}, nil
}

func (r *sourceReader) rewind() error {
	if r.body != nil {
		_ = r.body.Close()
		r.body = nil
	}
	body, err := r.source.OpenPrivateStorageSource(r.ctx, r.scope, r.epoch, r.kind, r.id, r.size)
	if err != nil {
		return err
	}
	r.body = body
	r.offset = 0
	r.sum = sha256.New()
	r.verified = false
	return nil
}

func (r *sourceReader) read(p []byte) (int, error) {
	n, err := io.ReadFull(r.body, p)
	if n > 0 {
		_, _ = r.sum.Write(p[:n])
		r.offset += int64(n)
	}
	return n, err
}

func (r *sourceReader) finish() error {
	if r.verified {
		return nil
	}
	var probe [1]byte
	n, err := io.ReadFull(r.body, probe[:])
	clear(probe[:])
	if n != 0 || !errors.Is(err, io.EOF) {
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return err
		}
		return ErrSource
	}
	if r.checksum != "" && hex.EncodeToString(r.sum.Sum(nil)) != r.checksum {
		return ErrSource
	}
	r.verified = true
	return nil
}

func (r *sourceReader) ReadAt(out []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if offset < 0 {
		return 0, ErrSource
	}
	if len(out) == 0 {
		return 0, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrClosed
	}
	if r.body == nil || offset < r.offset {
		if err := r.rewind(); err != nil {
			return 0, err
		}
	}
	wantedOffset := min(offset, r.size)
	if wantedOffset > r.offset {
		buffer := make([]byte, 64<<10)
		defer clear(buffer)
		for r.offset < wantedOffset {
			if err := r.ctx.Err(); err != nil {
				return 0, err
			}
			if _, err := r.read(buffer[:min(int64(len(buffer)), wantedOffset-r.offset)]); err != nil {
				return 0, err
			}
		}
	}
	if offset >= r.size {
		if err := r.finish(); err != nil {
			return 0, err
		}
		return 0, io.EOF
	}
	wanted := min(int64(len(out)), r.size-offset)
	n, err := r.read(out[:wanted])
	if err != nil {
		return n, err
	}
	if r.offset == r.size {
		if err := r.finish(); err != nil {
			clear(out[:n])
			return 0, err
		}
	}
	if n < len(out) {
		return n, io.EOF
	}
	return n, nil
}

func (r *sourceReader) Close() error {
	r.cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.body != nil {
		err := r.body.Close()
		r.body = nil
		return err
	}
	return nil
}

func (r *sourceReader) digest() (string, error) {
	if r.size == 0 {
		var probe [1]byte
		if _, err := r.ReadAt(probe[:], 0); !errors.Is(err, io.EOF) {
			return "", err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.verified || r.sum == nil {
		return "", ErrSource
	}
	return hex.EncodeToString(r.sum.Sum(nil)), nil
}

type bodySink struct {
	out       io.Writer
	remaining int64
	sum       hash.Hash
}

func newBodySink(out io.Writer, size int64) *bodySink {
	return &bodySink{out: out, remaining: size, sum: sha256.New()}
}

func (s *bodySink) Write(p []byte) (int, error) {
	if int64(len(p)) > s.remaining {
		return 0, ErrSource
	}
	n, err := s.out.Write(p)
	if n > 0 {
		_, _ = s.sum.Write(p[:n])
		s.remaining -= int64(n)
	}
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

func (s *bodySink) finish(want string) error {
	if s.remaining != 0 || hex.EncodeToString(s.sum.Sum(nil)) != want {
		return ErrSource
	}
	return nil
}
