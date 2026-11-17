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

func TestPublicationTargetRejectsAmbiguousEndpoints(t *testing.T) {
	if !(PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "DEVICE"}).Valid() {
		t.Fatal("valid target rejected")
	}
	for _, target := range []PublicationTarget{{RoomID: "!", DeviceID: "DEVICE"}, {RoomID: "!a b", DeviceID: "DEVICE"}, {RoomID: "!room", DeviceID: ""}, {RoomID: "!room", DeviceID: "other\x00device"}, {RoomID: "!room", DeviceID: string([]byte{0xff})}, {RoomID: "!room", DeviceID: strings.Repeat("a", 256)}} {
		if target.Valid() {
			t.Fatal("ambiguous target accepted")
		}
	}
}

func TestPrivateRecordExactReadbackKeepsEveryFieldAndOriginalNumericBytes(t *testing.T) {
	for _, product := range Products() {
		t.Run(product, func(t *testing.T) {
			original := Record{Version: 1, Scope: Scope{Tenant: "owner-org", Subject: "person", Product: product}, ObjectID: "object", RevisionID: "revision", Role: "content", Previous: []string{"$first", "$second"}, Payload: []byte(`{"integer":900719925474099312345,"name":"private"}`)}
			if !original.MatchesExact(original) {
				t.Fatal("exact readback rejected")
			}
			for _, fault := range []string{"version", "tenant", "subject", "product", "object", "revision", "role", "bytes", "rounded-number", "previous", "previous-order", "missing-payload"} {
				t.Run(fault, func(t *testing.T) {
					actual := original
					switch fault {
					case "version":
						actual.Version = 2
					case "tenant":
						actual.Scope.Tenant = "foreign"
					case "subject":
						actual.Scope.Subject = "foreign"
					case "product":
						if product == "drive" {
							actual.Scope.Product = "mail"
						} else {
							actual.Scope.Product = "drive"
						}
					case "object":
						actual.ObjectID = "other"
					case "revision":
						actual.RevisionID = "other"
					case "role":
						actual.Role = "index"
					case "bytes":
						actual.Payload = []byte(`{ "integer":900719925474099312345,"name":"private"}`)
					case "rounded-number":
						actual.Payload = []byte(`{"integer":900719925474099300000,"name":"private"}`)
					case "previous":
						actual.Previous = []string{"$first"}
					case "previous-order":
						actual.Previous = []string{"$second", "$first"}
					case "missing-payload":
						actual.Payload = nil
					}
					if original.MatchesExact(actual) || actual.MatchesExact(original) {
						t.Fatal("substituted logical record accepted", fault)
					}
				})
			}
		})
	}
	if (Record{}).MatchesExact(Record{}) {
		t.Fatal("two invalid empty records matched")
	}
}
