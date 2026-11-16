// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

type albumGraph struct {
	albums  []model.AlbumRow
	assets  map[string]model.AssetRow
	members map[string][]model.MembershipRow
}

// Personal albums retain the original relation rows. Deleting an album writes
// a tombstone to that album only; it never deletes photos, their history or the
// retained source graph needed by another authorized device.
func (r *Repository) albumGraph(ctx context.Context) (albumGraph, error) {
	graph := albumGraph{assets: map[string]model.AssetRow{}, members: map[string][]model.MembershipRow{}}
	assets, err := r.assets(ctx)
	if err != nil {
		return graph, err
	}
	for _, asset := range assets {
		graph.assets[asset.ID] = asset
	}
	versions, err := r.engine.Selected("album")
	if err != nil {
		return graph, err
	}
	for _, version := range versions {
		raw, err := r.engine.ReadJSON(ctx, version)
		if err != nil {
			return graph, err
		}
		var row model.AlbumRow
		err = json.Unmarshal(raw, &row)
		clear(raw)
		if err != nil || row.Owner != r.scope.Subject || row.Tenant != r.scope.Tenant {
			return graph, ErrSource
		}
		graph.albums = append(graph.albums, row)
	}
	catalog, err := r.engine.Catalog()
	if err != nil {
		return graph, err
	}
	memberships, err := r.indexedMemberships(ctx, catalog)
	if err != nil {
		return graph, err
	}
	for _, member := range memberships {
		graph.members[member.Row.AlbumID] = append(graph.members[member.Row.AlbumID], member.Row)
	}
	for id, members := range graph.members {
		slices.SortFunc(members, func(a, b model.MembershipRow) int {
			if a.Position < b.Position {
				return -1
			}
			if a.Position > b.Position {
				return 1
			}
			return strings.Compare(a.AssetID, b.AssetID)
		})
		graph.members[id] = members
	}
	if err := r.applyAlbumCheckpoints(ctx, &graph); err != nil {
		return graph, err
	}
	return graph, nil
}

func (r *Repository) applyAlbumCheckpoints(ctx context.Context, graph *albumGraph) error {
	catalog, err := r.engine.Catalog()
	if err != nil {
		return err
	}
	for _, album := range graph.albums {
		if len(album.PrivateMembers) == 0 {
			continue
		}
		members, err := r.checkpointMembers(ctx, catalog, album)
		if err != nil {
			return err
		}
		graph.members[album.ID] = members
	}
	return nil
}

func (g albumGraph) photos(id string) []api.Photo {
	photos := []api.Photo{}
	for _, member := range g.members[id] {
		if asset, ok := g.assets[member.AssetID]; ok && asset.DeletedAt == nil {
			photos = append(photos, photoView(asset))
		}
	}
	return photos
}

func (g albumGraph) view(row model.AlbumRow) api.Album {
	photos := g.photos(row.ID)
	view := api.Album{ID: row.ID, Title: row.Title, Count: len(photos)}
	if len(photos) > 0 {
		view.CoverAssetID = photos[0].ID
	}
	if row.CoverAssetID != nil {
		for _, photo := range photos {
			if photo.ID == *row.CoverAssetID {
				view.CoverAssetID = photo.ID
				break
			}
		}
	}
	return view
}

func (r *Repository) Albums(parent context.Context) ([]api.Album, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	graph, err := r.albumGraph(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(graph.albums, func(a, b model.AlbumRow) int {
		if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
			return order
		}
		return strings.Compare(b.ID, a.ID)
	})
	values := []api.Album{}
	for _, row := range graph.albums {
		if row.DeletedAt == nil {
			values = append(values, graph.view(row))
		}
	}
	return values, nil
}

func (r *Repository) Album(parent context.Context, id string) (api.Album, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return api.Album{}, err
	}
	defer done()
	graph, err := r.albumGraph(ctx)
	if err != nil {
		return api.Album{}, err
	}
	for _, row := range graph.albums {
		if row.ID == id && row.DeletedAt == nil {
			return graph.view(row), nil
		}
	}
	return api.Album{}, ErrNotFound
}

func (r *Repository) AlbumPhotos(parent context.Context, id string) ([]api.Photo, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	graph, err := r.albumGraph(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range graph.albums {
		if row.ID == id && row.DeletedAt == nil {
			return graph.photos(id), nil
		}
	}
	return nil, ErrNotFound
}

func (r *Repository) CreateAlbum(ctx context.Context, title string) (api.Album, error) {
	title, err := model.TituloDeAlbum(title)
	if err != nil {
		return api.Album{}, err
	}
	result, err := r.engine.Mutate(ctx, "album-create-v1", title, func(context.Context) (string, string, string, []byte, error) {
		now := time.Now().UTC()
		row := model.AlbumRow{ID: uuid.NewString(), Tenant: r.scope.Tenant, Owner: r.scope.Subject, Title: title, CreatedAt: now, UpdatedAt: now}
		raw, err := json.Marshal(row)
		return "album", row.ID, "live", raw, err
	})
	defer clear(result)
	if err != nil {
		return api.Album{}, err
	}
	var row model.AlbumRow
	if json.Unmarshal(result, &row) != nil {
		return api.Album{}, ErrSource
	}
	return api.Album{ID: row.ID, Title: row.Title}, nil
}

func (r *Repository) mutateAlbum(ctx context.Context, operation, id string, input any, patch func(map[string]json.RawMessage)) error {
	result, err := r.engine.Mutate(ctx, operation, input, func(ctx context.Context) (string, string, string, []byte, error) {
		raw, err := r.engine.ReadRaw(ctx, "album", id)
		defer clear(raw)
		if err != nil {
			return "", "", "", nil, err
		}
		var row model.AlbumRow
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &row) != nil || json.Unmarshal(raw, &fields) != nil || row.ID != id || row.Tenant != r.scope.Tenant || row.Owner != r.scope.Subject {
			return "", "", "", nil, ErrSource
		}
		if row.DeletedAt != nil {
			return "", "", "", nil, ErrNotFound
		}
		patch(fields)
		fields["updated_at"], _ = json.Marshal(time.Now().UTC())
		result, err := json.Marshal(fields)
		state := "live"
		if value := fields["deleted_at"]; len(value) > 0 && string(value) != "null" {
			state = "trash"
		}
		return "album", id, state, result, err
	})
	clear(result)
	return err
}

func (r *Repository) RenameAlbum(ctx context.Context, id, title string) error {
	title, err := model.TituloDeAlbum(title)
	if err != nil {
		return err
	}
	return r.mutateAlbum(ctx, "album-rename-v1", id, []string{id, title}, func(fields map[string]json.RawMessage) { fields["title"], _ = json.Marshal(title) })
}

func (r *Repository) DeleteAlbum(ctx context.Context, id string) error {
	return r.mutateAlbum(ctx, "album-delete-v1", id, id, func(fields map[string]json.RawMessage) { fields["deleted_at"], _ = json.Marshal(time.Now().UTC()) })
}

var _ api.PhotosAlbumData = (*Repository)(nil)
