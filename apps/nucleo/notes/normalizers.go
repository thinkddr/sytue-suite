// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

func normalizeNotebookInput(input api.NotebookInput) (api.NotebookInput, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return input, err
	}
	var checked model.NotebookInput
	if err = json.Unmarshal(encoded, &checked); err != nil {
		return input, ErrSource
	}
	if err := checked.Normalize(); err != nil {
		return input, err
	}
	encoded, err = json.Marshal(checked)
	if err != nil {
		return input, err
	}
	// Unmarshal into a fresh value: RawMessage, pointers and slices in input
	// belong to the caller and may be retained for a durable retry.
	var normalized api.NotebookInput
	err = json.Unmarshal(encoded, &normalized)
	if normalized.Encrypted {
		normalized.Crypto = input.Crypto
	}
	normalized.ExpectedUpdatedAt = input.ExpectedUpdatedAt
	return normalized, err
}

func normalizeNoteInput(input api.NoteInput, legacyEncrypted bool) (api.NoteInput, error) {
	if input.Visibility == "inherit" {
		input.Visibility = ""
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return input, err
	}
	var checked model.NoteInput
	if err = json.Unmarshal(encoded, &checked); err != nil {
		return input, ErrSource
	}
	if err := checked.Normalize(legacyEncrypted); err != nil {
		return input, err
	}
	encoded, err = json.Marshal(checked)
	if err != nil {
		return input, err
	}
	var normalized api.NoteInput
	err = json.Unmarshal(encoded, &normalized)
	normalized.ExpectedUpdatedAt = input.ExpectedUpdatedAt
	return normalized, err
}
