// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxLegacySourceRows  = 50
	MaxLegacySourceBytes = 64 << 10
)

// LegacyMigrationState describes a retained plaintext source during transition.
// Frozen is a consistent source, NOT a claim that application E2E is complete.
type LegacyMigrationState struct {
	Scope     Scope      `json:"scope"`
	Mode      string     `json:"mode"`
	Epoch     string     `json:"epoch,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	Rows      int64      `json:"rows"`
	Blobs     int64      `json:"blobs"`
}

type LegacySourceRow struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Raw UTF-8 JSON bytes, base64 on the wire; never round domain integers.
	Payload []byte `json:"payload"`
}

type LegacySourceCursor struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type LegacySourcePage struct {
	Scope Scope               `json:"scope"`
	Epoch string              `json:"epoch"`
	Rows  []LegacySourceRow   `json:"rows"`
	Next  *LegacySourceCursor `json:"next,omitempty"`
}

func MigrationEpoch(value string) bool {
	if len(value) != 36 || value == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	for i, c := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func SourceCursorValid(cursor LegacySourceCursor) bool {
	return Identifier(cursor.Kind) && cursor.ID != "" && len(cursor.ID) <= 2048 && utf8.ValidString(cursor.ID) && !strings.ContainsAny(cursor.ID, "\x00\r\n\t")
}

func (s LegacyMigrationState) Validate(scope Scope) error {
	if s.Scope != scope || !scope.Valid() || s.Rows < 0 || s.Blobs < 0 || s.Blobs > s.Rows || s.Rows >= 1<<53 {
		return ErrRecord
	}
	if s.Mode == "legacy" {
		if s.Epoch != "" || s.StartedAt != nil || s.Rows != 0 || s.Blobs != 0 {
			return ErrRecord
		}
		return nil
	}
	if (s.Mode != "frozen" && s.Mode != "private") || !MigrationEpoch(s.Epoch) || s.StartedAt == nil || s.StartedAt.IsZero() {
		return ErrRecord
	}
	return nil
}

func (p LegacySourcePage) Validate(scope Scope, epoch string) error {
	if p.Scope != scope || !scope.Valid() || !MigrationEpoch(epoch) || p.Epoch != epoch || len(p.Rows) > MaxLegacySourceRows {
		return ErrRecord
	}
	for i, row := range p.Rows {
		if i > 0 {
			last := p.Rows[i-1]
			if last.Kind > row.Kind || (last.Kind == row.Kind && last.ID >= row.ID) {
				return ErrRecord
			}
		}
		if !SourceCursorValid(LegacySourceCursor{Kind: row.Kind, ID: row.ID}) || len(row.Payload) > MaxLegacySourceBytes || !utf8.Valid(row.Payload) || !json.Valid(row.Payload) {
			return ErrRecord
		}
	}
	if p.Next != nil {
		if len(p.Rows) == 0 || !SourceCursorValid(*p.Next) {
			return ErrRecord
		}
		last := p.Rows[len(p.Rows)-1]
		if p.Next.Kind != last.Kind || p.Next.ID != last.ID {
			return ErrRecord
		}
	}
	return nil
}

// SameRetainedSource compares the original frozen source, not completion or
// current authorization. A legacy/empty epoch is never a retained snapshot.
func (s LegacyMigrationState) SameRetainedSource(other LegacyMigrationState) bool {
	return s.Mode != "legacy" && s.Validate(s.Scope) == nil && other.Validate(s.Scope) == nil && s.Scope == other.Scope && s.Mode == other.Mode && s.Epoch == other.Epoch && s.Rows == other.Rows && s.Blobs == other.Blobs && s.StartedAt.Equal(*other.StartedAt)
}
