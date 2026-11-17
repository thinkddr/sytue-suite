// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestMailPrivateAccountProvisioningUsesExplicitScopedMetadataOnlyPOST(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	epoch := "12345678-1234-1234-1234-123456789abc"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/private-storage/migration" || storage.ValidateRequestScope(r.Header, scope) != nil {
			t.Error("unscoped provisioning request")
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 2 || body["epoch"] != epoch || body["provision"] != true {
			t.Error("provisioning was not explicit metadata-only")
		}
		now := time.Now()
		_ = json.NewEncoder(w).Encode(storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &now, Rows: 7})
	}))
	defer server.Close()
	client := NewMail(server.URL, &sesionFalsa{tokens: []string{"synthetic"}}, server.Client())
	if _, err := client.ProvisionAndBeginPrivateStorageMigration(t.Context(), scope, epoch); err != nil {
		t.Fatal(err)
	}
	foreign := scope
	foreign.Product = "drive"
	if _, err := client.ProvisionAndBeginPrivateStorageMigration(t.Context(), foreign, epoch); err == nil {
		t.Fatal("foreign product reached provisioning")
	}
	if calls != 1 {
		t.Fatal("invalid account scope had side effects")
	}
}
