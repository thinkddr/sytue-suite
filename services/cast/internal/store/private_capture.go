// SPDX-License-Identifier: Apache-2.0
package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/cast/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

const maxPlaylistBytes = 8 << 20

// Hash every exact stored body while the caller holds the exclusive owner row
// lock. Never use a size/etag as a body receipt or re-encode historical media.
func (p *PrivateSource) inspectMedia(ctx context.Context, scope storage.Scope, video string, object objectstore.ListedObject) (model.MediaRow, []byte, error) {
	id, err := model.MediaID(video, object.Key)
	if err != nil || !strings.HasPrefix(object.Key, video+"/") || object.Size < 0 || object.Size > storage.MaxFileBytes {
		return model.MediaRow{}, nil, ErrPrivateSource
	}
	playlist := path.Ext(object.Key) == ".m3u8"
	if playlist && object.Size > maxPlaylistBytes {
		return model.MediaRow{}, nil, ErrPrivateSource
	}
	stored, err := p.objects.GetRange(ctx, tenancy.Tenant{Slug: scope.Tenant}, object.Key, "")
	if err != nil {
		if stored != nil && stored.Body != nil {
			_ = stored.Body.Close()
		}
		return model.MediaRow{}, nil, err
	}
	if stored == nil || stored.Body == nil {
		return model.MediaRow{}, nil, ErrPrivateSource
	}
	reader := stored.Body
	stop := context.AfterFunc(ctx, func() { _ = reader.Close() })
	defer stop()
	digest := sha256.New()
	var data bytes.Buffer
	var output io.Writer = digest
	if playlist {
		output = io.MultiWriter(digest, &data)
	}
	buffer := make([]byte, 64<<10)
	defer clear(buffer)
	n, readErr := io.CopyBuffer(output, io.LimitReader(reader, object.Size+1), buffer)
	if err := errors.Join(readErr, reader.Close(), ctx.Err()); err != nil {
		return model.MediaRow{}, nil, err
	}
	if n != object.Size {
		return model.MediaRow{}, nil, ErrPrivateSource
	}
	if stored.Size != n || stored.Total != n || stored.ContentRange != "" {
		return model.MediaRow{}, nil, ErrPrivateSource
	}
	contentType := stored.ContentType
	if contentType == "" {
		contentType = model.TipoDe(object.Key)
	}
	media := model.MediaRow{ID: id, Tenant: scope.Tenant, Owner: scope.Subject, VideoID: video, Key: object.Key, Size: n, SHA256: hex.EncodeToString(digest.Sum(nil)), ContentType: contentType}
	return media, data.Bytes(), nil
}

func (p *PrivateSource) videoMedia(ctx context.Context, scope storage.Scope, video string, remaining int) ([]model.MediaRow, map[string][]byte, error) {
	media := []model.MediaRow{}
	playlists := map[string][]byte{}
	cursor, last := "", ""
	seen := map[string]bool{}
	var playlistBytes int
	for {
		page, err := p.objects.List(ctx, tenancy.Tenant{Slug: scope.Tenant}, video+"/", cursor, 1000)
		if err != nil {
			return nil, nil, err
		}
		if len(page.Objects) > 1000 || len(page.Objects) == 0 && page.Next != "" {
			return nil, nil, ErrPrivateSource
		}
		for _, object := range page.Objects {
			if object.Key <= last {
				return nil, nil, ErrPrivateSource
			}
			last = object.Key
			value, playlist, err := p.inspectMedia(ctx, scope, video, object)
			if err != nil {
				return nil, nil, err
			}
			media = append(media, value)
			if len(media) > remaining {
				return nil, nil, ErrPrivateSource
			}
			if path.Ext(value.Key) == ".m3u8" {
				playlists[value.ID] = playlist
				playlistBytes += len(playlist)
				if playlistBytes > model.MaxRowBytes {
					return nil, nil, ErrPrivateSource
				}
			}
		}
		if page.Next == "" {
			return media, playlists, nil
		}
		if seen[page.Next] || page.Next == cursor {
			return nil, nil, ErrPrivateSource
		}
		seen[page.Next] = true
		cursor = page.Next
	}
}

func (p *PrivateSource) capture(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID) (pg.PrivateSnapshotCounts, error) {
	snapshot, err := ReadPrivateStorageInventory(ctx, tx, scope)
	if err != nil {
		return pg.PrivateSnapshotCounts{}, err
	}
	if err := snapshot.PersonalCompatible(); err != nil {
		return pg.PrivateSnapshotCounts{}, err
	}
	rows := snapshot.Rows()
	defer func() {
		for _, row := range rows {
			clear(row.JSON)
		}
	}()
	var metadataBytes int
	for _, row := range rows {
		metadataBytes += len(row.JSON)
	}
	media := []model.MediaRow{}
	playlists := map[string][]byte{}
	defer func() {
		for _, raw := range playlists {
			clear(raw)
		}
	}()
	var playlistBytes int
	for _, video := range snapshot.Videos() {
		values, lists, err := p.videoMedia(ctx, scope, video.ID, model.MaxRowBytes/1024-len(media))
		if err != nil {
			return pg.PrivateSnapshotCounts{}, err
		}
		media = append(media, values...)
		for id, raw := range lists {
			playlists[id] = raw
			playlistBytes += len(raw)
			if playlistBytes > model.MaxRowBytes {
				return pg.PrivateSnapshotCounts{}, ErrPrivateSource
			}
		}
	}
	descriptors := make([]storage.StreamSourceRow, 0, len(media))
	for _, value := range media {
		raw, err := json.Marshal(value)
		if err != nil {
			return pg.PrivateSnapshotCounts{}, err
		}
		metadataBytes += len(raw)
		if metadataBytes > model.MaxRowBytes {
			clear(raw)
			return pg.PrivateSnapshotCounts{}, ErrPrivateSource
		}
		row, err := model.RetainedRow("media", raw)
		clear(raw)
		if err != nil {
			return pg.PrivateSnapshotCounts{}, err
		}
		rows = append(rows, row)
		descriptors = append(descriptors, value.Descriptor())
	}
	complete, err := model.NewSnapshot(scope, rows)
	if err != nil {
		return pg.PrivateSnapshotCounts{}, err
	}
	if err := complete.ValidateMedia(descriptors); err != nil {
		return pg.PrivateSnapshotCounts{}, err
	}
	if err := complete.ValidatePlaylists(playlists); err != nil {
		return pg.PrivateSnapshotCounts{}, err
	}
	return insertCastCapture(ctx, tx, scope, epoch, rows, media)
}

func insertCastCapture(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID, rows []model.Row, media []model.MediaRow) (pg.PrivateSnapshotCounts, error) {
	for _, row := range rows {
		sum := sha256.Sum256(row.JSON)
		if _, err := tx.Exec(ctx, `INSERT INTO cast_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload,size_bytes,sha256)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, scope.Tenant, scope.Subject, epoch, row.Kind, row.ID, row.JSON, len(row.JSON), hex.EncodeToString(sum[:])); err != nil {
			return pg.PrivateSnapshotCounts{}, err
		}
	}
	for _, value := range media {
		if _, err := tx.Exec(ctx, `INSERT INTO cast_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,size_bytes,sha256,storage_key)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, scope.Tenant, scope.Subject, epoch, model.BinaryKind, value.ID, value.Size, value.SHA256, value.Key); err != nil {
			return pg.PrivateSnapshotCounts{}, err
		}
	}
	return pg.PrivateSnapshotCounts{Rows: int64(len(rows) + len(media)), Blobs: int64(len(media))}, nil
}
