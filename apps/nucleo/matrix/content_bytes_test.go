// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix/event"
)

func TestAuthenticatedContentKeepsOriginalCustomJSONNumbers(t *testing.T) {
	wire := []byte(`{"type":"m.room.message","content":{"msgtype":"m.text","body":"","org.sytue.custom":{"precise":9007199254740993}}}`)
	var parsed event.Event
	if err := json.Unmarshal(wire, &parsed); err != nil {
		t.Fatal(err)
	}
	if err := parsed.Content.ParseRaw(event.EventMessage); err != nil {
		t.Fatal(err)
	}
	content, err := eventContentBytes(&parsed)
	if err != nil || !bytes.Contains(content, []byte("9007199254740993")) {
		t.Fatal("authenticated JSON precision lost", err)
	}
}
