// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"encoding/json"
	"testing"
)

func TestConClientIDConservaData(t *testing.T) {
	in := NoteInput{Kind: "link", Data: json.RawMessage(`{"url":"https://x.eu"}`)}.ConClientID("c1")
	var d map[string]string
	if err := json.Unmarshal(in.Data, &d); err != nil || d["url"] != "https://x.eu" || d["client_id"] != "c1" {
		t.Fatalf("data = %s (%v)", in.Data, err)
	}
	if in.ClientID() != "c1" || (NoteInput{}).ClientID() != "" {
		t.Error("ClientID no lee la marca")
	}
	if (NoteInput{}).ConClientID("c2").ClientID() != "c2" {
		t.Error("sin data no se marca")
	}
}
