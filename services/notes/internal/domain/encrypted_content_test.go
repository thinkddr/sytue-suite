// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestEncryptedWriteSealsDataAndRejectsOlderClients(t *testing.T) {
	for _, data := range []string{`{"attachments":[{"dataUrl":"private-image"}]}`, `{"url":"https://example.org/private"}`, `{"repeated_into":"client-forged"}`} {
		in := NoteInput{CipherVersion: EncryptedNoteWriteVersion, ContentCipher: "encv1:AAAA", Data: json.RawMessage(data)}
		if !errors.Is(in.Normalize(true), ErrInvalidInput) {
			t.Fatalf("private or service-owned Data was accepted outside the ciphertext: %s", data)
		}
	}
	old := NoteInput{ContentCipher: "encv1:AAAA"}
	if !errors.Is(old.Normalize(true), ErrEncryptedClientUpgrade) {
		t.Fatal("an older writer could overwrite an envelope containing sealed Data")
	}
	modern := NoteInput{CipherVersion: EncryptedNoteWriteVersion, ContentCipher: "encv1:" + strings.Repeat("A", 7<<20), Data: json.RawMessage(`{"client_id":"capture-1"}`)}
	if err := modern.Normalize(true); err != nil {
		t.Fatalf("a sealed attachment within the documented limit was rejected: %v", err)
	}
	modern.ContentCipher = "encv1:" + strings.Repeat("A", MaxEncryptedNoteCipher)
	if !errors.Is(modern.Normalize(true), ErrInvalidInput) {
		t.Fatal("ciphertext limit was not enforced")
	}
}

func TestEncryptedDataKeepsOnlyServiceTaskControls(t *testing.T) {
	controls := json.RawMessage(`{"client_id":"capture-1","repeated_into":"opaque-id","carried_from":"2026-10-20","carry_count":2}`)
	if !EncryptedPublicData(controls, true) || EncryptedPublicData(controls, false) {
		t.Fatal("service task controls were confused with client writable content")
	}
	for _, data := range []string{`null`, `[]`, `"text"`, `{"attachments":[]}`, `{"html":"private"}`} {
		if EncryptedPublicData(json.RawMessage(data), true) {
			t.Fatalf("private Data was mistaken for routing metadata: %s", data)
		}
	}
}
