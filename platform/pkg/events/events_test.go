// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package events

import (
	"encoding/json"
	"testing"
)

func TestNewEvent(t *testing.T) {
	ev, err := NewEvent("drive", "drive.file.created", "acme", map[string]string{"path": "/a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID == "" || ev.CreatedAt.IsZero() {
		t.Fatal("ID/timestamp no generados")
	}
	if ev.Channel != "drive" || ev.Type != "drive.file.created" || ev.Tenant != "acme" {
		t.Fatalf("campos incorrectos: %+v", ev)
	}
	var p map[string]string
	if err := json.Unmarshal(ev.Payload, &p); err != nil || p["path"] != "/a.txt" {
		t.Fatalf("payload mal serializado: %v", err)
	}
}

func TestNewEventValidation(t *testing.T) {
	if _, err := NewEvent("", "x", "", nil); err == nil {
		t.Fatal("esperaba error: channel vacío")
	}
	if _, err := NewEvent("c", "", "", nil); err == nil {
		t.Fatal("esperaba error: type vacío")
	}
}

func TestChannelName(t *testing.T) {
	if got := Channel("drive").channelName(); got != "sytue_drive" {
		t.Fatalf("channelName = %q", got)
	}
}

func TestNewEventNilPayload(t *testing.T) {
	ev, err := NewEvent("hub", "hub.ping", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Payload != nil {
		t.Fatalf("payload nil esperado, got %s", ev.Payload)
	}
}

func TestNewEventUnmarshalablePayload(t *testing.T) {
	// Un canal no es serializable a JSON: NewEvent debe devolver error.
	if _, err := NewEvent("c", "t", "", make(chan int)); err == nil {
		t.Fatal("esperaba error de serialización")
	}
}

func TestChannelFromPG(t *testing.T) {
	cases := map[string]Channel{
		"sytue_drive": "drive",
		"sytue_chat":  "chat",
		"sytue_":      "sytue_", // prefijo exacto sin sufijo: se conserva
		"otro":        "otro",   // sin prefijo: se conserva tal cual
		"":            "",
	}
	for in, want := range cases {
		if got := channelFromPG(in); got != want {
			t.Errorf("channelFromPG(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNotificationRoundTrip(t *testing.T) {
	n := Notification{ID: "id-1", Channel: "drive", Type: "drive.file.created"}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var got Notification
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != n {
		t.Fatalf("round-trip: %+v != %+v", got, n)
	}
}
