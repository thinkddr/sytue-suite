// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"encoding/hex"
)

// StreamSourceRow is an immutable retained source's byte descriptor. Large JSON
// and binary bodies are opened separately by the application's source port; the
// page never parses/re-encodes their domain numbers or stores their contents.
type StreamSourceRow struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func (r StreamSourceRow) Validate() error {
	if !SourceCursorValid(LegacySourceCursor{Kind: r.Kind, ID: r.ID}) || r.Size < 0 || r.Size > MaxFileBytes {
		return ErrRecord
	}
	hash, err := hex.DecodeString(r.SHA256)
	if err != nil || len(hash) != 32 || hex.EncodeToString(hash) != r.SHA256 {
		return ErrRecord
	}
	return nil
}

type StreamSourcePage struct {
	Scope Scope               `json:"scope"`
	Epoch string              `json:"epoch"`
	Rows  []StreamSourceRow   `json:"rows"`
	Next  *LegacySourceCursor `json:"next,omitempty"`
}

// Validate binds the page to the requested snapshot AND cursor. Empty final
// pages are valid; an empty page cannot advance. A next cursor names exactly its
// last row, preventing duplicate/omitted work from becoming a completion receipt.
func (p StreamSourcePage) Validate(scope Scope, epoch string, after LegacySourceCursor) error {
	if !scope.Valid() || p.Scope != scope || !MigrationEpoch(epoch) || p.Epoch != epoch || len(p.Rows) > MaxLegacySourceRows {
		return ErrRecord
	}
	if after != (LegacySourceCursor{}) && !SourceCursorValid(after) {
		return ErrRecord
	}
	last := after
	for _, row := range p.Rows {
		if row.Validate() != nil || last.Kind != "" && (row.Kind < last.Kind || row.Kind == last.Kind && row.ID <= last.ID) {
			return ErrRecord
		}
		last = LegacySourceCursor{Kind: row.Kind, ID: row.ID}
	}
	if p.Next != nil && (len(p.Rows) == 0 || *p.Next != last) {
		return ErrRecord
	}
	return nil
}
