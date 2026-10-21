// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/olm"
	"maunium.net/go/mautrix/event"
)

// This consumes an independent RustCrypto/WASM producer fixture. An SDK echo
// test cannot prove web/native interoperability, so a missing vector is an error.
func TestRustCryptoMessagesAndAttachmentsDecryptInGoOlm(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "fixtures", "rooms-matrix-js.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Artificial bool                       `json:"artificial"`
		Generator  string                     `json:"generator"`
		Plaintext  string                     `json:"plaintext"`
		Sessions   []mxcrypto.ExportedSession `json:"sessions"`
		Events     []*event.Event             `json:"events"`
		Attachment struct {
			Descriptor event.EncryptedFileInfo `json:"descriptor"`
			Ciphertext string                  `json:"ciphertext"`
			Plaintext  string                  `json:"plaintext"`
		} `json:"attachment"`
	}
	if err = json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	if !vector.Artificial || len(vector.Sessions) == 0 || len(vector.Events) == 0 {
		t.Fatal("missing artificial independent SDK vector")
	}
	for _, encrypted := range vector.Events {
		if encrypted.Type != event.EventEncrypted {
			t.Fatal("interop vector is not encrypted")
		}
		if err = encrypted.Content.ParseRaw(encrypted.Type); err != nil {
			t.Fatal(err)
		}
		content := encrypted.Content.AsEncrypted()
		found := false
		for _, exported := range vector.Sessions {
			if exported.SessionID != content.SessionID {
				continue
			}
			if exported.RoomID != encrypted.RoomID || exported.SenderKey != content.SenderKey {
				t.Fatal("fixture identity mismatch")
			}
			session, importErr := olm.InboundGroupSessionImport([]byte(exported.SessionKey))
			if importErr != nil {
				t.Fatal(importErr)
			}
			cleartext, _, decryptErr := session.Decrypt(content.MegolmCiphertext)
			if decryptErr != nil {
				t.Fatal(decryptErr)
			}
			var message struct {
				RoomID  string                    `json:"room_id"`
				Type    string                    `json:"type"`
				Content event.MessageEventContent `json:"content"`
			}
			if err = json.Unmarshal(cleartext, &message); err != nil {
				t.Fatal(err)
			}
			if message.RoomID != encrypted.RoomID.String() || message.Type != event.EventMessage.Type || message.Content.Body != vector.Plaintext {
				t.Fatal("RustCrypto message changed through Go Olm")
			}
			found = true
		}
		if !found {
			t.Fatal("interop session is missing")
		}
	}
	ciphertext, err := base64.StdEncoding.DecodeString(vector.Attachment.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	want, err := base64.StdEncoding.DecodeString(vector.Attachment.Plaintext)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := bytes.Clone(ciphertext)
	corrupt[0] ^= 1
	if err = vector.Attachment.Descriptor.DecryptInPlace(corrupt); err == nil {
		t.Fatal("corrupted cross-SDK attachment accepted")
	}
	if err = vector.Attachment.Descriptor.DecryptInPlace(ciphertext); err != nil || !bytes.Equal(ciphertext, want) {
		t.Fatalf("RustCrypto attachment mismatch: %v", err)
	}
}
