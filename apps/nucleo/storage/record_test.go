// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrivateRecordRejectsAmbiguousOrOversizedContent(t *testing.T) {
	scope := Scope{Tenant: "org", Subject: "person", Product: "drive"}
	record := Record{Version: 1, Scope: scope, ObjectID: "object_1", RevisionID: "revision-1", Role: "manifest", Payload: json.RawMessage(`{"name":"private filename"}`)}
	if err := record.Validate(scope); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Record){
		func(r *Record) { r.Version = 2 },
		func(r *Record) { r.Scope.Tenant = "other" },
		func(r *Record) { r.Scope.Subject = "other" },
		func(r *Record) { r.Scope.Product = "mail" },
		func(r *Record) { r.ObjectID = "../secret" },
		func(r *Record) { r.RevisionID = "" },
		func(r *Record) { r.Role = "server-ai" },
		func(r *Record) { r.Payload = json.RawMessage(`{`) },
		func(r *Record) { r.Payload = []byte{'"', 0xff, '"'} },
		func(r *Record) { r.Payload = json.RawMessage(`"` + strings.Repeat("x", MaxRecordBytes) + `"`) },
		func(r *Record) { r.Previous = []string{"$one", "$one"} },
		func(r *Record) { r.Previous = []string{"$bad reference"} },
	} {
		bad := record
		change(&bad)
		if bad.Validate(scope) == nil {
			t.Fatal("invalid encrypted record accepted")
		}
	}
}

func TestPrivateScopeRejectsNormalizationAndTupleAmbiguity(t *testing.T) {
	for _, scope := range []Scope{
		{Tenant: "org\x00other", Subject: "person", Product: "drive"},
		{Tenant: "org", Subject: "person\x00other", Product: "drive"},
		{Tenant: "org", Subject: string([]byte{0xff}), Product: "drive"},
		{Tenant: strings.Repeat("a", 64), Subject: "person", Product: "drive"},
	} {
		if scope.Valid() {
			t.Fatal("ambiguous private scope accepted")
		}
	}
}
