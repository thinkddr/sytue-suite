// SPDX-License-Identifier: Apache-2.0
package cast

import (
	"context"
	"encoding/json"
	"path"
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/cast/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const privateMediaPrefix = "sytue-private-cast:"

type privateData struct{ repository *Repository }

// Data adapts the authenticated private catalog to the existing typed API. It
// does not activate a session or reinterpret public/org/people audiences.
func (r *Repository) Data() api.CastData                  { return &privateData{repository: r} }
func (d *privateData) PrivateStorageScope() storage.Scope { return d.repository.scope }

func (d *privateData) project(raw []byte) (api.CastVideo, model.VideoRow, error) {
	var video model.VideoRow
	var result api.CastVideo
	scope := d.repository.scope
	if json.Unmarshal(raw, &video) != nil || json.Unmarshal(raw, &result) != nil || video.Tenant != scope.Tenant || video.Owner != scope.Subject || video.Visibility != model.PrivateVisibility || video.DeletedAt != nil {
		return result, video, ErrSource
	}
	result.Owner = api.CastOwner{Sub: video.Owner}
	result.PosterURL = ""
	result.Can = api.CastCan{} // mutation/ACL capabilities are supplied only by their private implementations
	if video.PosterKey != "" {
		_, err := model.MediaID(video.ID, video.PosterKey)
		if err != nil {
			return result, video, err
		}
		result.PosterURL = privateMediaPrefix + video.ID + "/" + video.PosterKey
	}
	return result, video, nil
}

func (d *privateData) Video(ctx context.Context, id string) (api.CastVideo, error) {
	raw, err := d.repository.ReadRaw(ctx, "video", id)
	defer clear(raw)
	if err != nil {
		return api.CastVideo{}, err
	}
	result, video, err := d.project(raw)
	if err != nil {
		return api.CastVideo{}, err
	}
	if video.ID != id {
		return api.CastVideo{}, ErrSource
	}
	return result, nil
}

func (d *privateData) Videos(ctx context.Context, scope, status, q, cursor string, limit int) (api.CastPage, error) {
	if scope != "" && scope != "mine" {
		return api.CastPage{}, ErrShared
	}
	versions, err := d.repository.engine.Selected("video")
	if err != nil {
		return api.CastPage{}, err
	}
	videos := []api.CastVideo{}
	for _, version := range versions {
		raw, err := d.repository.engine.ReadJSON(ctx, version)
		if err != nil {
			return api.CastPage{}, err
		}
		var retained model.VideoRow
		err = json.Unmarshal(raw, &retained)
		if err != nil {
			clear(raw)
			return api.CastPage{}, ErrSource
		}
		if retained.DeletedAt != nil {
			clear(raw)
			continue
		}
		video, _, err := d.project(raw)
		clear(raw)
		if err != nil {
			return api.CastPage{}, err
		}
		if (status == "" || video.Status == status) && (q == "" || buscar.Contiene(video.Title+" "+video.Description, q)) {
			videos = append(videos, video)
		}
	}
	slices.SortFunc(videos, func(a, b api.CastVideo) int {
		if cmp := b.CreatedAt.Compare(a.CreatedAt); cmp != 0 {
			return cmp
		}
		return strings.Compare(b.ID, a.ID)
	})
	start := 0
	if cursor != "" {
		position := slices.IndexFunc(videos, func(v api.CastVideo) bool { return v.ID == cursor })
		if position < 0 {
			return api.CastPage{}, ErrSource
		}
		start = position + 1
	}
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 100)
	end := min(start+limit, len(videos))
	result := api.CastPage{Videos: videos[start:end]}
	if end < len(videos) {
		result.NextCursor = videos[end-1].ID
	}
	return result, nil
}

func (d *privateData) Play(ctx context.Context, id string) (api.CastPlay, error) {
	raw, err := d.repository.ReadRaw(ctx, "video", id)
	defer clear(raw)
	if err != nil {
		return api.CastPlay{}, err
	}
	projected, video, err := d.project(raw)
	if err != nil {
		return api.CastPlay{}, err
	}
	if video.ID != id {
		return api.CastPlay{}, ErrSource
	}
	if video.Kind != model.KindUpload || video.Status != model.StatusReady {
		return api.CastPlay{}, model.ErrDeviceWork
	}
	result := api.CastPlay{Kind: string(video.Kind), DurationMS: projected.DurationMS, PosterURL: projected.PosterURL, WebMQualities: projected.WebMQualities}
	for _, entry := range []struct {
		key    string
		target *string
	}{{video.OriginalKey, &result.DownloadURL}, {video.WebMKey, &result.WebMURL}} {
		if entry.key == "" {
			continue
		}
		_, err := model.MediaID(video.ID, entry.key)
		if err != nil {
			return api.CastPlay{}, err
		}
		*entry.target = privateMediaPrefix + video.ID + "/" + entry.key
	}
	// The catalog can change after it was opened. Check the retained metadata
	// for each variant again; OpenMedia verifies the selected binary commitment.
	seen := map[int]bool{}
	for _, quality := range result.WebMQualities {
		if quality <= 0 || video.WebMKey == "" || seen[quality] {
			return api.CastPlay{}, ErrSource
		}
		seen[quality] = true
		key := path.Join(path.Dir(video.WebMKey), strconv.Itoa(quality)+"p.webm")
		if _, err := d.mediaRow(ctx, video.ID, key); err != nil {
			return api.CastPlay{}, err
		}
	}
	// Retained HLS playlists remain exact archive bytes. Exposing their original
	// relative URLs would make an OS player fetch clear server media; HLS playback
	// needs a device resolver before this field can be offered.
	return result, nil
}

func (d *privateData) OpenMedia(ctx context.Context, route string) (api.CastPrivateMedia, error) {
	if !strings.HasPrefix(route, privateMediaPrefix) {
		return nil, ErrSource
	}
	videoID, key, ok := strings.Cut(strings.TrimPrefix(route, privateMediaPrefix), "/")
	if !ok {
		return nil, ErrSource
	}
	media, err := d.mediaRow(ctx, videoID, key)
	if err != nil {
		return nil, err
	}
	if _, err := d.Video(ctx, media.VideoID); err != nil {
		return nil, err
	}
	reader, err := d.repository.engine.OpenBinary(ctx, model.BinaryKind, media.ID)
	if err != nil {
		return nil, err
	}
	if reader.Size() != media.Size {
		_ = reader.Close()
		return nil, ErrSource
	}
	return reader, nil
}

func (d *privateData) mediaRow(ctx context.Context, videoID, key string) (model.MediaRow, error) {
	id, err := model.MediaID(videoID, key)
	if err != nil {
		return model.MediaRow{}, ErrSource
	}
	raw, err := d.repository.ReadRaw(ctx, "media", id)
	defer clear(raw)
	if err != nil {
		return model.MediaRow{}, err
	}
	var media model.MediaRow
	scope := d.repository.scope
	if json.Unmarshal(raw, &media) != nil || media.ID != id || media.VideoID != videoID || media.Key != key || media.Tenant != scope.Tenant || media.Owner != scope.Subject || media.Descriptor().Validate() != nil {
		return model.MediaRow{}, ErrSource
	}
	expected, err := model.MediaID(media.VideoID, media.Key)
	if err != nil || expected != id {
		return model.MediaRow{}, ErrSource
	}
	return media, nil
}
