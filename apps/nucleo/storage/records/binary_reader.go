// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type binaryRangeReader interface {
	ReadStorageFileRange(context.Context, *api.Drive, storage.Reference, string, string, int64, int64) ([]byte, error)
}

// BinaryReader seeks in one authenticated immutable revision. It retains at most
// one MiB of plaintext on the authorized device; Close and session cancellation
// erase that buffer and join any active SDK read before releasing the lease.
// Read/Seek may share a caller with Close, but do not run concurrently themselves.
type BinaryReader struct {
	mu             sync.Mutex
	once           sync.Once
	ctx            context.Context //nolint:containedctx // bounded by the reader and its repository's work gate
	cancel         context.CancelFunc
	stop           func() bool
	release        func()
	sdk            binaryRangeReader
	gateway        *api.Drive
	version        storage.ObjectVersion
	size, position int64
	buffer         []byte
	closed         bool
}

// OpenBinary never uses an HTTP source. Seeking is available only through the
// maintained SDK's authenticated range capability, not a plaintext spool file.
func (r *Repository) OpenBinary(parent context.Context, kind, id string) (*BinaryReader, error) {
	ctx, release, err := r.Start(parent)
	if err != nil {
		return nil, err
	}
	opened := false
	defer func() {
		if !opened {
			release()
		}
	}()
	sdk, ok := r.cfg.Crypto.(binaryRangeReader)
	if !ok {
		return nil, ErrSource
	}
	version, err := r.ReadyVersion(kind, id)
	if err != nil {
		return nil, err
	}
	row, err := r.verifyBinaryCommitment(ctx, version)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	life, cancel := context.WithCancel(ctx)
	reader := &BinaryReader{ctx: life, cancel: cancel, release: release, sdk: sdk, gateway: r.cfg.Gateway, version: version, size: row.Size}
	// The callback can run immediately, so publish stop under the same mutex used
	// by Close. No idle decoder can keep logout waiting for another Read call.
	reader.mu.Lock()
	reader.stop = context.AfterFunc(life, func() { _ = reader.Close() })
	reader.mu.Unlock()
	opened = true
	return reader, nil
}

func (r *BinaryReader) Size() int64 { return r.size }

func (r *BinaryReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrClosed
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.position >= r.size {
		return 0, io.EOF
	}
	if len(r.buffer) == 0 {
		length := min(int64(1<<20), r.size-r.position)
		pointer := r.version.Commit.Content
		body, err := r.sdk.ReadStorageFileRange(r.ctx, r.gateway, pointer.Reference, r.version.ObjectID, pointer.RevisionID, r.position, length)
		if err != nil {
			clear(body)
			return 0, err
		}
		if int64(len(body)) != length {
			clear(body)
			return 0, ErrSource
		}
		if err := r.ctx.Err(); err != nil {
			clear(body)
			return 0, err
		}
		r.buffer = body
	}
	n := copy(p, r.buffer)
	clear(r.buffer[:n])
	r.buffer = r.buffer[n:]
	r.position += int64(n)
	return n, nil
}

func (r *BinaryReader) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrClosed
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = r.position
	case io.SeekEnd:
		base = r.size
	default:
		return 0, errors.New("records: invalid seek origin")
	}
	if (offset > 0 && base > math.MaxInt64-offset) || (offset < 0 && base < math.MinInt64-offset) {
		return 0, errors.New("records: seek overflow")
	}
	position := base + offset
	if position < 0 {
		return 0, errors.New("records: negative seek")
	}
	if position != r.position {
		clear(r.buffer)
		r.buffer = nil
		r.position = position
	}
	return r.position, nil
}

func (r *BinaryReader) Close() error {
	r.once.Do(func() {
		r.cancel()
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.stop != nil {
			r.stop()
		}
		r.closed = true
		clear(r.buffer)
		r.buffer = nil
		clear(r.version.Commit.Metadata)
		r.version = storage.ObjectVersion{}
		r.release()
	})
	return nil
}
