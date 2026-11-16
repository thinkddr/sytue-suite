// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"encoding/hex"
	"hash"
	"io"
	"sync"
)

// FinalizeSource verifies the forward source's actual EOF through the SAME
// scoped read guard before exposing its Go-computed commitment. It never keeps
// or rewinds MIME bytes. Other ReaderAt implementations cannot impersonate it.
func (s *Sources) FinalizeSource(ctx context.Context, port Port, identity Identity, id string) (string, error) {
	if !ValidID(id) || port == nil {
		return "", ErrRejected
	}
	s.mu.Lock()
	source := s.items[id]
	if source == nil || source.closed || source.port.ID() != port.ID() || source.identity != identity {
		s.mu.Unlock()
		return "", ErrRejected
	}
	forward, ok := source.reader.(*forwardSource)
	size := source.size
	s.mu.Unlock()
	if !ok {
		return "", ErrRejected
	}
	empty, err := s.Read(ctx, port, identity, id, size, 0)
	clear(empty)
	if err != nil {
		return "", err
	}
	forward.mu.Lock()
	defer forward.mu.Unlock()
	if !forward.finalized || !ValidID(forward.sha256) {
		return "", ErrRejected
	}
	return forward.sha256, ctx.Err()
}

type forwardSource struct {
	mu           sync.Mutex
	reader       io.Reader
	offset, size int64
	hash         hash.Hash
	finalized    bool
	sha256       string
}

func (r *forwardSource) ReadAt(out []byte, offset int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if offset != r.offset || r.finalized {
		return 0, ErrRejected
	}
	if len(out) == 0 {
		if r.offset != r.size || r.hash == nil {
			return 0, ErrRejected
		}
		var extra [1]byte
		n, err := r.reader.Read(extra[:])
		clear(extra[:])
		if n != 0 || err != io.EOF {
			return 0, ErrRejected
		}
		r.finalized = true
		r.sha256 = hex.EncodeToString(r.hash.Sum(nil))
		return 0, nil
	}
	n, err := io.ReadFull(r.reader, out)
	r.offset += int64(n)
	if r.hash != nil {
		_, _ = r.hash.Write(out[:n])
	}
	return n, err
}
