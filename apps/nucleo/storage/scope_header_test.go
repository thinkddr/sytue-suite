// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestScopeHeaderCodec(t *testing.T) {
	for _, scope := range []Scope{{Tenant: "acme", Subject: "person", Product: "mail"}, {Tenant: "é", Subject: "💾 <owner>\u2028", Product: "drive"}} {
		header, err := EncodeScopeHeader(scope)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeScopeHeader(header)
		if err != nil || got != scope {
			t.Fatal("scope did not roundtrip", err)
		}
	}
	// Independently encoded by the browser's UTF-8 JSON writer (HTML is not escaped).
	browser := `{"tenant":"acme","subject":"💾 <owner>","product":"drive"}`
	got, err := DecodeScopeHeader("v1." + base64.RawURLEncoding.EncodeToString([]byte(browser)))
	if err != nil || got.Subject != "💾 <owner>" {
		t.Fatal("browser encoding", err)
	}
	for _, value := range []string{"", "v2.e30", "v1.e30=", "v1.Zh", "v1." + strings.Repeat("A", 4096), "v1.invalid"} {
		if _, err := DecodeScopeHeader(value); err == nil {
			t.Fatal("invalid header accepted")
		}
	}
	for _, raw := range []string{`{}`, `{"tenant":"acme","subject":"person","product":"unknown"}`, `{"tenant":"acme","subject":"person","product":"drive","extra":true}`, `{"tenant":"acme","subject":"person","product":"drive"} {}`, `{"tenant":"acme","subject":"person","product":"drive"}` + string([]byte{0xff})} {
		if _, err := DecodeScopeHeader("v1." + base64.RawURLEncoding.EncodeToString([]byte(raw))); err == nil {
			t.Fatal("invalid scope accepted")
		}
	}
	for _, scope := range []Scope{{Tenant: "acme", Subject: "", Product: "drive"}, {Tenant: "acme", Subject: "a\n", Product: "drive"}} {
		if _, err := EncodeScopeHeader(scope); err == nil {
			t.Fatal("invalid scope encoded")
		}
	}
}
