// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type (
	discardedReplyContextKey struct{}
	discardedReplyPort       struct {
		mu                             sync.Mutex
		identity                       Identity
		method, closed                 Method
		field                          string
		response                       []byte
		stale, cancelDuring            bool
		cleanupEntered, cleanupRelease chan struct{}
		cleanupErr                     error
		cancel                         context.CancelFunc
	}
)

func (p *discardedReplyPort) ID() string { return strings.Repeat("a", 64) }
func (p *discardedReplyPort) Current(ctx context.Context) (Identity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	value := p.identity
	if p.stale {
		value.Scope.Subject = "foreign"
	}
	return value, ctx.Err()
}

func (p *discardedReplyPort) Call(ctx context.Context, scope storage.Scope, method Method, raw []byte) ([]byte, error) {
	if scope != p.identity.Scope {
		return nil, ErrRejected
	}
	p.mu.Lock()
	if method == p.method {
		p.stale = true
		if p.cancelDuring {
			p.cancel()
		}
		response := p.response
		p.mu.Unlock()
		return response, nil
	}
	if method != p.closed {
		p.mu.Unlock()
		return nil, ErrRejected
	}
	p.mu.Unlock()
	if ctx.Err() != nil || ctx.Value(discardedReplyContextKey{}) != "original" {
		return nil, errors.New("cleanup lost original context values or kept cancellation")
	}
	var value map[string]string
	if json.Unmarshal(raw, &value) != nil || len(value) != 1 || value[p.field] != strings.Repeat("b", 64) {
		return nil, ErrRejected
	}
	close(p.cleanupEntered)
	<-p.cleanupRelease
	return nil, p.cleanupErr
}

func TestDiscardedHostCapabilityJoinsCleanupAfterStalePostIdentity(t *testing.T) {
	for _, item := range []struct {
		method, closed Method
		field          string
	}{
		{LeaseAcquire, LeaseRelease, "lease"},
		{JournalLock, JournalUnlock, "lock"},
		{StorageFileDownload, StorageFileClose, "stream"},
		{SourceOpen, SourceClose, "stream"},
		{ServiceRequest, ServiceClose, "stream"},
	} {
		t.Run(string(item.method), func(t *testing.T) {
			raw, _ := json.Marshal(map[string]string{item.field: strings.Repeat("b", 64)})
			p := &discardedReplyPort{identity: fixtureIdentity(), method: item.method, closed: item.closed, field: item.field, response: raw, cleanupEntered: make(chan struct{}), cleanupRelease: make(chan struct{})}
			ctx := context.WithValue(t.Context(), discardedReplyContextKey{}, "original")
			owner, err := OpenStorage(ctx, p, NewSources())
			if err != nil {
				t.Fatal(err)
			}
			var release sync.Once
			defer release.Do(func() { close(p.cleanupRelease) })
			done := make(chan error, 1)
			go func() {
				response, e := owner.callBytesOwned(ctx, item.method, nil)
				if response != nil {
					e = errors.New("stale response escaped")
				}
				done <- e
			}()
			select {
			case <-p.cleanupEntered:
			case err := <-done:
				t.Fatal("discarded capability returned without cleanup", err)
			}
			select {
			case err := <-done:
				t.Fatal("operation released before discarded handle cleanup joined", err)
			default:
			}
			release.Do(func() { close(p.cleanupRelease) })
			if err = <-done; !errors.Is(err, ErrRejected) {
				t.Fatal("stale POST identity did not fail closed", err)
			}
			if !bytes.Equal(raw, make([]byte, len(raw))) {
				t.Fatal("discarded response retained bytes")
			}
		})
	}
}

