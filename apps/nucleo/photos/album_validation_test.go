// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// This fixture supplies the authenticated catalog boundary only; full byte,
// scope and retained-file authentication have separate SDK witnesses.
type albumValidationCatalog struct {
	albumCatalogFixture
	entries     []records.Entry
	rows        map[string][]byte
	blobs       map[string]records.SnapshotRow
	memberReads int
}

func (f *albumValidationCatalog) Entries() ([]records.Entry, error) { return f.entries, nil }
func (f *albumValidationCatalog) ReadJSON(ctx context.Context, entry records.Entry) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if entry.Kind == "album_asset" {
		f.memberReads++
	}
	raw, ok := f.rows[entry.Kind+"/"+entry.ID]
	if !ok {
		return nil, ErrSource
	}
	return bytes.Clone(raw), nil
}

func (f *albumValidationCatalog) BinaryInfo(_ context.Context, entry records.Entry) (records.SnapshotRow, error) {
	row, ok := f.blobs[entry.ID]
	if !ok {
		return row, ErrSource
	}
	return row, nil
}

func TestAlbumValidationReusePinsScopeRevisionAndHashAndNeverReplacesSnapshotJSON(t *testing.T) {
	r := &Repository{scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}}
	f := &albumValidationCatalog{rows: map[string][]byte{}, blobs: map[string]records.SnapshotRow{}}
	asset, album := uuid.NewString(), uuid.NewString()
	add := func(kind, id string, value any) {
		raw, _ := json.Marshal(value)
		f.rows[kind+"/"+id] = raw
		f.entries = append(f.entries, records.Entry{Kind: kind, ID: id, Version: storage.ObjectVersion{Reference: storage.Reference{RoomID: "!photos:test", EventID: "$" + uuid.NewString()}, RevisionID: "original", Commit: storage.ObjectCommit{Metadata: []byte(`{"sha256":"` + planBytesHash(raw) + `"}`)}}})
	}
	add("asset", asset, model.AssetRow{ID: asset, Tenant: r.scope.Tenant, Owner: r.scope.Subject, DriveID: "own", Name: "original.png", MediaType: "image/png", Bytes: 1, Width: 1, Height: 1, UploadedAt: time.Now().UTC()})
	add("album", album, model.AlbumRow{ID: album, Tenant: r.scope.Tenant, Owner: r.scope.Subject, Title: "private"})
	memberID, _ := model.MembershipID(album, asset)
	member := []byte(`{"tenant":"acme","album_id":"` + album + `","asset_id":"` + asset + `","position":0,"added_at":"2026-10-31T00:00:00Z","future_integer":900719925474099312345}`)
	var memberValue json.RawMessage = member
	add("album_asset", memberID, memberValue)
	media := model.MediaRow{ID: asset, Tenant: r.scope.Tenant, Owner: r.scope.Subject, AssetID: asset, Role: "original", Size: 1, SHA256: planBytesHash([]byte("o")), ContentType: "image/png"}
	add("media", asset, media)
	f.entries = append(f.entries, records.Entry{Kind: "original", ID: asset, Binary: true})
	f.blobs[asset] = media.Descriptor()
	if _, err := r.readSnapshot(t.Context(), f, true); err != nil {
		t.Fatal(err)
	}
	if f.memberReads != 1 {
		t.Fatal("cold validation did not read original relationship")
	}
	if _, err := r.readSnapshot(t.Context(), f, true); err != nil || f.memberReads != 1 {
		t.Fatal("unchanged validation reloaded unknown relationship data", f.memberReads, err)
	}
	for i := range f.entries {
		if f.entries[i].Kind == "album_asset" {
			f.entries[i].Version.RevisionID = "new_revision"
		}
	}
	if _, err := r.readSnapshot(t.Context(), f, true); err != nil || f.memberReads != 2 {
		t.Fatal("same-reference revision change reused stale projection", f.memberReads, err)
	}
	for i := range f.entries {
		if f.entries[i].Kind == "album_asset" {
			f.entries[i].Version.Commit.Metadata = []byte(`{"sha256":"` + planBytesHash([]byte("different commitment")) + `"}`)
		}
	}
	if _, err := r.readSnapshot(t.Context(), f, true); err != nil || f.memberReads != 3 {
		t.Fatal("same-reference hash change reused stale projection", f.memberReads, err)
	}
	snapshot, err := r.readSnapshot(t.Context(), f, false)
	if err != nil || f.memberReads != 4 {
		t.Fatal("Snapshot replaced a data read by a projection hit", f.memberReads, err)
	}
	found := false
	for _, row := range snapshot.Rows() {
		if row.Kind == "album_asset" {
			found = bytes.Contains(row.JSON, []byte("900719925474099312345"))
			clear(row.JSON)
		}
	}
	if !found {
		t.Fatal("Snapshot dropped unknown relationship JSON")
	}
	fresh := &Repository{scope: r.scope}
	if _, err := fresh.readSnapshot(t.Context(), f, true); err != nil || f.memberReads != 5 {
		t.Fatal("new repository reused another instance's validation", f.memberReads, err)
	}
	r.scope.Subject = "other"
	if _, err := r.readSnapshot(t.Context(), f, true); err == nil || f.memberReads != 6 {
		t.Fatal("validation cache crossed its account", f.memberReads, err)
	}
}
