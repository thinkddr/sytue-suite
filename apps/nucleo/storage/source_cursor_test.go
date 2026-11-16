// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSourceCursorCodecRejectsUnframedAndUnknownData(t *testing.T) {
	for _, cursor := range []LegacySourceCursor{{}, {Kind: "member", ID: "one"}, {Kind: "email", ID: "composite:💾:key"}} {
		encoded, err := EncodeSourceCursor(cursor)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := DecodeSourceCursor(encoded)
		if err != nil || actual != cursor {
			t.Fatal("cursor changed", err)
		}
	}
	for _, raw := range []string{`{}`, `null`, `{"kind":"card"}`, `{"kind":"card","id":"x","unknown":true}`, `{"kind":"card","id":"x"} {}`, `{"kind":"card","id":"bad\nkey"}`, `{"kind":"card","id":"x"}` + string([]byte{255})} {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(raw))
		if _, err := DecodeSourceCursor(encoded); err == nil {
			t.Fatal("accepted invalid cursor body")
		}
	}
	for _, encoded := range []string{"?", "Zh", strings.Repeat("A", 8193), "e30="} {
		if _, err := DecodeSourceCursor(encoded); err == nil {
			t.Fatal("accepted invalid cursor encoding")
		}
	}
	for _, cursor := range []LegacySourceCursor{{Kind: "card"}, {ID: "x"}, {Kind: "card", ID: strings.Repeat("x", 2049)}} {
		if _, err := EncodeSourceCursor(cursor); err == nil {
			t.Fatal("encoded incomplete cursor")
		}
	}
}
