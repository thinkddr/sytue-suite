// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// The catalog boundary is synthetic here. Maintained SDK authentication and
// fresh-device recovery are exercised by the native integration witness.
type albumCatalogFixture struct {
	bodies map[storage.Reference][]byte
	reads  int
}

func (*albumCatalogFixture) Entries() ([]records.Entry, error) { return nil, nil }
func (*albumCatalogFixture) ReadJSON(context.Context, records.Entry) ([]byte, error) {
	return nil, ErrSource
}

type membershipIndexCatalog struct {
	albumCatalogFixture
	entries []records.Entry
	rows    map[string][]byte
	reads   int
}

func (f *membershipIndexCatalog) Entries() ([]records.Entry, error) { return f.entries, nil }
func (f *membershipIndexCatalog) ReadJSON(_ context.Context, entry records.Entry) ([]byte, error) {
	f.reads++
	raw, ok := f.rows[entry.ID]
	if !ok {
		return nil, ErrSource
	}
	return append([]byte(nil), raw...), nil
}

func TestAlbumMembershipIndexPinsImmutableVersionsWithoutReloadingUnrelatedBodies(t *testing.T) {
	r := &Repository{scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}}
	firstAlbum, otherAlbum := uuid.NewString(), uuid.NewString()
	catalog := &membershipIndexCatalog{rows: map[string][]byte{}}
	rows := make([]model.Row, 0, 3)
	for i := range 3 {
		album := firstAlbum
		if i > 0 {
			album = otherAlbum
		}
		asset := uuid.NewString()
		id, _ := model.MembershipID(album, asset)
		raw, _ := json.Marshal(model.MembershipRow{Tenant: r.scope.Tenant, AlbumID: album, AssetID: asset, Position: i})
		catalog.rows[id] = raw
		catalog.entries = append(catalog.entries, records.Entry{Kind: "album_asset", ID: id, Version: storage.ObjectVersion{Reference: storage.Reference{RoomID: "!photos:test", EventID: "$" + uuid.NewString()}}})
		rows = append(rows, model.Row{Kind: "album_asset", ID: id, JSON: raw})
	}
	if err := r.rememberMemberships(catalog.entries, rows); err != nil {
		t.Fatal(err)
	}
	if members, err := r.indexedMemberships(t.Context(), catalog); err != nil || len(members) != 3 || catalog.reads != 0 {
		t.Fatal("validated projection reloaded historical bodies", catalog.reads, err)
	}
	changed := catalog.entries[1].ID
	catalog.entries[1].Version.Reference.EventID = "$updated"
	var member model.MembershipRow
	_ = json.Unmarshal(catalog.rows[changed], &member)
	member.Position = 99
	catalog.rows[changed], _ = json.Marshal(member)
	members, err := r.indexedMemberships(t.Context(), catalog)
	if err != nil || catalog.reads != 1 || members[changed].Row.Position != 99 {
		t.Fatal("cached projection masked a changed authenticated revision", catalog.reads, err)
	}
	catalog.entries[1].Version.RevisionID = "changed_revision_with_same_reference"
	member.Position = 100
	catalog.rows[changed], _ = json.Marshal(member)
	members, err = r.indexedMemberships(t.Context(), catalog)
	if err != nil || catalog.reads != 2 || members[changed].Row.Position != 100 {
		t.Fatal("cache did not pin revision and hash as well as reference", catalog.reads, err)
	}
}

func (f *albumCatalogFixture) ValidateRetainedFile(ctx context.Context, file records.RetainedFile) error {
	body, ok := f.bodies[file.Link]
	if !ok || int64(len(body)) != file.Row.Size || planBytesHash(body) != file.Row.SHA256 {
		return ErrSource
	}
	return ctx.Err()
}

func (f *albumCatalogFixture) ReadRetainedFile(ctx context.Context, file records.RetainedFile, out io.Writer) error {
	f.reads++
	if err := f.ValidateRetainedFile(ctx, file); err != nil {
		return err
	}
	_, err := out.Write(f.bodies[file.Link])
	return err
}

