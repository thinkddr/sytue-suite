// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration && cgo

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	privatedrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateDriveWebObject struct {
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	Raw           []byte `json:"raw"`
	Name          string `json:"name"`
	BlockExternal bool   `json:"block_external"`
	Binary        []byte `json:"binary,omitempty"`
}

// Export only this fixture's artificial account, authenticated records and
// synthetic ciphertext. No session, token or recovery material is exported.
func exportPrivateDriveWebVector(ctx context.Context, t *testing.T, client *Client, repository *privatedrive.Repository, source *syntheticDriveSource, gateway *fileGatewayFixture) {
	t.Helper()
	directory := os.Getenv("SYTUE_DRIVE_PRIVATE_BROWSER_VECTOR")
	if directory == "" {
		return
	}
	if !filepath.IsAbs(directory) || !strings.HasPrefix(filepath.Clean(directory), "/tmp/sytue-private-drive-web-vector-") {
		t.Fatal("disposable absolute fixture directory required")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	names, err := os.ReadDir(directory)
	if err != nil || len(names) != 0 {
		t.Fatal("fixture directory must be empty", err)
	}
	binding, err := client.StorageBindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vector := struct {
		Artificial bool                         `json:"artificial"`
		Scope      storage.Scope                `json:"scope"`
		State      storage.LegacyMigrationState `json:"state"`
		Rooms      []string                     `json:"rooms"`
		Entries    []storage.RecordEntry        `json:"entries"`
		Objects    []privateDriveWebObject      `json:"objects"`
	}{Artificial: true, Scope: binding.Scope, State: source.State, Rooms: binding.Rooms}
	indexes := []storage.RecordEntry{}
	seen := map[storage.Reference]bool{}
	for _, room := range binding.Rooms {
		cursor := ""
		tokens := map[string]bool{}
		for {
			page, err := client.History(ctx, room, cursor, 100)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range page.Events {
				if seen[event.Reference] {
					t.Fatal("duplicate fixture record")
				}
				seen[event.Reference] = true
				record, err := client.readPrivateStorageRecord(ctx, event, binding.Scope, client.session.UserID.String())
				if err != nil {
					t.Fatal(err)
				}
				entry := storage.RecordEntry{Reference: event.Reference, Record: record}
				vector.Entries = append(vector.Entries, entry)
				if record.Role == "index" {
					indexes = append(indexes, entry)
				}
			}
			if page.Next == "" {
				break
			}
			if page.Next == cursor || tokens[page.Next] {
				t.Fatal("repeated fixture history cursor")
			}
			tokens[page.Next] = true
			cursor = page.Next
		}
	}
	heads, err := storage.CatalogHeads(binding.Scope, indexes)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range heads {
		var item struct {
			Kind       string `json:"kind"`
			SourceKind string `json:"source_kind"`
			SourceID   string `json:"source_id"`
		}
		if json.Unmarshal(version.Commit.Metadata, &item) != nil || item.Kind != "org.sytue.drive.item.v1" || item.SourceKind != "file" && item.SourceKind != "folder" {
			continue
		}
		object := privateDriveWebObject{Kind: item.SourceKind, ID: item.SourceID}
		if item.SourceKind == "file" {
			view, err := repository.File(ctx, item.SourceID)
			if errors.Is(err, privatedrive.ErrNotFound) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			object.Name = view.Name
			object.BlockExternal = view.Label != nil && view.Label.BlockExternalShare
			body, err := repository.Download(ctx, item.SourceID)
			if err != nil {
				t.Fatal(err)
			}
			object.Binary, err = io.ReadAll(body)
			_ = body.Close()
			if err != nil {
				t.Fatal(err)
			}
		} else {
			view, err := repository.Folder(ctx, item.SourceID)
			if errors.Is(err, privatedrive.ErrNotFound) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			object.Name = view.Name
		}
		object.Raw, err = repository.ReadRaw(ctx, item.SourceKind, item.SourceID)
		if err != nil {
			t.Fatal(err)
		}
		vector.Objects = append(vector.Objects, object)
	}
	gateway.mu.Lock()
	for id, data := range gateway.data {
		if !storage.BlobID(id) {
			gateway.mu.Unlock()
			t.Fatal("invalid fixture blob id")
		}
		if err := os.WriteFile(filepath.Join(directory, id), data, 0o600); err != nil {
			gateway.mu.Unlock()
			t.Fatal(err)
		}
	}
	gateway.mu.Unlock()
	encoded, err := json.Marshal(vector)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "vector.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("exported artificial native/web Drive fixture: %d authenticated records, %d visible objects", len(vector.Entries), len(vector.Objects))
}
