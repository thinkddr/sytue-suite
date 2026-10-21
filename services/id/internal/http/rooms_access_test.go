// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func TestRoomsAccessGuardRequiresRevocationAcknowledgement(t *testing.T) {
	status := http.StatusNoContent
	var mu sync.Mutex
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer lifecycle-secret" {
			t.Error("missing lifecycle authentication")
		}
		var body store.MatrixAccessChange
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Subject != "subject" || body.Version <= 0 {
			t.Errorf("bad request body: %+v, %v", body, err)
		}
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/internal/matrix/tenant-access" && (body.Tenant != "acme" || body.Status != "removed") {
			t.Errorf("bad organization departure: %+v", body)
		}
		w.Header().Set("Location", "/must-not-follow")
		w.WriteHeader(status)
	}))
	defer upstream.Close()
	guard, err := RoomsAccessGuard(upstream.URL, "lifecycle-secret")
	if err != nil {
		t.Fatal(err)
	}
	change := store.MatrixAccessChange{Subject: "subject", Status: "suspended", Version: 1}
	if err := guard.Apply(t.Context(), change); err != nil {
		t.Fatal(err)
	}
	if err := guard.Apply(t.Context(), store.MatrixAccessChange{Subject: "subject", Tenant: "acme", Status: "removed", Version: 2}); err != nil {
		t.Fatal(err)
	}
	for _, code := range []int{http.StatusTemporaryRedirect, http.StatusForbidden, http.StatusServiceUnavailable, http.StatusOK} {
		mu.Lock()
		status = code
		mu.Unlock()
		if err := guard.Apply(t.Context(), change); err == nil {
			t.Fatalf("accepted incomplete revocation: %d", code)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 6 {
		t.Fatalf("lifecycle request followed a redirect: %+v", paths)
	}
}

func TestRoomsAccessGuardRejectsIncompleteConfiguration(t *testing.T) {
	for _, c := range [][2]string{{"http://rooms", ""}, {"", "secret"}, {"http://user:password@rooms", "secret"}, {"https://rooms?token=x", "secret"}} {
		if _, err := RoomsAccessGuard(c[0], c[1]); err == nil {
			t.Fatal("accepted incomplete or credential-bearing lifecycle configuration")
		}
	}
}
