// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type privateBoundarySession struct{ calls atomic.Int32 }

func (s *privateBoundarySession) AccessToken(context.Context) (string, error) {
	s.calls.Add(1)
	return "synthetic-token", nil
}
func (*privateBoundarySession) Invalidate(string) {}

type closingPrivateBody struct {
	io.Reader
	closed bool
}

func (b *closingPrivateBody) Close() error { b.closed = true; return nil }

func TestPrivateDataClientNeverInheritsSessionOrCallsService(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	authority := &privateBoundarySession{}
	original := New(server.URL, authority, server.Client())
	denied := errors.New("synthetic private operation unavailable")
	private := privateDataClient(original, denied)
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		if _, err := private.Do(t.Context(), Request{Method: method, Path: "/new-future-route", Body: []byte("private application content")}); !errors.Is(err, denied) {
			t.Fatal("unimplemented operation reached old transport", method, err)
		}
	}
	if requests.Load() != 0 || authority.calls.Load() != 0 || private.session != nil || private.base != original.base {
		t.Fatal("private facade inherited authority or changed routing")
	}
	response, err := original.Do(context.Background(), Request{Path: "/still-original"})
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if requests.Load() != 1 {
		t.Fatal("private facade changed the original client")
	}
	if privateDataClient(nil, denied) != nil || privateDataClient(original, nil) != nil {
		t.Fatal("invalid private boundary created a usable client")
	}
}

func TestPrivateDataTransportClosesUnsentBodiesAndPreservesProductError(t *testing.T) {
	denied := errors.New("synthetic product-specific boundary")
	body := &closingPrivateBody{Reader: strings.NewReader("content never sent")}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://unused.invalid", body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := (privateDataHTTP{denied: denied}).RoundTrip(req)
	if response != nil || !errors.Is(err, denied) || !body.closed {
		t.Fatal("private body or product error escaped rejection", err)
	}
}
