// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"encoding/json"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// SourcePartition is routing for retained owner input, never a recipient grant.
// Raw SQL rows contain management secrets, including inside CRDT operations;
// none may be published to an epoch until the original domain projection and
// its resource capability have separately authorized that delivery.
type SourcePartition struct {
	Scope      storage.Scope
	DocumentID string
	Audience   SourceAudience
}

// SourceAudience keeps personal and owner-wide data outside a document's
// management archive. A management archive itself is NOT viewer/editor data.
type SourceAudience string

const (
	SourceDocumentManagement SourceAudience = "document_management"
	SourcePersonal           SourceAudience = "personal"
	SourceOwner              SourceAudience = "owner"
)

func parseSourceUUIDField(value map[string]json.RawMessage, field string) (string, bool) {
	var valueID string
	if json.Unmarshal(value[field], &valueID) != nil {
		return "", false
	}
	parsed, err := uuid.Parse(valueID)
	if err != nil || parsed == uuid.Nil || parsed.String() != valueID {
		return "", false
	}
	return valueID, true
}

func matchesSourceOwnerField(value map[string]json.RawMessage, field, subject string) bool {
	var sub string
	return json.Unmarshal(value[field], &sub) == nil && sub == subject
}

// PartitionSourceRow describes exactly one row, so importers can stream large
// snapshots through the shared encrypted source engine. It neither buffers a
// whole owner's archive nor re-encodes unknown fields or SQL int64 values.
// Source/epoch completeness and parent relationships remain the importer's
// responsibility; successful routing cannot mark a migration ready.
func PartitionSourceRow(scope storage.Scope, kind, id string, raw []byte) (SourcePartition, error) {
	if ValidateSourceRow(scope, kind, id, raw) != nil {
		return SourcePartition{}, ErrSource
	}
	if _, ok := ProductKind(scope.Product); !ok {
		return SourcePartition{}, ErrSource
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return SourcePartition{}, ErrSource
	}
	result := SourcePartition{Scope: scope}
	doc := func(field string) bool {
		docID, ok := parseSourceUUIDField(value, field)
		if ok {
			result.DocumentID = docID
		}
		return ok
	}
	switch kind {
	case "document":
		if !doc("id") {
			return SourcePartition{}, ErrSource
		}
		result.Audience = SourceDocumentManagement
	case "operation", "snapshot", "version", "permission", "share_link", "comment", "blob", "edit_request", "protection", "protection_key", "template":
		if !doc("document_id") {
			return SourcePartition{}, ErrSource
		}
		result.Audience = SourceDocumentManagement
	case "filter_view":
		if !doc("document_id") || !matchesSourceOwnerField(value, "sub", scope.Subject) {
			return SourcePartition{}, ErrSource
		}
		// Views on documents owned by somebody else are explicitly present in the
		// real source. Their per-person state never becomes a room-wide setting.
		result.Audience = SourcePersonal
	case "template_log":
		if !doc("document_id") {
			return SourcePartition{}, ErrSource
		}
		// A source log can name a foreign document through actor_sub. Preserve it
		// in the owner archive; a later manager projection resolves the relationship.
		result.Audience = SourceOwner
	default:
		return partitionOwnerSourceRow(scope, kind, value)
	}
	return result, nil
}

func partitionOwnerSourceRow(scope storage.Scope, kind string, value map[string]json.RawMessage) (SourcePartition, error) {
	result := SourcePartition{Scope: scope, Audience: SourceOwner}
	switch kind {
	case "script":
		var actual Kind
		want, _ := ProductKind(scope.Product)
		if !matchesSourceOwnerField(value, "author_sub", scope.Subject) || json.Unmarshal(value["kind"], &actual) != nil || actual != want {
			return SourcePartition{}, ErrSource
		}
	case "script_version", "brand", "link_policy":
	case "script_run":
		if !matchesSourceOwnerField(value, "sub", scope.Subject) {
			return SourcePartition{}, ErrSource
		}
	case "connection", "query":
		if scope.Product != "sheets" {
			return SourcePartition{}, ErrSource
		}
		if kind == "connection" && !matchesSourceOwnerField(value, "created_by", scope.Subject) {
			return SourcePartition{}, ErrSource
		}
	default:
		return SourcePartition{}, ErrSource
	}
	return result, nil
}

func isImmutableSourceField(field string, primary []string) bool {
	switch field {
	case "tenant", "owner_sub", "document_id", "sub", "author_sub", "created_by", "actor_sub", "script_id", "connection_id", "kind":
		return true
	}
	for _, key := range primary {
		if field == key {
			return true
		}
	}
	return false
}

// PatchSourceRow retains every unknown source field and its exact JSON number
// token while replacing only existing fields named by the original store.
// A patch cannot alter the SQL primary key, tenant, owner or document routing.
// This is a lossless domain field transform, not a persistence/publication API.
func PatchSourceRow(scope storage.Scope, kind, id string, raw []byte, changes map[string]json.RawMessage) ([]byte, error) {
	before, err := PartitionSourceRow(scope, kind, id, raw)
	if err != nil || len(changes) == 0 {
		return nil, ErrSource
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return nil, ErrSource
	}
	primary := SourcePrimaryFields(kind)
	for field, replacement := range changes {
		if _, exists := value[field]; !exists || isImmutableSourceField(field, primary) || !json.Valid(replacement) {
			return nil, ErrSource
		}
		value[field] = bytes.Clone(replacement)
	}
	result, err := json.Marshal(value)
	if err != nil {
		return nil, ErrSource
	}
	after, err := PartitionSourceRow(scope, kind, id, result)
	if err != nil || after != before {
		clear(result)
		return nil, ErrSource
	}
	return result, nil
}
