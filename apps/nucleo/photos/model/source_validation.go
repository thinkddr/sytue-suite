// SPDX-License-Identifier: Apache-2.0
package model

import (
	"encoding/json"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// SourceValidationRow is ONLY a small graph-validation projection. The exact
// original JSON and its descriptor remain the migration input; this projection
// must never be published as an application row or used to regenerate one.
// Large camera strings/vectors and unknown fields therefore do not accumulate
// while the device checks relationships and captures original pins.
func SourceValidationRow(scope storage.Scope, row Row) (Row, error) {
	if err := ValidateRow(scope, row); err != nil {
		return Row{}, err
	}
	var value any
	switch row.Kind {
	case "asset":
		var asset struct {
			ID           string          `json:"id"`
			Name         string          `json:"name"`
			Tenant       string          `json:"tenant"`
			Owner        string          `json:"owner_sub"`
			DriveID      string          `json:"drive_file_id"`
			MediaType    string          `json:"media_type"`
			Bytes        int64           `json:"bytes"`
			Width        int             `json:"width"`
			Height       int             `json:"height"`
			Orientation  int             `json:"orientation"`
			UploadedAt   time.Time       `json:"uploaded_at"`
			PrivateMedia json.RawMessage `json:"private_media,omitempty"`
		}
		if err := json.Unmarshal(row.JSON, &asset); err != nil {
			return Row{}, err
		}
		if len(asset.PrivateMedia) == 0 {
			asset.Name = ""
		}
		value = asset
	case "album":
		var album AlbumRow
		if err := json.Unmarshal(row.JSON, &album); err != nil {
			return Row{}, err
		}
		album.Title = "Retained source validation"
		value = album
	case "edit":
		var edit EditRow
		if err := json.Unmarshal(row.JSON, &edit); err != nil {
			return Row{}, err
		}
		value = edit
	case "album_asset":
		var member MembershipRow
		if err := json.Unmarshal(row.JSON, &member); err != nil {
			return Row{}, err
		}
		value = member
	case "album_person":
		return Row{}, ErrShared
	case "media":
		var media MediaRow
		if err := json.Unmarshal(row.JSON, &media); err != nil {
			return Row{}, err
		}
		value = media
	default:
		return Row{}, ErrSnapshot
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return Row{}, err
	}
	result := Row{Kind: row.Kind, ID: row.ID, JSON: raw}
	if len(raw) > MaxRowBytes || ValidateRow(scope, result) != nil {
		clear(raw)
		return Row{}, ErrSnapshot
	}
	return result, nil
}
