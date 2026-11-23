// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// El modo para elegir entre la API y Matrix no rechaza una cuenta con libretas
// compartidas (en la API no importa) pero sí una respuesta de otra persona.
func TestContactsPrivateStorageModeAdmiteCompartidasYNoOtraPersona(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "contacts"}
	otra := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if storage.ValidateRequestScope(r.Header, scope) != nil || r.Method != http.MethodGet || r.URL.Path != "/api/v1/private-storage/migration" {
			t.Errorf("petición %s %s", r.Method, r.URL.Path)
		}
		state := PrivateAppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}, Shared: true}
		if otra {
			state.Scope.Subject = "bob"
		}
		_ = json.NewEncoder(w).Encode(state)
	}))
	defer server.Close()
	client := NewContacts(server.URL, nil, server.Client())
	if mode, err := client.PrivateStorageMode(t.Context(), scope); err != nil || mode != "legacy" {
		t.Fatalf("modo %q: %v", mode, err)
	}
	otra = true
	if _, err := client.PrivateStorageMode(t.Context(), scope); err == nil {
		t.Fatal("aceptó el estado de otra persona")
	}
}
