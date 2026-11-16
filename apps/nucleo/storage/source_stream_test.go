// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func streamRow(id string) StreamSourceRow {
	sum := sha256.Sum256([]byte("synthetic immutable original"))
	return StreamSourceRow{Kind: "note", ID: id, Size: 16 << 20, SHA256: hex.EncodeToString(sum[:])}
}

func TestStreamSourcePreservesLargeDescriptorsAndChecksSnapshotCursor(t *testing.T) {
	scope := catalogScope
	epoch := "12345678-1234-1234-1234-123456789abc"
	page := StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []StreamSourceRow{streamRow("b"), streamRow("c")}, Next: &LegacySourceCursor{Kind: "note", ID: "c"}}
	if err := page.Validate(scope, epoch, LegacySourceCursor{Kind: "note", ID: "a"}); err != nil {
		t.Fatal("large source descriptor rejected", err)
	}
	if err := (StreamSourcePage{Scope: scope, Epoch: epoch}).Validate(scope, epoch, LegacySourceCursor{}); err != nil {
		t.Fatal("empty final page rejected", err)
	}
	for _, fault := range []string{"scope", "epoch", "after", "before", "duplicate", "next", "empty-next", "limit", "hash", "hash-upper", "negative", "size", "kind", "id"} {
		t.Run(fault, func(t *testing.T) {
			p := StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []StreamSourceRow{streamRow("b"), streamRow("c")}}
			after := LegacySourceCursor{Kind: "note", ID: "a"}
			switch fault {
			case "scope":
				p.Scope.Product = "mail"
			case "epoch":
				p.Epoch = "12345678-1234-1234-1234-123456789abd"
			case "after":
				after = LegacySourceCursor{ID: "a"}
			case "before":
				after.ID = "b"
			case "duplicate":
				p.Rows[1] = p.Rows[0]
			case "next":
				p.Next = &LegacySourceCursor{Kind: "note", ID: "other"}
			case "empty-next":
				p.Rows = nil
				p.Next = &LegacySourceCursor{Kind: "note", ID: "b"}
			case "limit":
				for len(p.Rows) <= MaxLegacySourceRows {
					p.Rows = append(p.Rows, streamRow("more"))
				}
			case "hash":
				p.Rows[0].SHA256 = "not-a-complete-digest"
			case "hash-upper":
				p.Rows[0].SHA256 = strings.ToUpper(p.Rows[0].SHA256)
			case "negative":
				p.Rows[0].Size = -1
			case "size":
				p.Rows[0].Size = MaxFileBytes + 1
			case "kind":
				p.Rows[0].Kind = "note/bad"
			case "id":
				p.Rows[0].ID = "bad\x00id"
			}
			if err := p.Validate(scope, epoch, after); err == nil {
				t.Fatal("invalid retained source accepted", fault)
			}
		})
	}
}
