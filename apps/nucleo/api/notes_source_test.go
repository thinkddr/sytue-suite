// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateNotesSourceTransportUsesBoundScopeAndExactStreams(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	epoch := "12345678-1234-1234-1234-123456789abc"
	raw := []byte(`{"id":"note","data":{"integer":900719925474099312345}}`)
	sum := sha256.Sum256(raw)
	row := storage.StreamSourceRow{Kind: "note", ID: "note", Size: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := storage.ValidateRequestScope(r.Header, scope); err != nil {
			t.Error("scope missing or wrong", err)
			w.WriteHeader(http.StatusConflict)
			return
		}
		if strings.Contains(r.URL.Path, "/source/") {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
			w.Header().Set("X-Sytue-Source-SHA256", row.SHA256)
			_, _ = w.Write(raw)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/rows") {
			_ = json.NewEncoder(w).Encode(storage.StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{row}})
			return
		}
		state := storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}
		if r.Method == http.MethodPost {
			now := time.Now().UTC()
			state.Mode = "frozen"
			state.Epoch = epoch
			state.StartedAt = &now
			state.Rows = 1
		}
		_ = json.NewEncoder(w).Encode(state)
	}))
	defer server.Close()
	source := NewNotes(server.URL, nil, server.Client())
	if _, err := source.PrivateStorageState(t.Context(), scope); err != nil {
		t.Fatal(err)
	}
	if _, err := source.BeginPrivateStorageMigration(t.Context(), scope, epoch); err != nil {
		t.Fatal(err)
	}
	page, err := source.PrivateStorageSourcePage(t.Context(), scope, epoch, storage.LegacySourceCursor{})
	if err != nil || len(page.Rows) != 1 {
		t.Fatal(err)
	}
	body, err := source.OpenPrivateStorageSource(t.Context(), scope, epoch, row)
	if err != nil {
		t.Fatal(err)
	}
	received, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(received, raw) {
		t.Fatal("exact source stream changed", err)
	}
	before := calls
	bad := scope
	bad.Product = "drive"
	if _, err := source.PrivateStorageState(t.Context(), bad); err == nil || calls != before {
		t.Fatal("wrong product reached HTTP")
	}
}
