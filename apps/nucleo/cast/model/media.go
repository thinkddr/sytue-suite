// SPDX-License-Identifier: Apache-2.0
package model

import (
	"encoding/json"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const BinaryKind = "media_bytes"

// VideoRow is a device projection of retained columns. Snapshot keeps the raw
// source bytes independently, including every column absent from this view.
type VideoRow struct {
	ID            string     `json:"id"`
	Tenant        string     `json:"tenant"`
	Owner         string     `json:"owner_sub"`
	Kind          Kind       `json:"kind"`
	Status        Status     `json:"status"`
	Visibility    Visibility `json:"visibility"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	OriginalKey   string     `json:"original_key"`
	HLSKey        string     `json:"hls_key"`
	WebMKey       string     `json:"webm_key"`
	WebMQualities []int      `json:"webm_qualities"`
	PosterKey     string     `json:"poster_key"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeletedAt     *time.Time `json:"deleted_at"`
}

// MediaRow describes one exact retained object, including every playlist and
// segment. Original byte identity and object relationships remain separate from
// codec/quality choices; no re-encoding can stand in for a retained object.
type MediaRow struct {
	ID          string `json:"id"`
	Tenant      string `json:"tenant"`
	Owner       string `json:"owner_sub"`
	VideoID     string `json:"video_id"`
	Key         string `json:"key"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
}

func MediaID(video, key string) (string, error) {
	if !canonicalUUID(video) || key == "" || strings.HasPrefix(key, "/") || strings.ContainsAny(key, "\\\x00") || path.Clean(key) != key || strings.HasPrefix(key, "../") || key == "." {
		return "", ErrSnapshot
	}
	return compositeID(video, key)
}

func (m MediaRow) Descriptor() storage.StreamSourceRow {
	return storage.StreamSourceRow{Kind: BinaryKind, ID: m.ID, Size: m.Size, SHA256: m.SHA256}
}

func validateMediaRow(raw []byte) error {
	var value MediaRow
	if json.Unmarshal(raw, &value) != nil || value.Owner == "" || value.ContentType == "" || value.Descriptor().Validate() != nil {
		return ErrSnapshot
	}
	id, err := MediaID(value.VideoID, value.Key)
	if err != nil || id != value.ID {
		return ErrSnapshot
	}
	return nil
}

func (s *Snapshot) Media() []MediaRow {
	result := []MediaRow{}
	for _, row := range s.rows {
		if row.Kind != "media" {
			continue
		}
		var value MediaRow
		_ = json.Unmarshal(row.JSON, &value)
		result = append(result, value)
	}
	slices.SortFunc(result, func(a, b MediaRow) int { return strings.Compare(a.ID, b.ID) })
	return result
}

func (s *Snapshot) Videos() []VideoRow {
	result := []VideoRow{}
	for _, row := range s.rows {
		if row.Kind != "video" {
			continue
		}
		var value VideoRow
		_ = json.Unmarshal(row.JSON, &value)
		result = append(result, value)
	}
	slices.SortFunc(result, func(a, b VideoRow) int { return strings.Compare(a.ID, b.ID) })
	return result
}

// ValidateMedia requires exact descriptors for all retained objects and each
// declared original/output root. ValidatePlaylists checks descendant references.
func (s *Snapshot) ValidateMedia(rows []storage.StreamSourceRow) error {
	actual := make(map[string]storage.StreamSourceRow, len(rows))
	for _, row := range rows {
		if _, exists := actual[row.ID]; exists || row.Kind != BinaryKind || row.Validate() != nil {
			return ErrSnapshot
		}
		actual[row.ID] = row
	}
	media := s.Media()
	if len(actual) != len(media) {
		return ErrSnapshot
	}
	byVideo := map[string]map[string]bool{}
	for _, m := range media {
		if actual[m.ID] != m.Descriptor() {
			return ErrSnapshot
		}
		if byVideo[m.VideoID] == nil {
			byVideo[m.VideoID] = map[string]bool{}
		}
		if byVideo[m.VideoID][m.Key] {
			return ErrSnapshot
		}
		byVideo[m.VideoID][m.Key] = true
	}
	for _, video := range s.Videos() {
		if video.OriginalKey == "" {
			return ErrSnapshot
		}
		for _, key := range []string{video.OriginalKey, video.HLSKey, video.WebMKey, video.PosterKey} {
			if key != "" && !byVideo[video.ID][key] {
				return ErrSnapshot
			}
		}
		// The native player derives each quality's path from WebMKey. Do not
		// advertise a variant unless the same video retained its exact bytes.
		seen := map[int]bool{}
		for _, quality := range video.WebMQualities {
			if video.WebMKey == "" || quality <= 0 || seen[quality] {
				return ErrSnapshot
			}
			seen[quality] = true
			key := path.Join(path.Dir(video.WebMKey), strconv.Itoa(quality)+"p.webm")
			if !byVideo[video.ID][key] {
				return ErrSnapshot
			}
		}
	}
	return nil
}

// TipoDe is the shared stored and streamed media Content-Type.
func TipoDe(ruta string) string {
	switch strings.ToLower(path.Ext(ruta)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".m4s":
		return "video/iso.segment"
	case ".mp4":
		return "video/mp4"
	case ".ts":
		return "video/mp2t"
	case ".webm":
		return "video/webm"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".png":
		return "image/png"
	}
	return "application/octet-stream"
}
