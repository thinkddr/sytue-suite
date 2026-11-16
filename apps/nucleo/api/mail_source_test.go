// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestMailSourceClientScopesDescriptorsAndExactBody(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	epoch := "12345678-1234-1234-1234-123456789abc"
	now := time.Now()
	row := storage.StreamSourceRow{Kind: "message", ID: "private-id", Size: 8, SHA256: strings.Repeat("a", 64)}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		got, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
		if err != nil || got != scope {
			t.Error("wrong scope", err)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/rows"):
			_ = json.NewEncoder(w).Encode(storage.StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{row}})
		case strings.Contains(r.URL.Path, "/source/"):
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Sytue-Source-SHA256", row.SHA256)
			w.Header().Set("Content-Length", "8")
			_, _ = io.WriteString(w, "original")
		default:
			_ = json.NewEncoder(w).Encode(storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &now, Rows: 1})
		}
	}))
	defer server.Close()
	client := NewMail(server.URL, &sesionFalsa{tokens: []string{"synthetic"}}, server.Client())
	if _, err := client.PrivateStorageState(t.Context(), scope); err != nil {
		t.Fatal(err)
	}
	if _, err := client.BeginPrivateStorageMigration(t.Context(), scope, epoch); err != nil {
		t.Fatal(err)
	}
	page, err := client.PrivateStorageSourcePage(t.Context(), scope, epoch, storage.LegacySourceCursor{})
	if err != nil || len(page.Rows) != 1 || page.Rows[0] != row {
		t.Fatal("descriptor changed", err)
	}
	body, err := client.OpenPrivateStorageSource(t.Context(), scope, epoch, row)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(raw) != "original" {
		t.Fatal("source changed", err)
	}
	bad := scope
	bad.Product = "drive"
	if _, err := client.PrivateStorageState(t.Context(), bad); err == nil {
		t.Fatal("foreign product reached Mail")
	}
	if calls != 4 {
		t.Fatal("invalid request reached source", calls)
	}
}

func TestPrivateAppMigrationStatePreservesCapabilityFields(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	state := PrivateAppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}, Shared: true}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PrivateAppMigrationState
	if json.Unmarshal(raw, &decoded) != nil || decoded.Validate(scope) != nil || !decoded.Shared || decoded.Version != 1 {
		t.Fatal("lost source capabilities", decoded)
	}
	decoded.Version = 2
	if decoded.Validate(scope) == nil {
		t.Fatal("accepted unknown version")
	}
}

func TestMailSourceRejectsMissingCommitmentHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.WriteString(w, "x")
	}))
	defer server.Close()
	client := NewMail(server.URL, &sesionFalsa{tokens: []string{"synthetic"}}, server.Client())
	row := storage.StreamSourceRow{Kind: "message", ID: "x", Size: 1, SHA256: strings.Repeat("a", 64)}
	if _, err := client.OpenPrivateStorageSource(t.Context(), storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}, "12345678-1234-1234-1234-123456789abc", row); err == nil {
		t.Fatal("uncommitted source accepted")
	}
}

func TestPrivateAppMigrationStateRequiresExplicitSharedCapability(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "contacts"}
	for _, shared := range []string{"", `,"shared":null`} {
		raw := []byte(`{"v":1,"scope":{"tenant":"acme","subject":"alice","product":"contacts"},"mode":"legacy","rows":0,"blobs":0` + shared + `}`)
		var state PrivateAppMigrationState
		if err := json.Unmarshal(raw, &state); err == nil && state.Validate(scope) == nil {
			t.Fatal("missing collaborative capability was treated as private")
		}
	}
}

func TestPrivateAppMigrationRequestPreservesVersionAndExplicitCapabilities(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	epoch := "12345678-1234-1234-1234-123456789abc"
	now := time.Now()
	state := storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &now, Rows: 1}, MaxBodyBytes: 100 << 20, PrivateIngressReady: true}
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			var body struct {
				Epoch string `json:"epoch"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Epoch != epoch {
				t.Error("source epoch changed")
			}
		}
		_ = json.NewEncoder(w).Encode(state)
	}))
	defer server.Close()
	client := NewMail(server.URL, &sesionFalsa{tokens: []string{"synthetic"}}, server.Client())
	got, err := client.PrivateStorageCapabilities(t.Context(), scope)
	if err != nil || got.Validate(scope) != nil || !got.PrivateIngressReady || got.MaxBodyBytes != 100<<20 || got.Shared {
		t.Fatal("lost source capabilities", got, err)
	}
	begun, err := privateAppMigrationState(t.Context(), client.c, scope, "mail", epoch)
	if err != nil || begun.Epoch != epoch || begun.Version != 1 || posts != 1 {
		t.Fatal("typed begin did not use common request", begun, err)
	}
}

func TestPrivateMailFacadeCannotReopenTheRetainedSource(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	original := NewMail(server.URL, nil, server.Client())
	private, err := original.WithPrivateData(&privateMailFixture{scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := private.PrivateStorageCapabilities(t.Context(), scope); err == nil {
		t.Fatal("private facade opened legacy capability HTTP")
	}
	if calls != 0 {
		t.Fatal("private facade reached original source server")
	}
}
