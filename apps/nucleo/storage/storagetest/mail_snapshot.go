// SPDX-License-Identifier: Apache-2.0
package storagetest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type MailSnapshotRow struct {
	Descriptor storage.StreamSourceRow `json:"descriptor"`
	Body       []byte                  `json:"body"`
}

// MailSnapshot contains explicitly synthetic historical data exported by the
// actual PostgreSQL fixture. It is never a production ingress capability.
type MailSnapshot struct {
	State     storage.AppMigrationState `json:"state"`
	Rows      []MailSnapshotRow         `json:"rows"`
	LiveID    string                    `json:"live_id"`
	DeletedID string                    `json:"deleted_id"`
	InboxID   string                    `json:"inbox_id"`
	LabelID   string                    `json:"label_id"`
	Signature string                    `json:"signature"`
}

func DecodeMailSnapshot(raw []byte) (MailSnapshot, error) {
	var value MailSnapshot
	if json.Unmarshal(raw, &value) != nil || value.Validate() != nil {
		return MailSnapshot{}, errors.New("invalid synthetic PostgreSQL Mail snapshot")
	}
	slices.SortFunc(value.Rows, func(a, b MailSnapshotRow) int {
		if order := strings.Compare(a.Descriptor.Kind, b.Descriptor.Kind); order != 0 {
			return order
		}
		return strings.Compare(a.Descriptor.ID, b.Descriptor.ID)
	})
	return value, nil
}

func (v MailSnapshot) Validate() error {
	if v.State.Validate(v.State.Scope) != nil || v.State.Scope.Product != "mail" || v.State.Mode != "frozen" || len(v.Rows) == 0 {
		return storage.ErrRecord
	}
	seen := map[storage.LegacySourceCursor]bool{}
	var rows, bodies int64
	for _, row := range v.Rows {
		key := storage.LegacySourceCursor{Kind: row.Descriptor.Kind, ID: row.Descriptor.ID}
		sum := sha256.Sum256(row.Body)
		if seen[key] || row.Descriptor.Validate() != nil || row.Descriptor.Size != int64(len(row.Body)) || row.Descriptor.SHA256 != hex.EncodeToString(sum[:]) {
			return storage.ErrRecord
		}
		seen[key] = true
		if strings.HasPrefix(key.Kind, "mime_") {
			bodies++
		} else {
			rows++
		}
	}
	if rows != v.State.Rows || bodies != v.State.Blobs {
		return storage.ErrRecord
	}
	return nil
}

func (v MailSnapshot) Page(after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	page := storage.StreamSourcePage{Scope: v.State.Scope, Epoch: v.State.Epoch, Rows: []storage.StreamSourceRow{}}
	if limit < 1 || limit > storage.MaxLegacySourceRows || after != (storage.LegacySourceCursor{}) && !storage.SourceCursorValid(after) {
		return page, storage.ErrRecord
	}
	for _, row := range v.Rows {
		item := row.Descriptor
		if item.Kind < after.Kind || item.Kind == after.Kind && item.ID <= after.ID {
			continue
		}
		if len(page.Rows) == limit {
			last := page.Rows[len(page.Rows)-1]
			page.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
			break
		}
		page.Rows = append(page.Rows, item)
	}
	return page, nil
}

func (v MailSnapshot) Body(kind, id string) (MailSnapshotRow, bool) {
	for _, row := range v.Rows {
		if row.Descriptor.Kind == kind && row.Descriptor.ID == id {
			return row, true
		}
	}
	return MailSnapshotRow{}, false
}
