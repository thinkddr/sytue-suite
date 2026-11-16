// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var partitionScope = storage.Scope{Tenant: "acme", Subject: "alice", Product: "sheets"}

const partitionDoc = "11111111-1111-4111-8111-111111111111"

func partitionRow(t *testing.T, kind string, edit func(map[string]any)) (string, []byte) {
	t.Helper()
	value := map[string]any{
		"tenant": "acme", "id": partitionDoc, "document_id": partitionDoc,
		"owner_sub": "alice", "kind": "spreadsheet", "seq": json.Number("9007199254740993"),
		"sub": "alice", "author_sub": "alice", "created_by": "alice", "actor_sub": "alice",
		"principal_kind": "user", "principal_id": "bob", "sujeto": "bob", "ambito": "libro",
		"script_id": partitionDoc, "connection_id": partitionDoc, "version": 1,
		"title": "Original", "password_hash": "private-manager-marker",
		"future": json.RawMessage(`{"counter":900719925474099312345,"negative":-900719925474099312345,"fraction":1.234567890123456789}`),
	}
	if kind == "script_run" {
		value["id"] = json.Number("9007199254740993")
	}
	if edit != nil {
		edit(value)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	primary := map[string]json.RawMessage{}
	for _, field := range SourcePrimaryFields(kind) {
		primary[field] = object[field]
	}
	id, err := SourceObjectID(kind, primary)
	if err != nil {
		t.Fatal(err)
	}
	return id, raw
}

func TestPartitionAllRetainedSourceRelationsWithoutRecipientPromotion(t *testing.T) {
	cases := map[string]SourceAudience{
		"document": SourceDocumentManagement, "operation": SourceDocumentManagement, "snapshot": SourceDocumentManagement,
		"version": SourceDocumentManagement, "permission": SourceDocumentManagement, "share_link": SourceDocumentManagement,
		"comment": SourceDocumentManagement, "blob": SourceDocumentManagement, "edit_request": SourceDocumentManagement,
		"protection": SourceDocumentManagement, "protection_key": SourceDocumentManagement, "template": SourceDocumentManagement,
		"filter_view": SourcePersonal, "template_log": SourceOwner, "script": SourceOwner, "script_version": SourceOwner,
		"script_run": SourceOwner, "brand": SourceOwner, "link_policy": SourceOwner, "connection": SourceOwner, "query": SourceOwner,
	}
	kinds := SourceKinds()
	if len(kinds) != len(cases)+1 {
		t.Fatal("source relation coverage differs from the original model inventory")
	}
	for _, kind := range kinds {
		if kind != BinarySourceKind {
			if _, present := cases[kind]; !present {
				t.Fatalf("source relation %s lacks a partition guard", kind)
			}
		}
	}
	for kind, audience := range cases {
		t.Run(kind, func(t *testing.T) {
			id, raw := partitionRow(t, kind, nil)
			before := bytes.Clone(raw)
			route, err := PartitionSourceRow(partitionScope, kind, id, raw)
			if err != nil || route.Scope != partitionScope || route.Audience != audience {
				t.Fatalf("route = %+v, %v", route, err)
			}
			if audience != SourceOwner && route.DocumentID != partitionDoc {
				t.Fatal("lost original document routing")
			}
			if !bytes.Equal(before, raw) {
				t.Fatal("routing rewrote original source bytes")
			}
			public, err := json.Marshal(route)
			if err != nil || bytes.Contains(public, []byte("private-manager-marker")) || bytes.Contains(public, []byte("future")) {
				t.Fatal("routing copied retained contents or secrets")
			}
		})
	}
}

func TestSourcePartitionKeepsForeignDocumentPersonalStatePrivate(t *testing.T) {
	foreign := "22222222-2222-4222-8222-222222222222"
	for _, kind := range []string{"filter_view", "template_log"} {
		id, raw := partitionRow(t, kind, func(value map[string]any) { value["document_id"] = foreign })
		route, err := PartitionSourceRow(partitionScope, kind, id, raw)
		if err != nil || route.DocumentID != foreign || route.Audience == SourceDocumentManagement {
			t.Fatalf("foreign %s became shared document state: %+v %v", kind, route, err)
		}
	}
}

func TestSourcePartitionRejectsForeignScopeOrRouting(t *testing.T) {
	cases := []struct {
		kind, field string
		value       any
	}{
		{"document", "owner_sub", "bob"},
		{"document", "kind", "document"},
		{"operation", "tenant", "other"},
		{"operation", "document_id", "foreign"},
		{"filter_view", "sub", "bob"},
		{"script", "author_sub", "bob"},
		{"script", "kind", "document"},
		{"script_run", "sub", "bob"},
		{"connection", "created_by", "bob"},
	}
	for _, tc := range cases {
		t.Run(tc.kind+"/"+tc.field, func(t *testing.T) {
			id, raw := partitionRow(t, tc.kind, func(value map[string]any) { value[tc.field] = tc.value })
			if _, err := PartitionSourceRow(partitionScope, tc.kind, id, raw); err == nil {
				t.Fatal("foreign source route accepted")
			}
		})
	}
	for _, kind := range []string{"connection", "query"} {
		id, raw := partitionRow(t, kind, nil)
		other := partitionScope
		other.Product = "docs"
		if _, err := PartitionSourceRow(other, kind, id, raw); err == nil {
			t.Fatal("Sheets-only private settings crossed products")
		}
	}
	if _, err := PartitionSourceRow(partitionScope, BinarySourceKind, strings.Repeat("a", 64), []byte(`{}`)); err == nil {
		t.Fatal("binary descriptor treated as a plaintext SQL row")
	}
}

func TestSourcePatchPreservesUnknownFieldsAndAllIntegerTokens(t *testing.T) {
	id, raw := partitionRow(t, "document", nil)
	before := bytes.Clone(raw)
	patched, err := PatchSourceRow(partitionScope, "document", id, raw, map[string]json.RawMessage{"title": json.RawMessage(`"Changed"`)})
	if err != nil {
		t.Fatal(err)
	}
	var original, actual map[string]json.RawMessage
	if json.Unmarshal(raw, &original) != nil || json.Unmarshal(patched, &actual) != nil {
		t.Fatal("invalid JSON")
	}
	if string(actual["title"]) != `"Changed"` {
		t.Fatal("store field did not change")
	}
	for key, want := range original {
		if key != "title" && !bytes.Equal(actual[key], want) {
			t.Fatalf("source field %s changed: %s", key, actual[key])
		}
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("patch changed caller-owned retained source")
	}
	if !bytes.Contains(patched, []byte("900719925474099312345")) || !bytes.Contains(patched, []byte("1.234567890123456789")) {
		t.Fatal("unknown field number was converted through float64")
	}
}

func TestSourcePatchCannotRewriteIdentityOrInventFields(t *testing.T) {
	for _, tc := range []struct {
		kind, field string
		value       json.RawMessage
	}{
		{"document", "id", json.RawMessage(`"22222222-2222-4222-8222-222222222222"`)},
		{"document", "tenant", json.RawMessage(`"other"`)},
		{"document", "owner_sub", json.RawMessage(`"bob"`)},
		{"template_log", "actor_sub", json.RawMessage(`"bob"`)},
		{"script_run", "script_id", json.RawMessage(`"22222222-2222-4222-8222-222222222222"`)},
		{"query", "connection_id", json.RawMessage(`"22222222-2222-4222-8222-222222222222"`)},
		{"document", "new_field", json.RawMessage(`true`)},
		{"document", "title", json.RawMessage(`{`)},
	} {
		id, raw := partitionRow(t, tc.kind, nil)
		if out, err := PatchSourceRow(partitionScope, tc.kind, id, raw, map[string]json.RawMessage{tc.field: tc.value}); err == nil || out != nil {
			t.Fatalf("%s/%s altered source identity", tc.kind, tc.field)
		}
	}
}
