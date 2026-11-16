// SPDX-License-Identifier: Apache-2.0
package model

import (
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestSourceIdentityUsesExactCompositePrimaryKey(t *testing.T) {
	a, err := SourceObjectID("operation", map[string]json.RawMessage{"document_id": json.RawMessage(`"d"`), "seq": json.RawMessage(`9007199254740993`)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := SourceObjectID("operation", map[string]json.RawMessage{"seq": json.RawMessage(`9007199254740993`), "document_id": json.RawMessage(`"d"`)})
	if err != nil || a != b {
		t.Fatal("primary key order changed identity", err)
	}
	c, err := SourceObjectID("operation", map[string]json.RawMessage{"document_id": json.RawMessage(`"d"`), "seq": json.RawMessage(`9007199254740992`)})
	if err != nil || a == c {
		t.Fatal("primary key precision lost", err)
	}
	d, err := SourceObjectID("permission", map[string]json.RawMessage{"document_id": json.RawMessage(`"d"`), "principal_kind": json.RawMessage(`"user"`), "principal_id": json.RawMessage(`"p"`)})
	if err != nil || a == d {
		t.Fatal("source kinds share identity", err)
	}
	if _, err := SourceObjectID("unknown", map[string]json.RawMessage{"id": json.RawMessage(`"d"`)}); err == nil {
		t.Fatal("unknown source admitted")
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "docs"}
	if ValidateSourceRow(scope, "document", a, []byte(`{"tenant":"acme","owner_sub":"bob","kind":"document"}`)) == nil {
		t.Fatal("other owner document admitted")
	}
	if ValidateSourceRow(scope, "document", a, []byte(`{"tenant":"acme","owner_sub":"alice","kind":"spreadsheet"}`)) == nil {
		t.Fatal("other product document admitted")
	}
}
