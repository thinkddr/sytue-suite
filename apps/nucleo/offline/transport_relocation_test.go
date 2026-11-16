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
	"sync/atomic"
	"testing"
)

func TestTransportRetainsRequestStoreAndFollowsVerifiedRelocationForNextRequest(t *testing.T) {
	transport, request, _, _ := streamFixture(t, strings.NewReader("before relocation"), 17)
	original := transport.Store
	relocated, _ := abrir(t)
	var current atomic.Pointer[Store]
	current.Store(original)
	transport.ResolveStore = func(context.Context) (*Store, error) { return current.Load(), nil }
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	current.Store(relocated)
	if _, err = io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	if err = response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	var captured respuestaGuardada
	if found, err := original.Read("ada/tenant", espacioHTTP("mail"), request.URL.String(), &captured); err != nil || !found || string(captured.Body) != "before relocation" {
		t.Fatal("in-flight body changed store midway", err)
	}
	target := respuestaGuardada{Status: http.StatusOK, Header: make(http.Header), Body: []byte("after relocation")}
	if err = relocated.Write("ada/tenant", espacioHTTP("mail"), request.URL.String(), target); err != nil {
		t.Fatal(err)
	}
	transport.Base = streamRoundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("synthetic offline") })
	response, err = transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || string(body) != "after relocation" {
		t.Fatal("next request read the retired source", string(body), err)
	}
}
