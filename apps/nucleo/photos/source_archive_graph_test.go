// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

func TestSourceGraphStagesOneExactEntityAtATimeAndChecksRelationships(t *testing.T) {
	store, _, _ := sourceArchiveCacheFixture(t)
	base, snapshot, _ := sourcePlanFixture(t)
	asset := snapshot.Rows()[0]
	cover := asset.ID
	album := model.AlbumRow{ID: uuid.NewString(), Tenant: base.Scope.Tenant, Owner: base.Scope.Subject, Title: "retained", CoverAssetID: &cover}
	raw, err := json.Marshal(album)
	if err != nil {
		t.Fatal(err)
	}
	rows := []model.Row{asset, {Kind: "album", ID: album.ID, JSON: raw}}
	control := pagedPreflightFixtureForRows(t, base.Scope, base.Epoch, rows)
	file, err := store.captureSourceGraph(t.Context(), control.info, control)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := store.openSourceGraph(t.Context(), control.info, file)
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.validate(t.Context()); err != nil {
		t.Fatal("valid graph rejected", err)
	}
	if err := graph.validate(t.Context()); err != nil {
		t.Fatal("idempotent retry changed uniqueness", err)
	}
	if control.maxActive != 1 || control.active != 0 {
		t.Fatal("metadata readers escaped one entity", control.maxActive, control.active)
	}
	descriptor, row, err := graph.lookup(t.Context(), "asset", asset.ID)
	if err != nil || descriptor != control.rows[1] || bytes.Equal(row.JSON, asset.JSON) {
		t.Fatal("graph projection replaced exact retained commitment", err)
	}
	// Missing lookup is a real relationship failure although every remaining row
	// has a valid hash, scope and standalone model.
	album.CoverAssetID = new(uuid.NewString())
	raw, err = json.Marshal(album)
	if err != nil {
		t.Fatal(err)
	}
	rows[1].JSON = raw
	missing := pagedPreflightFixtureForRows(t, base.Scope, base.Epoch, rows)
	next, err := store.captureSourceGraph(t.Context(), missing.info, missing)
	if err != nil {
		t.Fatal(err)
	}
	broken, err := store.openSourceGraph(t.Context(), missing.info, next)
	if err != nil {
		t.Fatal(err)
	}
	if err := broken.validate(t.Context()); err == nil {
		t.Fatal("missing owned cover accepted")
	}
}

func TestSourceGraphRejectsSharedDuplicateAndIncompleteSources(t *testing.T) {
	for _, change := range []string{"missing page", "changed body", "duplicate Drive", "shared album"} {
		t.Run(change, func(t *testing.T) {
			store, _, _ := sourceArchiveCacheFixture(t)
			base, snapshot, _ := sourcePlanFixture(t)
			rows := snapshot.Rows()
			for range 2 {
				row := rows[0]
				id := uuid.NewString()
				row.ID = id
				row.JSON = bytes.ReplaceAll(row.JSON, []byte(rows[0].ID), []byte(id))
				if change != "duplicate Drive" {
					row.JSON = bytes.ReplaceAll(row.JSON, []byte("retained-drive-id"), []byte("retained-"+id))
				}
				rows = append(rows, row)
			}
			if change == "shared album" {
				album := model.AlbumRow{ID: uuid.NewString(), Tenant: base.Scope.Tenant, Owner: base.Scope.Subject, Title: "shared", PublicToken: "public"}
				raw, err := json.Marshal(album)
				if err != nil {
					t.Fatal(err)
				}
				rows = append(rows, model.Row{Kind: "album", ID: album.ID, JSON: raw})
			}
			control := pagedPreflightFixtureForRows(t, base.Scope, base.Epoch, rows)
			control.change = change
			file, err := store.captureSourceGraph(t.Context(), control.info, control)
			if err == nil {
				graph, openErr := store.openSourceGraph(t.Context(), control.info, file)
				err = openErr
				if err == nil {
					err = graph.validate(t.Context())
				}
			}
			if err == nil || control.active != 0 {
				t.Fatal("invalid graph accepted", change, err, control.active)
			}
			if change == "shared album" && !errors.Is(err, model.ErrShared) {
				t.Fatal("sharing classification lost", err)
			}
		})
	}
}
