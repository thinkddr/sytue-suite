// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type semanticHandlePort struct {
	mu                          sync.Mutex
	identity                    Identity
	current, staleAfter, closes int
	open, close                 Method
	descriptor                  []byte
	read                        []byte
	closeErr                    error
	entered, release            chan struct{}
}

func (p *semanticHandlePort) ID() string { return strings.Repeat("a", 64) }
func (p *semanticHandlePort) Current(ctx context.Context) (Identity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current++
	value := p.identity
	if p.current > p.staleAfter {
		value.Scope.Subject = "foreign"
	}
	return value, ctx.Err()
}

func (p *semanticHandlePort) Call(ctx context.Context, scope storage.Scope, method Method, raw []byte) ([]byte, error) {
	if scope != p.identity.Scope {
		return nil, ErrRejected
	}
	if method == p.open {
		return p.descriptor, nil
	}
	if method == StorageFileRead || method == SourceRead {
		return p.read, nil
	}
	if method != p.close {
		return nil, ErrRejected
	}
	if ctx.Err() != nil || ctx.Value(discardedReplyContextKey{}) != "original" {
		return nil, errors.New("semantic close replaced original context")
	}
	var value map[string]string
	if json.Unmarshal(raw, &value) != nil || len(value) != 1 || value["stream"] != strings.Repeat("b", 64) {
		return nil, ErrRejected
	}
	p.mu.Lock()
	p.closes++
	p.mu.Unlock()
	if p.entered != nil {
		close(p.entered)
		<-p.release
	}
	return nil, p.closeErr
}

func semanticOwner(t *testing.T, p *semanticHandlePort) (context.Context, *Storage) {
	t.Helper()
	ctx := context.WithValue(t.Context(), discardedReplyContextKey{}, "original")
	owner, err := OpenStorage(ctx, p, NewSources())
	if err != nil {
		t.Fatal(err)
	}
	return ctx, owner
}

