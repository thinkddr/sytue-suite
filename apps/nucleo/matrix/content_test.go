// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestWebEnvelopeCardUsesClientNames(t *testing.T) {
	content := MessageContent{Version: 1, Body: "Artificial task", Card: &api.RoomsCard{Kind: "task", AssigneeSub: "person", AssigneeName: "Artificial person"}}
	wire, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "assignee_sub") || !strings.Contains(string(wire), `"assigneeSub":"person"`) {
		t.Fatal("wire card differs from web client")
	}
	raw, _ := json.Marshal(map[string]any{"body": content.Body, "org.sytue.rooms": json.RawMessage(wire)})
	decoded, err := DecodeMessage(DecryptedEvent{Type: event.EventMessage.Type, Content: raw})
	if err != nil || decoded.Card == nil || decoded.Card.AssigneeSub != "person" {
		t.Fatalf("cross-client card: %+v %v", decoded, err)
	}
}

func TestOpaqueMessageCannotBecomeImplicitLegacyPlaintext(t *testing.T) {
	service := NewRoomsService(nil, nil)
	_, err := service.hydrate(t.Context(), api.RoomsMessage{Body: "must not display as legacy"})
	if !errors.Is(err, ErrNotEncrypted) {
		t.Fatal("missing crypto reference admitted plaintext")
	}
	if _, err = service.hydrate(t.Context(), api.RoomsMessage{Encryption: "legacy", Body: "explicit old message"}); err != nil {
		t.Fatal(err)
	}
}

func TestWaitReadyStopsWhenClosed(t *testing.T) {
	c := &Client{firstSync: make(chan struct{})}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.WaitReady(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestRoomVersion12OpaqueBindingHasNoDomainRequirement(t *testing.T) {
	const room = "!AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	c := &Client{cfg: Config{Subject: "person", Tenant: "org", Bootstrap: Bootstrap{Actor: "person", Tenant: "org", MatrixUserID: "@person:matrix.test", HomeserverURL: "https://matrix.test", Encryption: "m.megolm.v1.aes-sha2"}}}
	next := c.cfg.Bootstrap
	next.Channels = []RoomBinding{{ChannelID: "channel", RoomID: room}}
	if err := c.UpdateBindings(next); err != nil {
		t.Fatal(err)
	}
	got, err := c.RoomID("channel")
	if err != nil || got != room || !c.allowedRoom(room) {
		t.Fatalf("version12 binding rejected: %s %v", got, err)
	}
	next.Tenant = "foreign"
	if err = c.UpdateBindings(next); !errors.Is(err, ErrIdentity) {
		t.Fatal("foreign tenant changed opaque room binding")
	}
}

func TestReceiptPageBoundsAndReadinessFailBeforeNetwork(t *testing.T) {
	c := &Client{}
	if c.allowedRoom("") {
		t.Fatal("an absent draft room authorized the empty room")
	}
	for _, n := range []int{0, 101} {
		if values, err := c.ReadMany(t.Context(), "!room:matrix.test", make([]EventRef, n)); err == nil || len(values) != 0 {
			t.Fatalf("unbounded receipt page of %d was allowed", n)
		}
	}
	refs := []EventRef{{RoomID: "!room:matrix.test", EventID: "$one"}}
	if _, err := c.ReadMany(t.Context(), "!room:matrix.test", refs); !errors.Is(err, ErrNotReady) {
		t.Fatalf("read before encrypted sync: %v", err)
	}
	c.closed.Store(true)
	if _, err := c.ReadMany(t.Context(), "!room:matrix.test", refs); !errors.Is(err, ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
}
