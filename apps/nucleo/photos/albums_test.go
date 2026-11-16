// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

func TestAlbumViewExcludesTrashedCoverWithoutChangingRetainedOrder(t *testing.T) {
	cover, fallback := "cover", "fallback"
	now := time.Now().UTC()
	graph := albumGraph{
		assets: map[string]model.AssetRow{
			cover:    {ID: cover, DeletedAt: &now},
			fallback: {ID: fallback},
		},
		members: map[string][]model.MembershipRow{"album": {{AssetID: cover}, {AssetID: fallback}}},
	}
	row := model.AlbumRow{ID: "album", Title: "private", CoverAssetID: &cover}
	view := graph.view(row)
	if view.Count != 1 || view.CoverAssetID != fallback {
		t.Fatal("trashed cover remained visible", view)
	}
	graph.assets[cover] = model.AssetRow{ID: cover}
	view = graph.view(row)
	if view.Count != 2 || view.CoverAssetID != cover || graph.photos("album")[0].ID != cover {
		t.Fatal("restoring a photo lost retained order or explicit cover", view)
	}
	graph.assets[fallback] = model.AssetRow{ID: fallback, DeletedAt: &now}
	graph.assets[cover] = model.AssetRow{ID: cover, DeletedAt: &now}
	view = graph.view(row)
	if view.Count != 0 || view.CoverAssetID != "" || len(graph.photos("album")) != 0 {
		t.Fatal("empty active album leaked a trashed photo", view)
	}
}