func TestDiscardedHostCapabilityPreservesCancellationAndCleanupFailure(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"stream": strings.Repeat("b", 64)})
	cleanupFailure := errors.New("original handle close failed")
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), discardedReplyContextKey{}, "original"))
	defer cancel()
	p := &discardedReplyPort{identity: fixtureIdentity(), method: SourceOpen, closed: SourceClose, field: "stream", response: raw, cancelDuring: true, cancel: cancel, cleanupEntered: make(chan struct{}), cleanupRelease: make(chan struct{}), cleanupErr: cleanupFailure}
	owner, err := OpenStorage(ctx, p, NewSources())
	if err != nil {
		t.Fatal(err)
	}
	close(p.cleanupRelease)
	response, err := owner.callBytesOwned(ctx, SourceOpen, nil)
	if response != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cleanupFailure) {
		t.Fatal("discard hid cancellation or cleanup failure", err)
	}
	if !bytes.Equal(raw, make([]byte, len(raw))) {
		t.Fatal("failed cleanup left response bytes")
	}
}

func TestDiscardedHostReplyCannotChooseCleanupRouteOrForeignIdentifier(t *testing.T) {
	for _, value := range []map[string]string{
		{},
		{"stream": "invalid", "method": "lease.release"},
		{"stream": strings.Repeat("b", 64), "lease": strings.Repeat("c", 64)},
	} {
		p := &discardedReplyPort{identity: fixtureIdentity(), method: SourceOpen, closed: SourceClose, field: "stream", cleanupEntered: make(chan struct{}), cleanupRelease: make(chan struct{})}
		close(p.cleanupRelease)
		ctx := context.WithValue(t.Context(), discardedReplyContextKey{}, "original")
		body, _ := json.Marshal(value)
		err := discardHostReply(ctx, p, p.identity.Scope, SourceOpen, body)
		if value["stream"] == strings.Repeat("b", 64) {
			if err != nil {
				t.Fatal("fixed source close route rejected", err)
			}
			select {
			case <-p.cleanupEntered:
			default:
				t.Fatal("valid unadopted stream was not closed")
			}
		} else {
			if !errors.Is(err, ErrRejected) {
				t.Fatal("invalid handle was adopted", err)
			}
			select {
			case <-p.cleanupEntered:
				t.Fatal("invalid response chose cleanup effect")
			default:
			}
		}
		if !bytes.Equal(body, make([]byte, len(body))) {
			t.Fatal("response was not erased")
		}
	}
}

func TestDiscardedOrdinaryReplyIsErasedWithoutEffect(t *testing.T) {
	p := &discardedReplyPort{identity: fixtureIdentity(), cleanupEntered: make(chan struct{}), cleanupRelease: make(chan struct{})}
	body := []byte("private bytes")
	if err := discardHostReply(t.Context(), p, p.identity.Scope, StorageReadRecord, body); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, make([]byte, len(body))) {
		t.Fatal("ordinary late plaintext retained")
	}
	select {
	case <-p.cleanupEntered:
		t.Fatal("ordinary result selected a close method")
	default:
	}
}

func TestMalformedHostDescriptorClosesItsUnadoptedCapability(t *testing.T) {
	identity := fixtureIdentity()
	id := strings.Repeat("b", 64)
	raw, _ := json.Marshal(map[string]string{"stream": id})
	closed := false
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method Method, input []byte) ([]byte, error) {
		switch method {
		case SourceOpen:
			return raw, nil
		case SourceClose:
			var value map[string]string
			if json.Unmarshal(input, &value) != nil || len(value) != 1 || value["stream"] != id {
				return nil, ErrRejected
			}
			closed = true
			return nil, nil
		default:
			return nil, ErrRejected
		}
	}}
	owner, err := OpenStorage(t.Context(), port, NewSources())
	if err != nil {
		t.Fatal(err)
	}
	// A typed descriptor cannot adopt the string capability as a numeric field.
	var invalid struct{ Stream int }
	if err = owner.call(t.Context(), SourceOpen, nil, &invalid); !errors.Is(err, ErrRejected) || !closed {
		t.Fatal("invalid descriptor leaked its valid host handle", err, closed)
	}
	if !bytes.Equal(raw, make([]byte, len(raw))) {
		t.Fatal("invalid descriptor bytes survived")
	}
}
