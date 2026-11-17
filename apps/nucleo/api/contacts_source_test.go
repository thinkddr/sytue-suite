// SPDX-License-Identifier: Apache-2.0
package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestContactsRetainedSourceRequiresVersionedPersonalSnapshotAndExactBytes(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "contacts"}
	epoch := "10000000-0000-4000-8000-000000000001"
	now := time.Now()
	raw := `{"note":"private contact","number":900719925474099512345}`
	hash := sha256.Sum256([]byte(raw))
	row := storage.StreamSourceRow{Kind: "card", ID: "10000000-0000-4000-8000-000000000002", Size: int64(len(raw)), SHA256: hex.EncodeToString(hash[:])}
	var mode, calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if storage.ValidateRequestScope(r.Header, scope) != nil {
			t.Error("lost source job scope")
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/rows"):
			after, err := storage.DecodeSourceCursor(r.URL.Query().Get("after"))
			if err != nil || after != (storage.LegacySourceCursor{Kind: "book", ID: epoch}) {
				t.Error("changed source cursor", err)
			}
			page := storage.StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{row}}
			if mode.Load() == 3 {
				page.Scope.Subject = "bob"
			}
			_ = json.NewEncoder(w).Encode(page)
		case strings.Contains(r.URL.Path, "/blobs/"):
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Sytue-Source-SHA256", row.SHA256)
			if mode.Load() == 4 {
				w.Header().Set("X-Sytue-Source-SHA256", strings.Repeat("0", 64))
			}
			_, _ = io.WriteString(w, raw)
		default:
			if r.Method == http.MethodPost {
				var input struct {
					Epoch string `json:"epoch"`
				}
				if json.NewDecoder(r.Body).Decode(&input) != nil || input.Epoch != epoch {
					t.Error("changed begin epoch")
				}
			}
			state := PrivateAppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &now, Rows: 1}}
			if mode.Load() == 1 {
				state.Version = 0
			}
			if mode.Load() == 2 {
				state.Shared = true
			}
			_ = json.NewEncoder(w).Encode(state)
		}
	}))
	defer server.Close()
	client := NewContacts(server.URL, nil, server.Client())
	if _, err := client.PrivateStorageState(t.Context(), scope); err != nil {
		t.Fatal(err)
	}
	if _, err := client.BeginPrivateStorageMigration(t.Context(), scope, epoch); err != nil {
		t.Fatal(err)
	}
	after := storage.LegacySourceCursor{Kind: "book", ID: epoch}
	if page, err := client.PrivateStorageSourcePage(t.Context(), scope, epoch, after); err != nil || len(page.Rows) != 1 || page.Rows[0] != row {
		t.Fatal("changed source inventory", err)
	}
	body, err := client.OpenPrivateStorageSource(t.Context(), scope, epoch, row)
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(bytes) != raw {
		t.Fatal("source JSON was parsed or rounded", err)
	}
	for _, stateMode := range []int32{1, 2} {
		mode.Store(stateMode)
		if _, err := client.PrivateStorageState(t.Context(), scope); err == nil {
			t.Fatal("accepted unversioned/shared source")
		}
	}
	mode.Store(3)
	if _, err := client.PrivateStorageSourcePage(t.Context(), scope, epoch, after); err == nil {
		t.Fatal("accepted another person's source page")
	}
	mode.Store(4)
	if body, err := client.OpenPrivateStorageSource(t.Context(), scope, epoch, row); err == nil {
		_ = body.Close()
		t.Fatal("accepted another source digest")
	}
	before := calls.Load()
	other := scope
	other.Product = "notes"
	if _, err := client.PrivateStorageState(t.Context(), other); err == nil {
		t.Fatal("accepted foreign product")
	}
	if _, err := client.BeginPrivateStorageMigration(t.Context(), scope, "bad"); err == nil {
		t.Fatal("accepted malformed epoch")
	}
	invalid := row
	invalid.Kind = "unknown"
	if _, err := client.OpenPrivateStorageSource(t.Context(), scope, epoch, invalid); err == nil {
		t.Fatal("accepted unknown source kind")
	}
	if calls.Load() != before {
		t.Fatal("invalid source request reached network")
	}
}
