// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/olm"
	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type storageOwnerUnicodeVector struct {
	Room   string            `json:"room"`
	Record storage.Record    `json:"record"`
	Digest string            `json:"digest"`
	Proof  storageOwnerProof `json:"proof"`
}

type storageInteropVector struct {
	OwnerKey     []byte                     `json:"owner_key"`
	OwnerKeyID   string                     `json:"owner_key_id"`
	OwnerProofs  []storageOwnerProof        `json:"owner_proofs"`
	OwnerUnicode storageOwnerUnicodeVector  `json:"owner_unicode"`
	Artificial   bool                       `json:"artificial"`
	Generator    string                     `json:"generator"`
	Scope        storage.Scope              `json:"scope"`
	Entries      []storage.RecordEntry      `json:"entries"`
	Sessions     []mxcrypto.ExportedSession `json:"sessions"`
	Events       []*event.Event             `json:"events"`
	Heads        []string                   `json:"heads"`
}

// Opt-in regeneration exports ONLY disposable keys made by this test. These
// vectors prove wire/SDK interoperability, never sender trust or live recovery.
func TestStorageGoVectorBridge(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "person", Product: "drive"}
	room := id.RoomID("!private:matrix.test")
	own := id.UserID("@person:matrix.test")
	account := mxcrypto.NewOlmAccount()
	outbound, err := olm.NewOutboundGroupSession()
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := olm.NewInboundGroupSession([]byte(outbound.Key()))
	if err != nil {
		t.Fatal(err)
	}
	exported, err := incoming.Export(incoming.FirstKnownIndex())
	if err != nil {
		t.Fatal(err)
	}
	defer clear(exported)
	vector := storageInteropVector{
		Artificial: true, Generator: "mautrix-go v0.32.0/goolm; ARTIFICIAL PRIVATE STORAGE ONLY", Scope: scope,
		Sessions: []mxcrypto.ExportedSession{{Algorithm: id.AlgorithmMegolmV1, RoomID: room, SenderKey: account.IdentityKey(), SenderClaimedKeys: mxcrypto.SenderClaimedKeys{Ed25519: account.SigningKey()}, SessionID: outbound.ID(), SessionKey: string(exported), ForwardingChains: &[]string{}}}, Heads: []string{"$deleted", "$edit"},
	}
	metadata := []byte(`{"title":"private <filename>","amount":900719925474099312345,"fraction":1.00000000000000000001}`)
	content := storage.Record{Version: 1, Scope: scope, ObjectID: "object", RevisionID: "content", Role: "content", Payload: bytes.Clone(metadata)}
	vector.Entries = append(vector.Entries, storage.RecordEntry{Reference: storage.Reference{RoomID: string(room), EventID: "$content"}, Record: content})
	for _, revision := range []string{"base", "edit", "deleted"} {
		commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: bytes.Clone(metadata), Content: &storage.ContentPointer{Kind: "record", RevisionID: "content", Reference: vector.Entries[0].Reference}}
		var parents []string
		if revision != "base" {
			commit.Parents = []storage.Reference{{RoomID: string(room), EventID: "$base"}}
			parents = []string{"$base"}
		}
		if revision == "deleted" {
			commit.State = "deleted"
			commit.Content = nil
		}
		payload, err := json.Marshal(commit)
		if err != nil {
			t.Fatal(err)
		}
		rec := storage.Record{Version: 1, Scope: scope, ObjectID: "object", RevisionID: revision, Role: "index", Previous: parents, Payload: payload}
		vector.Entries = append(vector.Entries, storage.RecordEntry{Reference: storage.Reference{RoomID: string(room), EventID: "$" + revision}, Record: rec})
	}
	ownerKey, err := ssss.NewKey("")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(ownerKey.Key)
	vector.OwnerKey, vector.OwnerKeyID = bytes.Clone(ownerKey.Key), ownerKey.ID
	unicodeRoom, unicodeRecord := artificialOwnerRecord()
	unicodeProof, err := sealStorageOwnerRecord(unicodeRoom, unicodeRecord, ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	unicodeDigest, err := storage.RecordDigest(unicodeRoom, unicodeRecord)
	if err != nil {
		t.Fatal(err)
	}
	vector.OwnerUnicode = storageOwnerUnicodeVector{Room: unicodeRoom, Record: unicodeRecord, Digest: unicodeDigest, Proof: unicodeProof}
	for _, entry := range vector.Entries {
		if entry.Record.Validate(scope) != nil {
			t.Fatal("invalid native record")
		}
		raw, err := json.Marshal(map[string]any{"room_id": room, "type": event.EventMessage.Type, "content": map[string]any{"msgtype": "m.text", "body": "", "org.sytue.storage": entry.Record}})
		if err != nil {
			t.Fatal(err)
		}
		cipher, err := outbound.Encrypt(raw)
		if err != nil {
			t.Fatal(err)
		}
		proof, err := sealStorageOwnerRecord(entry.Reference.RoomID, entry.Record, ownerKey)
		if err != nil {
			t.Fatal(err)
		}
		vector.OwnerProofs = append(vector.OwnerProofs, proof)
		vector.Events = append(vector.Events, &event.Event{ID: id.EventID(entry.Reference.EventID), RoomID: room, Sender: own, Type: event.EventEncrypted, Timestamp: 1792152000000, Content: event.Content{Parsed: &event.EncryptedEventContent{Algorithm: id.AlgorithmMegolmV1, SenderKey: account.IdentityKey(), SessionID: outbound.ID(), MegolmCiphertext: cipher}}})
	}
	if output := os.Getenv("SYTUE_STORAGE_GO_VECTOR_OUT"); output != "" {
		encoded, err := json.MarshalIndent(vector, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, append(encoded, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	verifyStorageVector(t, vector)
}

func TestStorageRustVectorDecodesWithoutRoundingPrivateRecords(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "fixtures", "storage-matrix-js.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vector storageInteropVector
	if json.Unmarshal(raw, &vector) != nil || !vector.Artificial || len(vector.Events) < 4 || len(vector.Events) != len(vector.Entries) || len(vector.Sessions) == 0 {
		t.Fatal("missing independent artificial storage vector")
	}
	verifyStorageVector(t, vector)
}

func verifyStorageVector(t *testing.T, vector storageInteropVector) {
	t.Helper()
	if len(vector.OwnerKey) != 32 || !ownerKeyID(vector.OwnerKeyID) || len(vector.OwnerProofs) != len(vector.Entries) {
		t.Fatal("missing artificial independent SSSS owner proofs")
	}
	owner := &ssss.Key{ID: vector.OwnerKeyID, Key: vector.OwnerKey}
	for i, entry := range vector.Entries {
		if err := verifyStorageOwnerRecord(entry.Reference.RoomID, entry.Record, vector.OwnerProofs[i], owner); err != nil {
			t.Fatal("independent SDK owner proof failed", err)
		}
	}
	unicodeDigest, err := storage.RecordDigest(vector.OwnerUnicode.Room, vector.OwnerUnicode.Record)
	if err != nil || unicodeDigest != vector.OwnerUnicode.Digest {
		t.Fatal("cross-SDK Unicode checksum changed", err)
	}
	if err := verifyStorageOwnerRecord(vector.OwnerUnicode.Room, vector.OwnerUnicode.Record, vector.OwnerUnicode.Proof, owner); err != nil {
		t.Fatal("cross-SDK Unicode proof failed", err)
	}
	var indexes []storage.RecordEntry
	for i, encrypted := range vector.Events {
		if encrypted.Type != event.EventEncrypted {
			t.Fatal("plaintext storage vector")
		}
		if encrypted.Content.Parsed == nil {
			if err := encrypted.Content.ParseRaw(encrypted.Type); err != nil {
				t.Fatal(err)
			}
		}
		content := encrypted.Content.AsEncrypted()
		var cleartext []byte
		for _, exported := range vector.Sessions {
			if exported.SessionID != content.SessionID {
				continue
			}
			if exported.RoomID != encrypted.RoomID || exported.SenderKey != content.SenderKey {
				t.Fatal("foreign storage session")
			}
			incoming, err := olm.InboundGroupSessionImport([]byte(exported.SessionKey))
			if err != nil {
				t.Fatal(err)
			}
			cleartext, _, err = incoming.Decrypt(content.MegolmCiphertext)
			if err != nil {
				t.Fatal(err)
			}
			corrupted := bytes.Clone(content.MegolmCiphertext)
			corrupted[len(corrupted)/2] ^= 1
			if _, _, err = incoming.Decrypt(corrupted); err == nil {
				t.Fatal("corrupted SDK ciphertext accepted")
			}
			break
		}
		var envelope struct {
			Type    string          `json:"type"`
			RoomID  string          `json:"room_id"`
			Content json.RawMessage `json:"content"`
		}
		if len(cleartext) == 0 || json.Unmarshal(cleartext, &envelope) != nil || envelope.RoomID != string(encrypted.RoomID) {
			t.Fatal("storage envelope lost room")
		}
		entry := vector.Entries[i]
		rec, err := decodeStorageRecord(DecryptedEvent{Sender: encrypted.Sender.String(), Type: envelope.Type, Content: envelope.Content}, vector.Scope, encrypted.Sender.String(), entry.Record.ObjectID, entry.Record.Role)
		if err != nil || rec.RevisionID != entry.Record.RevisionID || !bytes.Equal(rec.Payload, entry.Record.Payload) || !slices.Equal(rec.Previous, entry.Record.Previous) {
			t.Fatal("cross-SDK private bytes changed", err)
		}
		if rec.Role == "index" {
			indexes = append(indexes, storage.RecordEntry{Reference: entry.Reference, Record: rec})
		}
	}
	heads, err := storage.CatalogHeads(vector.Scope, indexes)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(heads))
	for i, head := range heads {
		ids[i] = head.Reference.EventID
	}
	if !slices.Equal(ids, vector.Heads) {
		t.Fatal("cross-SDK conflicts changed", ids)
	}
}