func TestAlbumCheckpointProjectionDoesNotRedownloadBodiesOrMaskNewRoot(t *testing.T) {
	r, catalog, album, set, manifest := albumCheckpointFixture(t)
	assets := map[string]bool{}
	for _, ref := range manifest.Pages {
		var page albumMemberPage
		_ = json.Unmarshal(catalog.bodies[ref.File.Link], &page)
		var member model.MembershipRow
		_ = json.Unmarshal(page.Members[0], &member)
		assets[member.AssetID] = true
	}
	encoded, _ := json.Marshal(set)
	row := model.AlbumRow{ID: album, Tenant: r.scope.Tenant, Owner: r.scope.Subject, Title: "private", PrivateMembers: encoded}
	raw, _ := json.Marshal(row)
	if err := r.validateAlbumMemberships(t.Context(), catalog, raw, assets); err != nil {
		t.Fatal(err)
	}
	reads := catalog.reads
	members, err := r.checkpointMembers(t.Context(), catalog, row)
	if err != nil || len(members) != 3 || catalog.reads != reads {
		t.Fatal("ordinary metadata read downloaded checkpoint bodies", catalog.reads, reads, err)
	}
	members[0].AssetID = "changed by caller"
	members, err = r.checkpointMembers(t.Context(), catalog, row)
	if err != nil || members[0].AssetID == "changed by caller" {
		t.Fatal("metadata projection exposed its owned buffer", err)
	}
	manifest.Pages = manifest.Pages[:2]
	root, _ := json.Marshal(manifest)
	set.Root.Link.EventID = "$new_root"
	set.Root.Row.ID = planBytesHash(root)
	set.Root.Row.SHA256 = planBytesHash(root)
	set.Root.Row.Size = int64(len(root))
	catalog.bodies[set.Root.Link] = root
	row.PrivateMembers, _ = json.Marshal(set)
	members, err = r.checkpointMembers(t.Context(), catalog, row)
	if err != nil || len(members) != 2 || catalog.reads <= reads {
		t.Fatal("cached membership replaced a new authenticated root", len(members), err)
	}
	for i := range 4 {
		set.Root.Link.EventID = fmt.Sprintf("$next_root_%d", i)
		catalog.bodies[set.Root.Link] = root
		row.PrivateMembers, _ = json.Marshal(set)
		raw, _ = json.Marshal(row)
		if err := r.validateAlbumMemberships(t.Context(), catalog, raw, assets); err != nil {
			t.Fatal(err)
		}
		if len(r.albumCheckpoints) != 1 || r.albumCheckpoints[album].Set != set {
			t.Fatal("old checkpoint projections accumulated")
		}
	}
}

func albumCheckpointFixture(t *testing.T) (*Repository, *albumCatalogFixture, string, albumMemberSet, albumManifest) {
	t.Helper()
	r := &Repository{scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}}
	catalog := &albumCatalogFixture{bodies: map[storage.Reference][]byte{}}
	target := storage.PublicationTarget{RoomID: "!photos:test", DeviceID: "DEVICE"}
	file := func(kind string, value any) records.RetainedFile {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		ref := storage.Reference{RoomID: target.RoomID, EventID: "$" + uuid.NewString()}
		catalog.bodies[ref] = raw
		return records.RetainedFile{Version: 1, Scope: r.scope, Target: target, Row: records.SnapshotRow{Kind: kind, ID: planBytesHash(raw), Size: int64(len(raw)), SHA256: planBytesHash(raw)}, Revision: uuid.NewString(), Link: ref}
	}
	album := uuid.NewString()
	manifest := albumManifest{Version: 1, Scope: r.scope, Album: album, Pages: []albumPageReference{}}
	for range 3 {
		member, _ := json.Marshal(model.MembershipRow{Tenant: r.scope.Tenant, AlbumID: album, AssetID: uuid.NewString()})
		page := albumMemberPage{Version: 1, Scope: r.scope, Album: album, Members: []json.RawMessage{member}}
		manifest.Pages = append(manifest.Pages, albumPageReference{File: file(albumPageKind, page), Count: 1})
	}
	return r, catalog, album, albumMemberSet{Version: 1, Root: file(albumIndexKind, manifest)}, manifest
}

func TestAlbumCheckpointRejectsRepeatedPagesAndForeignRelationships(t *testing.T) {
	for _, change := range []string{"valid", "cycle", "duplicate page", "scope", "album", "count", "duplicate asset", "foreign relation", "suffix"} {
		t.Run(change, func(t *testing.T) {
			r, catalog, album, set, manifest := albumCheckpointFixture(t)
			switch change {
			case "cycle":
				manifest.Pages[0].File = set.Root
			case "duplicate page":
				manifest.Pages[1] = manifest.Pages[0]
			case "scope":
				manifest.Scope.Subject = "other"
			case "album":
				manifest.Album = uuid.NewString()
			case "count":
				manifest.Pages[0].Count++
			case "duplicate asset", "foreign relation":
				var first, second albumMemberPage
				_ = json.Unmarshal(catalog.bodies[manifest.Pages[0].File.Link], &first)
				_ = json.Unmarshal(catalog.bodies[manifest.Pages[1].File.Link], &second)
				if change == "duplicate asset" {
					second.Members = first.Members
				} else {
					var member model.MembershipRow
					_ = json.Unmarshal(second.Members[0], &member)
					member.AlbumID = uuid.NewString()
					second.Members[0], _ = json.Marshal(member)
				}
				body, _ := json.Marshal(second)
				ref := manifest.Pages[1].File.Link
				catalog.bodies[ref] = body
				manifest.Pages[1].File.Row.Size = int64(len(body))
				manifest.Pages[1].File.Row.SHA256 = planBytesHash(body)
			}
			root, _ := json.Marshal(manifest)
			if change == "suffix" {
				root = append(root, []byte(` {"second":true}`)...)
			}
			catalog.bodies[set.Root.Link] = root
			set.Root.Row.Size = int64(len(root))
			set.Root.Row.SHA256 = planBytesHash(root)
			pages, err := r.readAlbumPages(t.Context(), catalog, album, set)
			defer func() {
				for _, page := range pages {
					clearAlbumPage(page)
				}
			}()
			if change == "valid" {
				if err != nil || len(pages) != 3 {
					t.Fatal("direct paged checkpoint did not open", err)
				}
			} else if err == nil {
				t.Fatal("invalid checkpoint became visible", change)
			}
		})
	}
}
