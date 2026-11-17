// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"maunium.net/go/mautrix/crypto/ssss"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func artificialOwnerRecord() (string, storage.Record) {
	return "!privée:matrix.test", storage.Record{Version: 1, Scope: storage.Scope{Tenant: "équipe", Subject: "person<>&💻", Product: "drive"}, ObjectID: "object", RevisionID: "revision", Role: "content", Previous: []string{"$old-é:matrix.test", "$old-two:matrix.test"}, Payload: []byte(`{"text":"private <filename> 💻","integer":900719925474099312345,"fraction":1.000000000000000001}`)}
}

func TestStorageOwnerProofUsesMaintainedSecretStorageAndBindsEveryField(t *testing.T) {
	room, record := artificialOwnerRecord()
	key, err := ssss.NewKey("")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key.Key)
	proof, err := sealStorageOwnerRecord(room, record, key)
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyStorageOwnerRecord(room, record, proof, key); err != nil {
		t.Fatal(err)
	}
	again, err := sealStorageOwnerRecord(room, record, key)
	if err != nil || again.IV == proof.IV || again.Ciphertext == proof.Ciphertext {
		t.Fatal("SSSS did not randomize the owner proof", err)
	}
	changes := map[string]func(*storage.Record){
		"tenant":       func(r *storage.Record) { r.Scope.Tenant = "other" },
		"subject":      func(r *storage.Record) { r.Scope.Subject = "other" },
		"product":      func(r *storage.Record) { r.Scope.Product = "mail" },
		"object":       func(r *storage.Record) { r.ObjectID = "other" },
		"revision":     func(r *storage.Record) { r.RevisionID = "other" },
		"role":         func(r *storage.Record) { r.Role = "index" },
		"parents":      func(r *storage.Record) { r.Previous = []string{"$foreign:matrix.test"} },
		"parent-order": func(r *storage.Record) { r.Previous = []string{record.Previous[1], record.Previous[0]} },
		"payload":      func(r *storage.Record) { r.Payload = []byte(`{"integer":900719925474099312346}`) },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			other := record
			change(&other)
			if !errors.Is(verifyStorageOwnerRecord(room, other, proof, key), ErrUnverified) {
				t.Fatal("owner proof accepted changed application data")
			}
		})
	}
	if !errors.Is(verifyStorageOwnerRecord("!foreign:matrix.test", record, proof, key), ErrUnverified) {
		t.Fatal("owner proof crossed rooms")
	}
	encoded, _ := json.Marshal(proof)
	if len(encoded) > 512 || bytes.Contains(encoded, record.Payload) {
		t.Fatal("proof leaks content or exceeds the event budget")
	}
}

func TestStorageOwnerProofRejectsMalformedOrForgedSecretStorageData(t *testing.T) {
	room, record := artificialOwnerRecord()
	key, err := ssss.NewKey("")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key.Key)
	proof, err := sealStorageOwnerRecord(room, record, key)
	if err != nil {
		t.Fatal(err)
	}
	mutate := map[string]func(*storageOwnerProof){
		"version":      func(p *storageOwnerProof) { p.Version = 2 },
		"algorithm":    func(p *storageOwnerProof) { p.Algorithm = "other" },
		"key-id":       func(p *storageOwnerProof) { p.KeyID = "other" },
		"iv-size":      func(p *storageOwnerProof) { p.IV = "AA" },
		"iv-canonical": func(p *storageOwnerProof) { p.IV += "====" },
		"iv": func(p *storageOwnerProof) {
			iv, _ := base64.RawStdEncoding.DecodeString(p.IV)
			iv[0] ^= 1
			p.IV = base64.RawStdEncoding.EncodeToString(iv)
		},
		"mac":    func(p *storageOwnerProof) { p.MAC = base64.StdEncoding.EncodeToString(make([]byte, 32)) },
		"cipher": func(p *storageOwnerProof) { p.Ciphertext = base64.RawStdEncoding.EncodeToString(make([]byte, 86)) },
	}
	for name, change := range mutate {
		t.Run(name, func(t *testing.T) {
			other := proof
			change(&other)
			if !errors.Is(verifyStorageOwnerRecord(room, record, other, key), ErrUnverified) {
				t.Fatal("invalid owner proof accepted")
			}
		})
	}
	foreign, err := ssss.NewKey("")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(foreign.Key)
	foreign.ID = key.ID
	if !errors.Is(verifyStorageOwnerRecord(room, record, proof, foreign), ErrUnverified) {
		t.Fatal("another secret authenticated the owner")
	}
}
