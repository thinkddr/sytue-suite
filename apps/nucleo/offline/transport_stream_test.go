// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type streamRoundTrip func(*http.Request) (*http.Response, error)

func (f streamRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type observedBody struct {
	io.Reader
	reads, closes int
	closeErr      error
}

func (b *observedBody) Read(p []byte) (int, error) { b.reads++; return b.Reader.Read(p) }
func (b *observedBody) Close() error               { b.closes++; return b.closeErr }

type failedStream struct{}

func (failedStream) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func streamFixture(t *testing.T, body io.Reader, length int64) (*Transport, *http.Request, *observedBody, *string) {
	t.Helper()
	store, _ := abrir(t)
	owner := "ada/tenant"
	raw := &observedBody{Reader: body}
	transport := &Transport{Store: store, Owner: func() string { return owner }, Product: "mail", CacheProducts: []string{"mail", "drive"}}
	transport.Base = streamRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: raw, ContentLength: length, Request: r}, nil
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://suite.invalid/api/list", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	return transport, req, raw, &owner
}

func assertUncached(t *testing.T, transport *Transport, req *http.Request) {
	t.Helper()
	var saved respuestaGuardada
	ok, err := transport.Store.Read("ada/tenant", espacioHTTP("mail"), req.URL.String(), &saved)
	if err != nil || ok {
		t.Fatalf("an invalid/unfinished stream was cached: ok=%v err=%v", ok, err)
	}
}

func TestTransportEntregaAntesDeLeerYPersisteSoloEOFIntegro(t *testing.T) {
	transport, req, raw, _ := streamFixture(t, strings.NewReader("artificial"), 10)
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if raw.reads != 0 {
		t.Fatal("RoundTrip consumed the body")
	}
	first := make([]byte, 3)
	if _, err := io.ReadFull(response.Body, first); err != nil {
		t.Fatal(err)
	}
	assertUncached(t, transport, req)
	rest, err := io.ReadAll(response.Body)
	if err != nil || string(first)+string(rest) != "artificial" {
		t.Fatal(string(rest), err)
	}
	if err := response.Body.Close(); err != nil || raw.closes != 1 {
		t.Fatal(err, raw.closes)
	}
	var saved respuestaGuardada
	ok, err := transport.Store.Read("ada/tenant", espacioHTTP("mail"), req.URL.String(), &saved)
	if err != nil || !ok || string(saved.Body) != "artificial" {
		t.Fatal(ok, err, string(saved.Body))
	}
}

func TestTransportNoGuardaTruncadoCanceladoCerradoNiOtraSesion(t *testing.T) {
	for _, kind := range []string{"truncated", "read-error", "close-error", "early-close", "canceled", "owner", "scope", "set-cookie", "range"} {
		t.Run(kind, func(t *testing.T) {
			transport, req, raw, owner := streamFixture(t, strings.NewReader("abc"), 3)
			revision := "one"
			transport.Scope = func() string { return revision }
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			req = req.WithContext(ctx)
			switch kind {
			case "truncated":
				transport.Base = streamRoundTrip(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: raw, ContentLength: 5, Header: make(http.Header), Request: r}, nil
				})
			case "read-error":
				raw.Reader = io.MultiReader(strings.NewReader("a"), failedStream{})
			case "close-error":
				raw.closeErr = io.ErrClosedPipe
			case "range":
				req.Header.Set("Range", "bytes=0-2")
			case "set-cookie":
				base := transport.Base
				transport.Base = streamRoundTrip(func(r *http.Request) (*http.Response, error) {
					resp, err := base.RoundTrip(r)
					resp.Header.Set("Set-Cookie", "artificial=nonsecret")
					return resp, err
				})
			}
			response, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "owner":
				*owner = "grace/tenant"
			case "scope":
				revision = "two"
			case "canceled":
				cancel()
			case "early-close":
				if err := response.Body.Close(); err != nil {
					t.Fatal(err)
				}
			}
			_, readErr := io.ReadAll(response.Body)
			closeErr := response.Body.Close()
			if kind == "read-error" && !errors.Is(readErr, io.ErrUnexpectedEOF) {
				t.Fatal("lost read error", readErr)
			}
			if kind == "close-error" && (!errors.Is(readErr, io.ErrClosedPipe) || !errors.Is(closeErr, io.ErrClosedPipe)) {
				t.Fatal("lost close error", readErr, closeErr)
			}
			assertUncached(t, transport, req)
		})
	}
}

type ownerChangingBackend struct {
	backend
	change func()
}

func (b ownerChangingBackend) get(key string) ([]byte, error) {
	data, err := b.backend.get(key)
	b.change()
	return data, err
}

func TestOfflineFallbackNoCruzaCambioDeDuenoDuranteLectura(t *testing.T) {
	transport, req, _, owner := streamFixture(t, strings.NewReader("abc"), 3)
	if err := transport.Store.Write(*owner, espacioHTTP("mail"), req.URL.String(), respuestaGuardada{Status: 200, Header: make(http.Header), Body: []byte("old")}); err != nil {
		t.Fatal(err)
	}
	transport.Store.b = ownerChangingBackend{backend: transport.Store.b, change: func() { *owner = "grace/tenant" }}
	networkErr := errors.New("artificial offline")
	transport.Base = streamRoundTrip(func(*http.Request) (*http.Response, error) { return nil, networkErr })
	response, err := transport.RoundTrip(req)
	if response != nil || !errors.Is(err, networkErr) {
		t.Fatal("old owner response escaped", response, err)
	}
}

func TestCacheWriteFailureDoesNotBreakNetworkBody(t *testing.T) {
	transport, req, _, _ := streamFixture(t, strings.NewReader("abc"), 3)
	transport.Store.b = benchmarkBackend{backend: transport.Store.b, fail: true}
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil || string(data) != "abc" {
		t.Fatal(string(data), err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	assertUncached(t, transport, req)
}

func TestChunkedOverLimitIsDeliveredButNeverCached(t *testing.T) {
	transport, req, _, _ := streamFixture(t, strings.NewReader(strings.Repeat("x", MaxCacheBytes+1)), -1)
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	size, err := io.Copy(io.Discard, response.Body)
	if err != nil || size != MaxCacheBytes+1 {
		t.Fatal(size, err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	assertUncached(t, transport, req)
}

func TestStoreWriteReplacesCorruptRecordWithoutReadingIt(t *testing.T) {
	store, _ := abrir(t)
	if err := store.b.put(clave("owner", "drafts", "one"), []byte("{broken")); err != nil {
		t.Fatal(err)
	}
	if err := store.Write("owner", "drafts", "one", map[string]string{"body": "artificial"}); err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	if ok, err := store.Read("owner", "drafts", "one", &value); err != nil || !ok || value["body"] != "artificial" {
		t.Fatal(ok, value, err)
	}
}

func TestRangeCannotUseWholeBodyOfflineFallback(t *testing.T) {
	transport, req, _, owner := streamFixture(t, strings.NewReader("abc"), 3)
	if err := transport.Store.Write(*owner, espacioHTTP("mail"), req.URL.String(), respuestaGuardada{Status: http.StatusOK, Header: make(http.Header), Body: []byte("whole")}); err != nil {
		t.Fatal(err)
	}
	networkErr := errors.New("artificial offline")
	transport.Base = streamRoundTrip(func(*http.Request) (*http.Response, error) { return nil, networkErr })
	req.Header.Set("Range", "bytes=0-1")
	response, err := transport.RoundTrip(req)
	if response != nil || !errors.Is(err, networkErr) {
		t.Fatal("Range received whole cached body", response, err)
	}
}
