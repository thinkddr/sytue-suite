// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestFileLinkIsAnAuthenticatedContentRecordOutsideIndexCatalog(t *testing.T) {
	scope := Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
	for _, change := range []string{"valid", "scope", "role", "kind", "revision", "root", "parents"} {
		t.Run(change, func(t *testing.T) {
			link := FileLink{Kind: FileLinkKind, Content: ContentPointer{Kind: "file", RevisionID: "revision", Reference: Reference{RoomID: "!photos:test", EventID: "$file"}}}
			record := Record{Version: 1, Scope: scope, ObjectID: "plan", RevisionID: "revision", Role: "content"}
			switch change {
			case "scope":
				record.Scope.Subject = "other"
			case "role":
				record.Role = "index"
			case "kind":
				link.Kind = "other"
			case "revision":
				link.Content.RevisionID = "other"
			case "root":
				link.Content.Reference = Reference{}
			case "parents":
				record.Previous = []string{"$previous"}
			}
			record.Payload, _ = json.Marshal(link)
			got, err := DecodeFileLink(record, scope)
			if change == "valid" {
				if err != nil || got.Content != link.Content {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrRecord) {
				t.Fatal("invalid sidecar link accepted", err)
			}
		})
	}
}
