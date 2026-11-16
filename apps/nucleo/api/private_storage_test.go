// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateStorageMigrationClientScopesAndStreamsOriginals(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	epoch := "12345678-1234-1234-1234-123456789abc"
	now := time.Now()
	calls := 0
	after := storage.LegacySourceCursor{Kind: "file", ID: "日本語"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		got, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
		if err != nil || got != scope {
			t.Error("source identity changed", err)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/rows"):
			cursor, err := base64.RawURLEncoding.DecodeString(r.URL.Query().Get("after"))
			if err != nil {
				t.Error(err)
			}
			var decoded storage.LegacySourceCursor
			if err := json.Unmarshal(cursor, &decoded); err != nil || decoded != after {
				t.Error("cursor changed", err)
			}
			_ = json.NewEncoder(w).Encode(storage.LegacySourcePage{Scope: scope, Epoch: epoch, Rows: []storage.LegacySourceRow{{Kind: "file", ID: "𐀀", Payload: []byte(`{"n":900719925474099512345}`)}}})
		case strings.Contains(r.URL.Path, "/blobs/"):
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", "8")
			_, _ = io.WriteString(w, "original")
		default:
			if r.URL.Path != "/api/v1/private-storage/migration" {
				t.Error("bad root", r.URL.Path)
			}
			if r.Method == http.MethodPost {
				var input struct {
					Epoch string `json:"epoch"`
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Epoch != epoch {
					t.Error("epoch changed", err)
				}
			}
			_ = json.NewEncoder(w).Encode(storage.LegacyMigrationState{Scope: scope, Epoch: epoch, StartedAt: &now, Mode: "frozen", Rows: 2, Blobs: 1})
		}
	}))
	defer server.Close()
	drive := NewDrive(server.URL, &sesionFalsa{tokens: []string{"synthetic"}}, server.Client())
	if _, err := drive.PrivateStorageState(t.Context(), scope); err != nil {
		t.Fatal(err)
	}
	if _, err := drive.BeginPrivateStorageMigration(t.Context(), scope, epoch); err != nil {
		t.Fatal(err)
	}
	page, err := drive.PrivateStorageSourcePage(t.Context(), scope, epoch, after)
	if err != nil || string(page.Rows[0].Payload) != `{"n":900719925474099512345}` {
		t.Fatal("source bytes changed", err)
	}
	body, err := drive.OpenPrivateStorageSource(t.Context(), scope, epoch, "file", "12345678-1234-1234-1234-123456789abc", 8)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(raw) != "original" {
		t.Fatal("source stream changed", err)
	}
	bad := scope
	bad.Product = "mail"
	if _, err := drive.PrivateStorageState(t.Context(), bad); err == nil {
		t.Fatal("foreign product queried Drive source")
	}
	if _, err := drive.BeginPrivateStorageMigration(t.Context(), scope, "bad"); err == nil {
		t.Fatal("invalid epoch sent")
	}
	if _, err := drive.PrivateStorageSourcePage(t.Context(), scope, epoch, storage.LegacySourceCursor{Kind: "file"}); err == nil {
		t.Fatal("invalid cursor sent")
	}
	if calls != 4 {
		t.Fatal("invalid source request reached backend", calls)
	}
}
