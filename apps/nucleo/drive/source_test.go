// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var readerScope = storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}

const readerEpoch = "12345678-1234-1234-1234-123456789abc"

type streamSource struct {
	Source
	open  func(context.Context) (io.ReadCloser, error)
	calls int
}

func (s *streamSource) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch, kind, id string, _ int64) (io.ReadCloser, error) {
	s.calls++
	if scope != readerScope || epoch != readerEpoch || kind != "file" || id != "source" {
		return nil, ErrSource
	}
	return s.open(ctx)
}

func TestFrozenSourceReaderUsesSequentialStreamsWithoutAPlaintextSpool(t *testing.T) {
	original := bytes.Repeat([]byte("private immutable source;"), 5000)
	sum := sha256.Sum256(original)
	source := &streamSource{open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(original)), nil }}
	reader, err := openSource(t.Context(), source, readerScope, readerEpoch, "file", "source", int64(len(original)), hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	for pass := range 2 {
		offset := int64(0)
		for offset < int64(len(original)) {
			buffer := make([]byte, min(65536, len(original)-int(offset)))
			n, err := reader.ReadAt(buffer, offset)
			if err != nil || n != len(buffer) || !bytes.Equal(buffer, original[offset:offset+int64(n)]) {
				t.Fatal("streamed revision changed", pass, offset, err)
			}
			offset += int64(n)
		}
		var tail [1]byte
		if n, err := reader.ReadAt(tail[:], offset); n != 0 || !errors.Is(err, io.EOF) {
			t.Fatal("source tail was accepted", n, err)
		}
		got, err := reader.digest()
		if err != nil || got != hex.EncodeToString(sum[:]) {
			t.Fatal("verified source digest changed", err)
		}
	}
	if source.calls != 2 {
		t.Fatal("ReaderAt restarted HTTP on every small read", source.calls)
	}
}

func TestFrozenSourceReaderRejectsTruncationOverrunAndChangedChecksum(t *testing.T) {
	expected := sha256.Sum256([]byte("expected"))
	for _, test := range []struct {
		name, body, checksum string
		size                 int64
	}{{"truncated", "ab", "", 3}, {"overrun", "abcd", "", 3}, {"checksum", "changed!", hex.EncodeToString(expected[:]), 8}} {
		t.Run(test.name, func(t *testing.T) {
			source := &streamSource{open: func(context.Context) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader([]byte(test.body))), nil
			}}
			reader, err := openSource(t.Context(), source, readerScope, readerEpoch, "file", "source", test.size, test.checksum)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = reader.Close() }()
			buffer := make([]byte, test.size)
			if _, err := reader.ReadAt(buffer, 0); err == nil {
				t.Fatal("changed source was accepted")
			}
			if _, err := reader.digest(); err == nil {
				t.Fatal("incomplete source gained a verified digest")
			}
		})
	}
}

func TestFrozenEmptySourceVerifiesActualEOFBeforeReturningItsDigest(t *testing.T) {
	for _, body := range []string{"", "unexpected"} {
		t.Run(body, func(t *testing.T) {
			source := &streamSource{open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewBufferString(body)), nil }}
			reader, err := openSource(t.Context(), source, readerScope, readerEpoch, "file", "source", 0, "")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = reader.Close() }()
			sum, err := reader.digest()
			if body == "" {
				if err != nil || sum != digest(nil) {
					t.Fatal("empty original failed verification", err)
				}
			} else if err == nil {
				t.Fatal("nonempty source verified as empty")
			}
			if source.calls != 1 {
				t.Fatal("empty original did not verify its HTTP stream", source.calls)
			}
		})
	}
}

type hugeStream struct {
	remaining int64
	read      int64
}

func (s *hugeStream) Read(p []byte) (int, error) {
	if s.remaining == 0 {
		return 0, io.EOF
	}
	n := int(min(int64(len(p)), s.remaining))
	clear(p[:n])
	s.remaining -= int64(n)
	s.read += int64(n)
	return n, nil
}
func (*hugeStream) Close() error { return nil }

func TestFrozenSourceReaderDoesNotDownloadOrAllocateTheWholeLargeOriginal(t *testing.T) {
	body := &hugeStream{remaining: 1 << 40}
	source := &streamSource{open: func(context.Context) (io.ReadCloser, error) { return body, nil }}
	reader, err := openSource(t.Context(), source, readerScope, readerEpoch, "file", "source", body.remaining, "")
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 1<<20)
	if n, err := reader.ReadAt(buffer, 0); err != nil || n != len(buffer) {
		t.Fatal(err)
	}
	if body.read != int64(len(buffer)) {
		t.Fatal("reader staged an entire terabyte source", body.read)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
}

type cancelledStream struct {
	ctx     context.Context
	entered chan struct{}
	closed  atomic.Bool
}

func (s *cancelledStream) Read([]byte) (int, error) {
	close(s.entered)
	<-s.ctx.Done()
	return 0, s.ctx.Err()
}
func (s *cancelledStream) Close() error { s.closed.Store(true); return nil }

func TestFrozenSourceCloseCancelsABlockedReaderBeforeWaitingForItsLock(t *testing.T) {
	entered := make(chan struct{})
	var body *cancelledStream
	source := &streamSource{open: func(ctx context.Context) (io.ReadCloser, error) {
		body = &cancelledStream{ctx: ctx, entered: entered}
		return body, nil
	}}
	reader, err := openSource(t.Context(), source, readerScope, readerEpoch, "file", "source", 100, "")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := reader.ReadAt(make([]byte, 10), 0); done <- err }()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- reader.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close waited on a blocked body before cancelling it")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("read survived close", err)
	}
	if !body.closed.Load() {
		t.Fatal("original HTTP body not closed")
	}
}
