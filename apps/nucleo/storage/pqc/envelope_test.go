// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"filippo.io/age"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestHybridEnvelopeAuthenticatesEntireStreamContextAndProfile(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	context := []byte("typed scope and room v1")
	original := []byte("private body with exact bytes \x00 / 漢字")
	encrypted, err := Seal(identity.Recipient(), context, original)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := Open(identity, context, encrypted)
	if err != nil || !bytes.Equal(opened, original) {
		t.Fatal("hybrid envelope did not roundtrip", err)
	}
	clear(opened)
	other, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	classical, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := ParseIdentity(classical.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"context", "key", "classical", "profile", "key-id", "truncated", "tampered", "extended"} {
		t.Run(fault, func(t *testing.T) {
			value := encrypted
			value.Ciphertext = bytes.Clone(encrypted.Ciphertext)
			key := identity
			scope := bytes.Clone(context)
			switch fault {
			case "context":
				scope = append(scope, 'x')
			case "key":
				key = other
			case "classical":
				key = legacy
			case "profile":
				value.Profile = LegacyProfile
			case "key-id":
				value.KeyID = strings.Repeat("0", 64)
			case "truncated":
				value.Ciphertext = value.Ciphertext[:len(value.Ciphertext)-1]
			case "tampered":
				value.Ciphertext[len(value.Ciphertext)-1] ^= 1
			case "extended":
				value.Ciphertext = append(value.Ciphertext, 0)
			}
			raw, err := Open(key, scope, value)
			if err == nil || len(raw) != 0 {
				t.Fatal("invalid envelope released private bytes", fault, err)
			}
		})
	}
}

func TestHybridStorageRecordPreservesExactJSONAndMaximumWireRecord(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	record := storage.Record{Version: 1, Scope: scope, ObjectID: "private_object", RevisionID: "revision", Role: "content", Payload: []byte("{ \"integer\":900719925474099312345, \"padding\":\"x\" }")}
	proof := []byte("{ \"owner_proof\":\"opaque SDK AES256 attestation\" }")
	verify := func(record storage.Record) {
		t.Helper()
		encrypted, err := SealStorageRecord(identity.Recipient(), scope, "!storage:test", record, proof)
		if err != nil {
			t.Fatal(err)
		}
		opened, owner, err := OpenStorageRecord(identity, scope, "!storage:test", encrypted)
		if err != nil || !bytes.Equal(opened.Payload, record.Payload) || opened.ObjectID != record.ObjectID || opened.RevisionID != record.RevisionID || opened.Role != record.Role || len(owner) == 0 {
			t.Fatal("hybrid record changed exact private data", err)
		}
		clear(opened.Payload)
		clear(owner)
		foreign := scope
		foreign.Product = "mail"
		if raw, _, err := OpenStorageRecord(identity, foreign, "!storage:test", encrypted); err == nil || len(raw.Payload) != 0 {
			t.Fatal("moved record scope was accepted")
		}
		if raw, _, err := OpenStorageRecord(identity, scope, "!other:test", encrypted); err == nil || len(raw.Payload) != 0 {
			t.Fatal("moved record room was accepted")
		}
	}
	verify(record)
	random := make([]byte, 40<<10)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	padding := base64.RawStdEncoding.EncodeToString(random)
	fits := 0
	for low, high := 1, len(padding); low <= high; {
		n := low + (high-low)/2
		record.Payload = []byte("{\"high_entropy\":\"" + padding[:n] + "\"}")
		if record.Validate(scope) != nil {
			high = n - 1
		} else {
			fits = n
			low = n + 1
		}
	}
	record.Payload = []byte("{\"high_entropy\":\"" + padding[:fits] + "\"}")
	raw, err := json.Marshal(record)
	if err != nil || len(raw) < storage.MaxRecordBytes-4 || record.Validate(scope) != nil {
		t.Fatal("maximum wire vector was not exercised", len(raw), err)
	}
	verify(record)
}
