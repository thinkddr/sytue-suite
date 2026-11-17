// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ErrEncryptedBackupReview preserves an unsafe legacy archive for recovery on
// an authorized device. No key or plaintext is supplied to the server.
var ErrEncryptedBackupReview = fmt.Errorf("notes: encrypted backup requires local recovery before transfer: %w", ErrEncryptedMigrationRequired)

// ValidateEncryptedBackup rejects cases where the existing exporter/importer
// would expose legacy fields or silently discard encrypted-notebook structure.
// A clean envelope is not authenticated here: only a client with the key can
// verify it. Ordinary clear notebooks retain their existing behavior.
func ValidateEncryptedBackup(nb Notebook, notes []Note) error {
	if !nb.Encrypted {
		for _, note := range notes {
			if note.ContentCipher != "" {
				return ErrEncryptedBackupReview
			}
		}
		return nil
	}
	if nb.Crypto == nil || nb.Crypto.Validate() != nil || nb.ParentID != nil || nb.RoomID != "" || nb.DailyDate != "" || nb.ArchivedAt != nil {
		return ErrEncryptedBackupReview
	}
	for _, note := range notes {
		if err := validateEncryptedBackupNote(note); err != nil {
			return err
		}
	}
	return nil
}

func validateEncryptedBackupNote(note Note) error {
	if note.ContentCipher == "" || note.Title != "" || note.Content != "" || note.DataJSON != "" ||
		len(note.Links) != 0 || len(note.States) != 0 || note.GroupID != nil || note.RepeatedInto != nil {
		return ErrEncryptedBackupReview
	}
	data := note.Data
	if len(data) == 0 || string(data) == "null" {
		data = json.RawMessage(`{}`)
	}
	if !EncryptedPublicData(data, false) {
		return ErrEncryptedBackupReview
	}
	input := note.AImportar()
	if err := input.Normalize(true); err != nil {
		return ErrEncryptedBackupReview
	}
	return nil
}

// LeerNativoTransferencia ensures that the local decision and the server see
// the same unambiguous archive. Encrypted archives reject unknown fields, which
// the server would otherwise silently discard after receiving their contents.
func LeerNativoTransferencia(raw []byte) (Notebook, []Note, error) {
	if err := ValidarJSONSinClavesDuplicadas(raw, backupEsStruct); err != nil {
		return Notebook{}, nil, ErrFormatoNoReconocido
	}
	book, notes, err := LeerNativo(raw)
	if err != nil {
		return Notebook{}, nil, err
	}
	if book.Encrypted {
		strict := json.NewDecoder(bytes.NewReader(raw))
		strict.DisallowUnknownFields()
		var doc exportNativo
		if err := strict.Decode(&doc); err != nil {
			return Notebook{}, nil, ErrEncryptedBackupReview
		}
	}
	if err := ValidateEncryptedBackup(book, notes); err != nil {
		return Notebook{}, nil, err
	}
	return book, notes, nil
}

// Apply Go's case-insensitive struct field matching only to the archive's
// structs, never to keys in the user's arbitrary Data map.
func backupEsStruct(path []string) bool {
	return len(path) == 0 ||
		len(path) == 1 && strings.EqualFold(path[0], "notebook") ||
		len(path) == 2 && strings.EqualFold(path[0], "notebook") && strings.EqualFold(path[1], "crypto") ||
		len(path) == 2 && strings.EqualFold(path[0], "notas") && path[1] == "*"
}

// A readable format that omits encrypted content cannot be a successful
// export; HTML and .onex retain the ciphertext for later recovery.
func ValidateEncryptedBackupFormat(nb Notebook, format FormatoExport) error {
	if nb.Encrypted && format != FormatoHTML && format != FormatoNativo {
		return ErrEncryptedBackupReview
	}
	return nil
}

// ValidateBackupUpload keeps encrypted archives on the authorized device until
// a private importer can verify, rebind and reseal them locally.
func ValidateBackupUpload(nb Notebook, notes []Note) error {
	if nb.Encrypted {
		return ErrEncryptedBackupReview
	}
	return ValidateEncryptedBackup(nb, notes)
}
