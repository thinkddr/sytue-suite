// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestEncodeJSONLimitChecksActualEncodedBytesWithoutEscapingHTML(t *testing.T) {
	value := map[string]string{"data_json": `{"raw":"<>&\"","number":9007199254740993}`, "separator": "\u2028"}
	full, err := EncodeJSONLimit(value, 1024)
	if err != nil || !json.Valid(full) {
		t.Fatalf("bounded encoding: %s %v", full, err)
	}
	exact, err := EncodeJSONLimit(value, len(full))
	if err != nil || !bytes.Equal(exact, full) {
		t.Fatal("exact byte boundary rejected valid JSON")
	}
	for _, limit := range []int{-1, 0, len(full) - 1} {
		got, err := EncodeJSONLimit(value, limit)
		if !errors.Is(err, ErrJSONResponseTooLarge) || len(got) != 0 {
			t.Fatalf("oversize response escaped bound %d: %d %v", limit, len(got), err)
		}
	}
	var decoded map[string]string
	if json.Unmarshal(full, &decoded) != nil || decoded["data_json"] != value["data_json"] {
		t.Fatal("raw JSON string changed during bounded encoding")
	}
}
