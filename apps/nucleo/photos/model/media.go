// SPDX-License-Identifier: Apache-2.0
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// MediaRow binds exact retained bytes to an asset and, for a derived image, its
// recipe/variant. It contains no key, Matrix reference or plaintext bytes.
type MediaRow struct {
	ID          string   `json:"id"`
	Tenant      string   `json:"tenant"`
	Owner       string   `json:"owner_sub"`
	AssetID     string   `json:"asset_id"`
	Role        string   `json:"role"`
	Size        int64    `json:"size"`
	SHA256      string   `json:"sha256"`
	ContentType string   `json:"content_type"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	Variant     Variante `json:"variant,omitempty"`
	Format      string   `json:"format,omitempty"`
}

func (m MediaRow) Descriptor() storage.StreamSourceRow {
	return storage.StreamSourceRow{Kind: m.Role, ID: m.ID, Size: m.Size, SHA256: m.SHA256}
}

func (m MediaRow) Validate() error {
	if !canonicalUUID(m.AssetID) || m.ContentType == "" || m.Descriptor().Validate() != nil {
		return ErrSnapshot
	}
	switch m.Role {
	case "original":
		if m.ID != m.AssetID || m.Fingerprint != "" || m.Variant != "" || m.Format != "" {
			return ErrSnapshot
		}
	case "variant":
		key, err := (Derivada{ActivoID: parsedUUID(m.AssetID), Huella: m.Fingerprint, Variante: m.Variant, Formato: m.Format}).Clave()
		if err != nil {
			return ErrSnapshot
		}
		sum := sha256.Sum256([]byte(key))
		if m.ID != hex.EncodeToString(sum[:]) {
			return ErrSnapshot
		}
	default:
		return ErrSnapshot
	}
	return nil
}

func (s *Snapshot) Media() []MediaRow {
	result := make([]MediaRow, 0)
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

// ValidateMedia requires every original and every retained derived descriptor.
// A catalog-only import never opens readiness for a nonempty gallery.
func (s *Snapshot) ValidateMedia(rows []storage.StreamSourceRow, linked ...MediaRow) error {
	actual := make(map[string]storage.StreamSourceRow, len(rows))
	for _, row := range rows {
		key := row.Kind + "\x00" + row.ID
		if _, exists := actual[key]; exists || row.Validate() != nil {
			return ErrSnapshot
		}
		actual[key] = row
	}
	originals := map[string]bool{}
	media := append(s.Media(), linked...)
	if len(actual) != len(media) {
		return ErrSnapshot
	}
	for _, m := range media {
		if m.Validate() != nil {
			return ErrSnapshot
		}
		if actual[m.Role+"\x00"+m.ID] != m.Descriptor() {
			return ErrSnapshot
		}
		asset, exists := s.assets[m.AssetID]
		if !exists || m.Owner != asset.Owner {
			return ErrSnapshot
		}
		if m.Role == "original" {
			if m.Size != asset.Bytes || m.ContentType != asset.MediaType {
				return ErrSnapshot
			}
			originals[m.AssetID] = true
		}
	}
	for id := range s.assets {
		if !originals[id] {
			return ErrSnapshot
		}
	}
	return nil
}
