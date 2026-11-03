// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestIndependentHybridFragmentsResumeAcceptedPrefixWithoutChangingRecord(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	room := "!storage:test"
	record := storage.Record{Version: 1, Scope: scope, ObjectID: "object", RevisionID: "revision", Role: "content", Payload: []byte("{ \"precise\":900719925474099312345,\"data\":\"" + strings.Repeat("a", 20<<10) + "\" }")}
	raw, err := EncodeStorageRecordPayload(record)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	repeated, err := EncodeStorageRecordPayload(record)
	if err != nil || !bytes.Equal(repeated, raw) {
		t.Fatal("record framing was not deterministic", err)
	}
	clear(repeated)
	digest, err := PayloadDigest(raw)
	if err != nil {
		t.Fatal(err)
	}
	count := (len(raw) + StorageFragmentBytes - 1) / StorageFragmentBytes
	if count < 2 {
		t.Fatal("accepted-prefix recovery was not exercised")
	}
	accepted := make([]Envelope, count)
	root := FragmentRoot{Version: 1, SHA256: digest, SizeBytes: len(raw), Proof: json.RawMessage("{\"owner\":\"opaque AES256 proof\"}")}
	for index := 0; index < count; index++ {
		first := index * StorageFragmentBytes
		part := raw[first:min(first+StorageFragmentBytes, len(raw))]
		// Part zero belongs to the first attempt. Remaining independent age streams
		// can be created after restart; their random nonces do not change this input.
		accepted[index], err = SealStorageFragment(identity.Recipient(), scope, room, digest, index, count, part)
		if err != nil {
			t.Fatal(err)
		}
		root.References = append(root.References, storage.Reference{RoomID: room, EventID: "$part_" + string(rune('a'+index))})
	}
	rootEnvelope, err := SealStorageFragmentRoot(identity.Recipient(), scope, room, root)
	if err != nil {
		t.Fatal(err)
	}
	openedRoot, err := OpenStorageFragmentRoot(identity, scope, room, rootEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	parts := make([][]byte, count)
	defer func() {
		for _, part := range parts {
			clear(part)
		}
	}()
	for index := range accepted {
		parts[index], err = OpenStorageFragment(identity, scope, room, digest, index, count, accepted[index])
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := AssembleStorageFragments(openedRoot, room, parts)
	if err != nil || !bytes.Equal(result, raw) {
		t.Fatal("independent fragment set changed the accepted source", err)
	}
	decoded, err := DecodeStorageRecordPayload(scope, result)
	clear(result)
	if err != nil || !bytes.Equal(decoded.Payload, record.Payload) {
		t.Fatal("precise original JSON changed", err)
	}
	clear(decoded.Payload)
	for _, fault := range []string{"missing", "reordered", "truncated", "changed", "digest", "room", "duplicate-reference"} {
		t.Run(fault, func(t *testing.T) {
			altered := make([][]byte, len(parts))
			for i, part := range parts {
				altered[i] = bytes.Clone(part)
			}
			defer func() {
				for _, part := range altered {
					clear(part)
				}
			}()
			changedRoot := openedRoot
			changedRoot.References = append([]storage.Reference(nil), openedRoot.References...)
			where := room
			switch fault {
			case "missing":
				altered = altered[:len(altered)-1]
			case "reordered":
				altered[0], altered[1] = altered[1], altered[0]
			case "truncated":
				altered[0] = altered[0][:len(altered[0])-1]
			case "changed":
				altered[0][0] ^= 1
			case "digest":
				changedRoot.SHA256 = strings.Repeat("0", 64)
			case "room":
				where = "!other:test"
			case "duplicate-reference":
				changedRoot.References[1] = changedRoot.References[0]
			}
			result, err := AssembleStorageFragments(changedRoot, where, altered)
			if err == nil || len(result) != 0 {
				t.Fatal("invalid partial set released bytes", fault, err)
			}
		})
	}
	if part, err := OpenStorageFragment(identity, scope, room, digest, 1, count, accepted[0]); err == nil || len(part) != 0 {
		t.Fatal("fragment moved to another ordinal")
	}
	other := scope
	other.Product = "mail"
	if part, err := OpenStorageFragment(identity, other, room, digest, 0, count, accepted[0]); err == nil || len(part) != 0 {
		t.Fatal("fragment moved to another application")
	}
}
