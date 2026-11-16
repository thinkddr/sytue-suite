// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// The consumer runs the maintained Matrix SDK against a disposable Synapse.
// This optional bridge contains exclusively synthetic data from real PostgreSQL.
func TestPrivateStorageExportSyntheticVectorForNativeImporter(t *testing.T) {
	destination := os.Getenv("SYTUE_DRIVE_PRIVATE_VECTOR")
	if destination == "" {
		t.Skip("requires remote native importer interoperability run")
	}
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := store.WithOwner(tenantCtx("artificial-storage"), "artificial")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatal(err)
	}
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := st.CreateFolder(ctx, root.ID, "Private synthetic folder", "artificial", false)
	if err != nil {
		t.Fatal(err)
	}
	bodies := map[string][]byte{}
	create := func(parent uuid.UUID, name, key string, data []byte) domain.File {
		t.Helper()
		sum := sha256.Sum256(data)
		bodies[key] = data
		file, err := st.CreateFile(ctx, store.NewFileRecord{FolderID: parent, Name: name, StorageKey: key, SizeBytes: int64(len(data)), ContentType: "application/octet-stream", Checksum: hex.EncodeToString(sum[:])})
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	create(root.ID, "Private synthetic file.bin", "synthetic/previous", bytes.Repeat([]byte("Synthetic previous content;"), 97))
	file := create(root.ID, "Private synthetic file.bin", "synthetic/current", bytes.Repeat([]byte("Synthetic current private content;"), 7301))
	if _, err := st.SetFileLabel(ctx, file.ID, domain.Label{ID: "synthetic-restricted", Name: "Private synthetic restricted", Rank: 2, BlockExternalShare: true}); err != nil {
		t.Fatal(err)
	}
	trashed := create(folder.ID, "Private synthetic trash.bin", "synthetic/trash", []byte("Synthetic retained trash"))
	if _, err := st.DeleteFile(ctx, trashed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateShare(ctx, store.NewShareRecord{FileID: &file.ID, Kind: domain.ShareKind("user"), Subject: "synthetic-bob", Permission: domain.PermRead, CreatedBy: "artificial"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFavorite(ctx, "artificial", &file.ID, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PedirEdicion(ctx, file.ID, peticiones.Peticion{Sujeto: "synthetic-bob", Nombre: "Private synthetic requester"}); err != nil {
		t.Fatal(err)
	}
	provenance := json.RawMessage(`{"integer":900719925474099512345,"description":"` + strings.Repeat("Private synthetic metadata;", 900) + `"}`)
	if err := st.GuardarProcedencia(ctx, file.ID, 2, "sytue-bases", provenance, "artificial"); err != nil {
		t.Fatal(err)
	}
	state, err := st.BeginPrivateStorageMigration(ctx, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	vector := struct {
		State  storage.LegacyMigrationState `json:"state"`
		Rows   []storage.LegacySourceRow    `json:"rows"`
		Blobs  map[string][]byte            `json:"blobs"`
		Root   string                       `json:"root"`
		File   string                       `json:"file"`
		Folder string                       `json:"folder"`
		Trash  string                       `json:"trash"`
	}{State: state, Blobs: map[string][]byte{}, Root: root.ID.String(), File: file.ID.String(), Folder: folder.ID.String(), Trash: trashed.ID.String()}
	cursor := storage.LegacySourceCursor{}
	for {
		page, err := st.PrivateStorageSourcePage(ctx, uuid.MustParse(state.Epoch), cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		if page.Validate(state.Scope, state.Epoch) != nil {
			t.Fatal("invalid source page")
		}
		vector.Rows = append(vector.Rows, page.Rows...)
		for _, row := range page.Rows {
			if row.Kind != "file" && row.Kind != "version" {
				continue
			}
			source, err := st.PrivateStorageSourceBlob(ctx, uuid.MustParse(state.Epoch), row.Kind, row.ID)
			if err != nil {
				t.Fatal(err)
			}
			data, ok := bodies[source.StorageKey]
			if !ok || int64(len(data)) != source.SizeBytes {
				t.Fatal("source routing lost retained body")
			}
			vector.Blobs[row.Kind+"/"+row.ID] = data
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	if len(vector.Rows) != 9 || len(vector.Blobs) != 3 {
		t.Fatal("incomplete synthetic inventory", len(vector.Rows), len(vector.Blobs))
	}
	if _, err := st.GetFile(ctx, file.ID); err != nil {
		t.Fatal("original removed", err)
	}
	raw, err := json.Marshal(vector)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateStorageNewAccountCapturesPersonalRootBeforeFreeze(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := store.WithOwner(tenantCtx("acme"), "alice")
	state, err := st.BeginPrivateStorageMigration(ctx, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	rows := privateRows(ctx, t, st, uuid.MustParse(state.Epoch), 50)
	if state.Rows != 1 || state.Blobs != 0 || len(rows) != 1 || rows[0].Kind != "folder" {
		t.Fatal("new account froze without its root", state, rows)
	}
	var root domain.Folder
	if json.Unmarshal(rows[0].Payload, &root) != nil || !root.IsRoot || root.OwnerSub != "alice" {
		t.Fatal("invalid personal root", string(rows[0].Payload))
	}
}
