// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	EncryptedNoteWriteVersion = 2
	// Includes the existing 5 MiB Data limit and JSON/base64 expansion in both clients.
	MaxEncryptedNoteCipher       = 16 << 20
	MaxNoteRequestBody     int64 = 24 << 20
)

var (
	ErrBackendWriteUpgrade        = errors.New("notes: the backend does not support the database write contract")
	ErrEncryptedClientUpgrade     = errors.New("notes: encrypted writes require a client that seals note data")
	ErrEncryptedMigrationRequired = errors.New("notes: legacy content must be sealed with a checked migration")
	ErrNoteChanged                = errors.New("notes: the note changed before encrypted migration")
	ErrNotebookCryptoChanged      = errors.New("notes: notebook encryption parameters changed")
)

// EncryptedPublicData admits only the opaque capture ID in client requests.
// The other keys are existing service-maintained task controls; clients cannot set them.
func EncryptedPublicData(data json.RawMessage, serverControls bool) bool {
	var values map[string]json.RawMessage
	if json.Unmarshal(data, &values) != nil || values == nil {
		return false
	}
	for key := range values {
		if key == "client_id" {
			continue
		}
		if serverControls && (key == "repeated_into" || key == "carried_from" || key == "carry_count") {
			continue
		}
		return false
	}
	return true
}

// EncryptedContentInput replaces only the content under a note and notebook CAS.
// Encryption happens in the client; ExpectedCrypto contains only public KDF parameters.
type EncryptedContentInput struct {
	CipherVersion     int             `json:"cipher_version"`
	ExpectedUpdatedAt time.Time       `json:"expected_updated_at"`
	ExpectedCrypto    json.RawMessage `json:"expected_crypto"`
	ContentCipher     string          `json:"content_cipher"`
	Data              json.RawMessage `json:"data"`
}

func (in *EncryptedContentInput) Normalize() error {
	if in.ExpectedUpdatedAt.IsZero() || len(in.ExpectedCrypto) == 0 || len(in.ExpectedCrypto) > 4096 {
		return ErrInvalidInput
	}
	var crypto CryptoParams
	if json.Unmarshal(in.ExpectedCrypto, &crypto) != nil || crypto.Validate() != nil {
		return ErrInvalidInput
	}
	note := NoteInput{CipherVersion: in.CipherVersion, ContentCipher: in.ContentCipher, Data: in.Data}
	if err := note.Normalize(true); err != nil {
		return err
	}
	in.ContentCipher, in.Data = note.ContentCipher, note.Data
	return nil
}