func TestSourceSemanticMismatchJoinsOriginalHandleCloseAndRetainsError(t *testing.T) {
	epoch := "018f3340-7b12-7000-8000-000000000001"
	sum := sha256.Sum256([]byte("x"))
	row := storage.StreamSourceRow{Kind: "mime_message", ID: epoch, Size: 1, SHA256: hex.EncodeToString(sum[:])}
	changed := row
	changed.Size++
	raw, _ := json.Marshal(map[string]any{"stream": strings.Repeat("b", 64), "row": changed})
	closeFailure := errors.New("source original close failed")
	p := &semanticHandlePort{identity: fixtureIdentity(), staleAfter: 3, open: SourceOpen, close: SourceClose, descriptor: raw, closeErr: closeFailure, entered: make(chan struct{}), release: make(chan struct{})}
	ctx, owner := semanticOwner(t, p)
	source, err := NewSource(owner, false)
	if err != nil {
		t.Fatal(err)
	}
	var release sync.Once
	defer release.Do(func() { close(p.release) })
	done := make(chan error, 1)
	go func() {
		reader, e := source.OpenPrivateStorageSource(ctx, p.identity.Scope, epoch, row)
		if reader != nil {
			e = errors.New("mismatched row returned a reader")
		}
		done <- e
	}()
	select {
	case <-p.entered:
	case e := <-done:
		t.Fatal("mismatched row returned before original close", e)
	}
	select {
	case e := <-done:
		t.Fatal("mismatched row did not join original close", e)
	default:
	}
	release.Do(func() { close(p.release) })
	if err = <-done; !errors.Is(err, ErrRejected) || !errors.Is(err, closeFailure) {
		t.Fatal("semantic or cleanup failure was hidden", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current != 3 || p.closes != 1 {
		t.Fatal("close queried replacement Current or leaked stream", p.current, p.closes)
	}
}

func TestDownloadSemanticFailuresCloseOriginalStreamAndRetainCloseError(t *testing.T) {
	for _, kind := range []string{"request", "hash", "short", "checksum"} {
		t.Run(kind, func(t *testing.T) {
			sum := sha256.Sum256([]byte("x"))
			request := storage.FileRequest{ObjectID: "object", RevisionID: "revision", Metadata: []byte("{}"), SizeBytes: 1}
			checksum := hex.EncodeToString(sum[:])
			read := []byte("x")
			want := ErrRejected
			switch kind {
			case "request":
				request.ObjectID = "foreign"
			case "hash":
				checksum = "invalid"
			case "short":
				read = nil
				want = io.ErrUnexpectedEOF
			case "checksum":
				checksum = strings.Repeat("a", 64)
			}
			raw, _ := json.Marshal(map[string]any{"stream": strings.Repeat("b", 64), "request": request, "sha256": checksum})
			failure := errors.New("download original close failed")
			p := &semanticHandlePort{identity: fixtureIdentity(), staleAfter: 3, open: StorageFileDownload, close: StorageFileClose, descriptor: raw, read: read, closeErr: failure}
			ctx, owner := semanticOwner(t, p)
			var out bytes.Buffer
			_, err := owner.Download(ctx, storage.Reference{RoomID: "!room:test", EventID: "$file"}, "object", "revision", &out)
			clear(out.Bytes())
			if !errors.Is(err, want) || !errors.Is(err, failure) {
				t.Fatal("download lost semantic or close failure", kind, err)
			}
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.current != 3 || p.closes != 1 {
				t.Fatal("download close adopted changed Current or leaked", p.current, p.closes)
			}
		})
	}
}

func TestSourceReadMismatchClosesBeforeReturningAndRetainsCloseFailure(t *testing.T) {
	epoch := "018f3340-7b12-7000-8000-000000000001"
	sum := sha256.Sum256([]byte("x"))
	row := storage.StreamSourceRow{Kind: "mime_message", ID: epoch, Size: 1, SHA256: hex.EncodeToString(sum[:])}
	raw, _ := json.Marshal(map[string]any{"stream": strings.Repeat("b", 64), "row": row})
	failure := errors.New("reader original close failed")
	p := &semanticHandlePort{identity: fixtureIdentity(), staleAfter: 5, open: SourceOpen, close: SourceClose, descriptor: raw, read: nil, closeErr: failure}
	ctx, owner := semanticOwner(t, p)
	source, _ := NewSource(owner, false)
	reader, err := source.OpenPrivateStorageSource(ctx, p.identity.Scope, epoch, row)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, 1)
	n, err := reader.Read(out)
	if n != 0 || !errors.Is(err, io.ErrUnexpectedEOF) || !errors.Is(err, failure) {
		t.Fatal("read mismatch lost original close failure", n, err)
	}
	if err = reader.Close(); !errors.Is(err, failure) {
		t.Fatal("repeated close hid original failure", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current != 5 || p.closes != 1 {
		t.Fatal("read mismatch did not close exactly original handle", p.current, p.closes)
	}
}

func TestCopyRejectionPreservesOriginalStreamCloseFailure(t *testing.T) {
	request := storage.FileRequest{ObjectID: "object", RevisionID: "revision", Metadata: []byte("{}"), SizeBytes: 1}
	raw, _ := json.Marshal(map[string]any{"stream": strings.Repeat("b", 64), "request": request, "sha256": "invalid"})
	failure := errors.New("copy original close failed")
	p := &semanticHandlePort{identity: fixtureIdentity(), staleAfter: 3, open: StorageFileDownload, close: StorageFileClose, descriptor: raw, closeErr: failure}
	ctx, owner := semanticOwner(t, p)
	_, err := owner.CopyTo(ctx, owner, storage.Reference{RoomID: "!room:test", EventID: "$file"}, "object", "revision", "job", storage.PublicationTarget{RoomID: "!room:test", DeviceID: p.identity.Binding.DeviceID}, request)
	if !errors.Is(err, ErrRejected) || !errors.Is(err, failure) {
		t.Fatal("copy rejection hid original close failure", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current != 3 || p.closes != 1 {
		t.Fatal("copy did not close original stream once", p.current, p.closes)
	}
}
