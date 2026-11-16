// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestCipherUsageRequestPinsOwnerAndRejectsWrongProjection(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	value := storage.CipherUsage{Version: 1, Scope: scope, Bytes: 100, SytueBytes: 60, ExternalBytes: 40, PendingDeleteBytes: 60, Blobs: 2}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		got, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
		if err != nil || got != scope || r.URL.Path != "/api/v1/cipher/usage/mail" || r.Method != http.MethodGet {
			t.Error("usage request escaped captured scope")
		}
		_ = json.NewEncoder(w).Encode(value)
	}))
	defer server.Close()
	client := NewDrive(server.URL, &sesionFalsa{tokens: []string{"artificial"}}, server.Client())
	if got, err := client.CipherUsage(t.Context(), scope); err != nil || got != value {
		t.Fatal("usage projection changed", err)
	}
	value.Scope.Subject = "bob"
	if _, err := client.CipherUsage(t.Context(), scope); err == nil {
		t.Fatal("foreign ciphertext usage accepted")
	}
	value.Scope = scope
	value.ExternalBytes = 41
	if _, err := client.CipherUsage(t.Context(), scope); err == nil {
		t.Fatal("inconsistent ciphertext totals accepted")
	}
	invalid := scope
	invalid.Subject = ""
	if _, err := client.CipherUsage(t.Context(), invalid); err == nil || calls != 3 {
		t.Fatal("invalid scope reached network", err, calls)
	}
}
