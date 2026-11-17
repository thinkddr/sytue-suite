// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestPrivateNormalizersPreserveCallerOwnedInputsForRetry(t *testing.T) {
	data := json.RawMessage(`{ "integer": 900719925474099312345, "future": { "nested": true }, "scientific": 1.234567890123456789e+99 }`)
	expected := time.Date(2026, 10, 30, 12, 0, 0, 123456000, time.UTC)
	in := api.NoteInput{Kind: "text", Title: "  normal title  ", Data: data, ExpectedUpdatedAt: &expected}
	want := bytes.Clone(data)
	first, err := normalizeNoteInput(in, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(in.Data, want) || !json.Valid(in.Data) {
		t.Fatal("normalization overwrote the caller's original JSON")
	}
	second, err := normalizeNoteInput(in, false)
	if err != nil || !bytes.Equal(first.Data, second.Data) || first.ExpectedUpdatedAt != &expected || second.ExpectedUpdatedAt != &expected {
		t.Fatal("normalization changed a retained retry", err)
	}
	if in.Title != "  normal title  " || first.Title != "normal title" {
		t.Fatal("normalization did not retain the original value separately")
	}
}

func TestPrivateNotebookNormalizerPreservesCallerCryptoBeforeRetry(t *testing.T) {
	params := json.RawMessage(`{ "v": 1, "kdf": "PBKDF2-SHA256", "salt": "abcdefghijklmnop12345678", "iterations": 600000, "check": "encv1:fixture", "future": { "integer": 900719925474099312345 } }`)
	want := bytes.Clone(params)
	in := api.NotebookInput{Title: " private ", Encrypted: true, Crypto: params}
	for range 2 {
		normalized, err := normalizeNotebookInput(in)
		if err != nil || !bytes.Equal(in.Crypto, want) || !bytes.Equal(normalized.Crypto, want) {
			t.Fatal("normalization overwrote caller encryption parameters or their unknown extensions", err)
		}
	}
}
