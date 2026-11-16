// SPDX-License-Identifier: Apache-2.0
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

type (
	semanticServiceContextKey struct{}
	semanticServicePort       struct {
		mu                          sync.Mutex
		identity                    webport.Identity
		current, staleAfter, closes int
		descriptor, read            []byte
		closeErr                    error
	}
)

func (p *semanticServicePort) ID() string { return strings.Repeat("a", 64) }
func (p *semanticServicePort) Current(ctx context.Context) (webport.Identity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current++
	value := p.identity
	if p.current > p.staleAfter {
		value.Scope.Subject = "foreign"
	}
	return value, ctx.Err()
}

func (p *semanticServicePort) Call(ctx context.Context, scope storage.Scope, method webport.Method, raw []byte) ([]byte, error) {
	if scope != p.identity.Scope {
		return nil, webport.ErrRejected
	}
	switch method {
	case webport.ServiceRequest:
		return p.descriptor, nil
	case webport.ServiceRead:
		return p.read, nil
	case webport.ServiceClose:
		if ctx.Err() != nil || ctx.Value(semanticServiceContextKey{}) != "original" {
			return nil, errors.New("close replaced original context")
		}
		var value map[string]string
		if json.Unmarshal(raw, &value) != nil || len(value) != 1 || value["stream"] != strings.Repeat("b", 64) {
			return nil, webport.ErrRejected
		}
		p.mu.Lock()
		p.closes++
		p.mu.Unlock()
		return nil, p.closeErr
	default:
		return nil, webport.ErrRejected
	}
}

func semanticServiceRequest(t *testing.T, p *semanticServicePort) (*ServiceTransport, *http.Request) {
	t.Helper()
	ctx := context.WithValue(t.Context(), semanticServiceContextKey{}, "original")
	owner, err := webport.OpenStorage(ctx, p, webport.NewSources())
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewServiceTransport(owner, "mail")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://mail.invalid/api/v1/private-storage/migration", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := storage.EncodeScopeHeader(p.identity.Scope)
	req.Header.Set(storage.ScopeHeader, scope)
	return transport, req
}

func TestSemanticServiceDescriptorClosesOriginalStreamEvenWhenCurrentChanged(t *testing.T) {
	for _, kind := range []string{"status", "length", "type", "hash", "json"} {
		t.Run(kind, func(t *testing.T) {
			value := map[string]any{"status": 200, "stream": strings.Repeat("b", 64), "content_length": 0, "content_type": "application/json"}
			switch kind {
			case "status":
				value["status"] = 199
			case "length":
				value["content_length"] = -2
			case "type":
				value["content_type"] = strings.Repeat("x", 257)
			case "hash":
				value["source_sha256"] = "invalid"
			case "json":
				value["status"] = "invalid"
			}
			raw, _ := json.Marshal(value)
			failure := errors.New("service original close failed")
			p := &semanticServicePort{identity: fixtureIdentity(), staleAfter: 3, descriptor: raw, closeErr: failure}
			transport, req := semanticServiceRequest(t, p)
			res, err := transport.RoundTrip(req)
			if res != nil || !errors.Is(err, webport.ErrRejected) || !errors.Is(err, failure) {
				t.Fatal("descriptor rejection hid cleanup failure", kind, err)
			}
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.current != 3 || p.closes != 1 {
				t.Fatal("semantic close queried replacement Current or leaked", p.current, p.closes)
			}
			if !bytes.Equal(raw, make([]byte, len(raw))) {
				t.Fatal("invalid service descriptor retained bytes")
			}
		})
	}
}

func TestSemanticServiceReadClosesBeforeReturningAndRetainsCloseFailure(t *testing.T) {
	raw, _ := json.Marshal(serviceResponse{Status: 200, Stream: strings.Repeat("b", 64), ContentLength: 2, ContentType: "application/json"})
	failure := errors.New("read original close failed")
	body := []byte("xx")
	p := &semanticServicePort{identity: fixtureIdentity(), staleAfter: 5, descriptor: raw, read: body, closeErr: failure}
	transport, req := semanticServiceRequest(t, p)
	res, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, 1)
	n, err := res.Body.Read(out)
	if n != 0 || !errors.Is(err, webport.ErrRejected) || !errors.Is(err, failure) {
		t.Fatal("oversized read hid original close failure", n, err)
	}
	if err = res.Body.Close(); !errors.Is(err, failure) {
		t.Fatal("repeated body close hid prior failure", err)
	}
	if !bytes.Equal(body, make([]byte, len(body))) {
		t.Fatal("rejected service data retained bytes")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current != 5 || p.closes != 1 {
		t.Fatal("read did not close original stream once", p.current, p.closes)
	}
}
