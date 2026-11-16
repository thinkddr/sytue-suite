// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	photoID = "97e57701-8223-42cf-ae26-e4d3ec4a235c"
	albumID = "a37c70b7-9543-46c7-8118-a99ce2be512d"
	editID  = "992f1cf3-f362-44bb-b2c7-e5bcaef60525"
)

func photoScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
}

func retainedRows(t *testing.T) []Row {
	t.Helper()
	membership, err := MembershipID(albumID, photoID)
	if err != nil {
		t.Fatal(err)
	}
	return []Row{
		{Kind: "asset", ID: photoID, JSON: []byte(`{"id":"` + photoID + `","tenant":"acme","owner_sub":"alice","drive_file_id":"original-drive-id","name":"original.heic","media_type":"image/heic","bytes":9007199254740993,"width":4032,"height":3024,"orientation":6,"uploaded_at":"2022-06-01T10:00:00Z","taken_at":"2021-01-02T12:34:56Z","favorite":true,"deleted_at":"2026-01-01T12:00:00Z","embedding":[0.6,0.8],"embedding_model":"retained-model","future_integer":900719925474099312345}`)},
		{Kind: "album", ID: albumID, JSON: []byte(`{"id":"` + albumID + `","tenant":"acme","owner_sub":"alice","title":"Álbum intacto","cover_asset_id":"` + photoID + `","public_token":"","public_token_hash":""}`)},
		{Kind: "album_asset", ID: membership, JSON: []byte(`{"tenant":"acme","album_id":"` + albumID + `","asset_id":"` + photoID + `","position":-3,"added_at":"2022-06-02T10:00:00Z"}`)},
	}
}

func TestSnapshotRetainsExactMetadataTrashOriginalsAndIndependentCopies(t *testing.T) {
	rows := retainedRows(t)
	want := bytes.Clone(rows[0].JSON)
	snapshot, err := NewSnapshot(photoScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.PersonalCompatible(); err != nil {
		t.Fatal(err)
	}
	rows[0].JSON[0] = '!'
	var asset Row
	for _, row := range snapshot.Rows() {
		if row.Kind == "asset" {
			asset = row
		}
	}
	if !bytes.Equal(asset.JSON, want) {
		t.Fatal("retained asset bytes changed")
	}
	asset.JSON[0] = '!'
	for _, row := range snapshot.Rows() {
		if row.Kind == "asset" && !bytes.Equal(row.JSON, want) {
			t.Fatal("view aliases retained JSON")
		}
	}
	originals := snapshot.Originals()
	if len(originals) != 1 || originals[0].DriveID != "original-drive-id" || originals[0].Size != 9007199254740993 {
		t.Fatal("trash or original relationship lost", originals)
	}
	var stored AssetRow
	if err := json.Unmarshal(want, &stored); err != nil {
		t.Fatal(err)
	}
	view := stored.Activo()
	w, h := view.Vistas()
	if w != 3024 || h != 4032 {
		t.Fatal("EXIF orientation no longer shared", w, h)
	}
	when, captured := view.FechaDeGaleria()
	if !captured || when.Year() != 2021 {
		t.Fatal("capture date replaced by upload date")
	}
}

func TestSnapshotRejectsIncompleteOrReboundGraph(t *testing.T) {
	tests := map[string]func([]Row) []Row{
		"missing asset": func(rows []Row) []Row { return rows[1:] },
		"missing album": func(rows []Row) []Row { return append(rows[:1], rows[2:]...) },
		"duplicate row": func(rows []Row) []Row { return append(rows, rows[0]) },
		"changed tenant": func(rows []Row) []Row {
			rows[0].JSON = bytes.ReplaceAll(rows[0].JSON, []byte(`"acme"`), []byte(`"other"`))
			return rows
		},
		"rebound membership": func(rows []Row) []Row { rows[2].ID = "wrong"; return rows },
		"unknown table":      func(rows []Row) []Row { rows[2].Kind = "future_table"; return rows },
		"partial GPS": func(rows []Row) []Row {
			rows[0].JSON = bytes.Replace(rows[0].JSON, []byte(`"width":`), []byte(`"latitude":3,"width":`), 1)
			return rows
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := NewSnapshot(photoScope(), change(retainedRows(t))); !errors.Is(err, ErrSnapshot) {
				t.Fatal("incomplete graph accepted", err)
			}
		})
	}
}

func TestSnapshotRetainsSharesAndRejectsPersonalConversion(t *testing.T) {
	for _, share := range []string{"public", "person", "other owner"} {
		t.Run(share, func(t *testing.T) {
			rows := retainedRows(t)
			switch share {
			case "public":
				rows[1].JSON = bytes.Replace(rows[1].JSON, []byte(`"public_token_hash":""`), []byte(`"public_token_hash":"retained-hash"`), 1)
			case "person":
				id, err := AlbumPersonID(albumID, "bob:original/é")
				if err != nil {
					t.Fatal(err)
				}
				rows = append(rows, Row{Kind: "album_person", ID: id, JSON: []byte(`{"tenant":"acme","album_id":"` + albumID + `","sujeto":"bob:original/é","concedido_por":"alice","puede_editar":true}`)})
			case "other owner":
				rows[1].JSON = bytes.Replace(rows[1].JSON, []byte(`"alice"`), []byte(`"bob"`), 1)
			}
			snapshot, err := NewSnapshot(photoScope(), rows)
			if err != nil {
				t.Fatal("share must be retained for migration review", err)
			}
			if len(snapshot.Rows()) != len(rows) {
				t.Fatal("ACL row discarded")
			}
			if err := snapshot.PersonalCompatible(); !errors.Is(err, ErrShared) {
				t.Fatal("shared album accepted as personal", err)
			}
		})
	}
}

func TestSnapshotChecksRecipeFingerprintAndOriginalIdentity(t *testing.T) {
	rows := retainedRows(t)
	recipe := Receta{Version: VersionReceta, Pasos: []Paso{{Tipo: PasoGiro, Params: map[string]float64{"grados": 90}}}}
	fingerprint, err := recipe.Huella()
	if err != nil {
		t.Fatal(err)
	}
	edit := EditRow{ID: editID, Tenant: "acme", Owner: "alice", AssetID: photoID, Recipe: recipe, Fingerprint: fingerprint}
	raw, err := json.Marshal(edit)
	if err != nil {
		t.Fatal(err)
	}
	rows = append(rows, Row{Kind: "edit", ID: editID, JSON: raw})
	if _, err := NewSnapshot(photoScope(), rows); err != nil {
		t.Fatal(err)
	}
	edit.Fingerprint = "wrong"
	rows[len(rows)-1].JSON, _ = json.Marshal(edit)
	if _, err := NewSnapshot(photoScope(), rows); !errors.Is(err, ErrSnapshot) {
		t.Fatal("different recipe result accepted", err)
	}
}
