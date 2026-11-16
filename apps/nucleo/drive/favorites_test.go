// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateFavoritesRetainOrderingPathsAndInheritedTrash(t *testing.T) {
	r := queryFixture(t, 2)
	now := time.Now().UTC()
	root, folder := "root", "folder"
	addQueryRow(t, r, "folder", folder, api.Folder{ID: folder, ParentID: &root, Name: "Folder", UpdatedAt: now})
	addQueryRow(t, r, "file", "file_001", api.File{ID: "file_001", FolderID: folder, Name: "File.txt", UpdatedAt: now, Version: 1, Label: &api.DriveLabel{Name: "Private", BlockExternalShare: true}})
	for _, row := range []favoriteRow{
		{ID: "mark_folder", Subject: "owner", FolderID: folder, CreatedAt: now.Add(-time.Hour)},
		{ID: "mark_file1", Subject: "owner", FileID: "file_001", CreatedAt: now},
		{ID: "mark_file2", Subject: "owner", FileID: "file_002", CreatedAt: now.Add(-time.Minute)},
		{ID: "mark_foreign", Subject: "owner", FileID: "absent_foreign_target", CreatedAt: now},
	} {
		addQueryRow(t, r, "favorite", row.ID, row)
	}
	page, err := r.Favorites(t.Context())
	if err != nil || len(page) != 3 || page[0].ID != folder || !page[0].IsDir || page[1].ID != "file_001" || page[2].ID != "file_002" || page[1].Path != "/Folder/File.txt" || page[1].Label == nil || !page[1].Label.BlockExternalShare {
		t.Fatal("private favorites lost ordering, paths or sensitivity", page, err)
	}
	addQueryRow(t, r, "folder", folder, api.Folder{ID: folder, ParentID: &root, Name: "Folder", UpdatedAt: now, DeletedAt: &now})
	page, err = r.Favorites(t.Context())
	if err != nil || len(page) != 1 || page[0].ID != "file_002" {
		t.Fatal("private favorites exposed inherited trash", page, err)
	}
	addQueryRow(t, r, "folder", folder, api.Folder{ID: folder, ParentID: &root, Name: "Folder", UpdatedAt: now})
	addQueryRow(t, r, "favorite", "mark_file2", favoriteRow{ID: "mark_file2", Subject: "owner", FileID: "file_002", CreatedAt: now, DeletedAt: &now})
	page, err = r.Favorites(t.Context())
	if err != nil || len(page) != 2 || page[0].ID != folder || page[1].ID != "file_001" {
		t.Fatal("restored favorites lost marks or resurrected removed star", page, err)
	}
}

func TestPrivateFavoritesRejectForeignSubjectDuplicateTargetAndConflictingBranches(t *testing.T) {
	for _, scenario := range []string{"subject", "duplicate", "branches", "two_targets", "identity"} {
		t.Run(scenario, func(t *testing.T) {
			r := queryFixture(t, 1)
			row := favoriteRow{ID: "mark", Subject: "owner", FileID: "file_001", CreatedAt: time.Now().UTC()}
			want := ErrSource
			switch scenario {
			case "subject":
				row.Subject = "other"
			case "two_targets":
				row.FolderID = "root"
			case "identity":
				row.ID = "another"
			}
			addQueryRow(t, r, "favorite", "mark", row)
			if scenario == "duplicate" {
				row.ID = "duplicate"
				addQueryRow(t, r, "favorite", row.ID, row)
				want = ErrConflict
			}
			if scenario == "branches" {
				key := objectID("favorite", "mark")
				r.heads[key] = append(r.heads[key], r.heads[key][0])
				want = ErrConflict
			}
			if _, err := r.Favorites(t.Context()); !errors.Is(err, want) {
				t.Fatal("invalid favorite accepted", scenario, err)
			}
		})
	}
}

func TestFavoriteIntentRejectsChangedTargetScopeOrParentsBeforeSDKAccess(t *testing.T) {
	for _, scenario := range []string{"target", "subject", "scope", "parents", "mixed_intents"} {
		t.Run(scenario, func(t *testing.T) {
			scope := storage.Scope{Tenant: "tenant", Subject: "owner", Product: "drive"}
			request := favoriteRequest{Kind: api.DriveFile, ID: "file", On: true}
			row := favoriteRow{ID: "mark", Subject: scope.Subject, FileID: request.ID, CreatedAt: time.Now().UTC()}
			if scenario == "target" {
				row.FileID = "other"
			}
			if scenario == "subject" {
				row.Subject = "other"
			}
			raw, _ := json.Marshal(row)
			item, err := summarize(storage.LegacySourceRow{Kind: "favorite", ID: row.ID, Payload: raw}, "")
			if err != nil {
				t.Fatal(err)
			}
			input, _ := json.Marshal(request)
			hash := digest(input)
			work := rowWork{V: 1, Scope: scope, SourceKind: "favorite", SourceID: row.ID, RawSHA256: item.RawSHA256, Revision: uuid.NewString(), Bodies: map[string]*fileWork{}, ParentsSet: true, Favorite: &favoriteIntent{InputSHA256: hash, Item: item, Raw: raw}}
			switch scenario {
			case "scope":
				work.Scope.Subject = "other"
			case "parents":
				work.Parents = []storage.Reference{{RoomID: "!foreign:test", EventID: "$foreign"}}
			case "mixed_intents":
				work.Creation = &creationIntent{}
			}
			// No SDK is supplied: any call crossing this boundary would panic.
			r := &Repository{cfg: Config{Scope: scope}}
			if err := r.checkFavoriteIntent(t.Context(), work, request, hash); !errors.Is(err, ErrSource) {
				t.Fatal("changed private favorite intent reached SDK", scenario, err)
			}
		})
	}
}
