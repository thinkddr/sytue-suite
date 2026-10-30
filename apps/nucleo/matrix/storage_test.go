// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"maunium.net/go/mautrix"
	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func storageConfig(t *testing.T, server *httptest.Server, product string) Config {
	t.Helper()
	cfg := configFor(t, server)
	cfg.Bootstrap.Channels = nil
	cfg.Bootstrap.StorageProduct = product
	cfg.Bootstrap.StorageRoomID = "!" + product + ":matrix.test"
	cfg.Bootstrap.StorageRoomIDs = []string{cfg.Bootstrap.StorageRoomID}
	return cfg
}

func TestStorageDeviceAndRoomBindingsCannotCrossProducts(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	cfg := storageConfig(t, server, "drive")
	drive, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	mail, err := newClient(storageConfig(t, server, "mail"))
	if err != nil {
		t.Fatal(err)
	}
	rooms, err := newClient(configFor(t, server))
	if err != nil {
		t.Fatal(err)
	}
	if drive.account == mail.account || drive.account == rooms.account || mail.account == rooms.account {
		t.Fatal("apps reused one Matrix device/store")
	}
	if !drive.allowedRoom("!drive:matrix.test") || drive.allowedRoom("!mail:matrix.test") {
		t.Fatal("room binding crossed apps")
	}
	cfg.Bootstrap.StorageRoomIDs[0] = "!injected:matrix.test"
	if drive.allowedRoom("!injected:matrix.test") {
		t.Fatal("caller mutated pinned bindings")
	}
	next := drive.cfg.Bootstrap
	next.StorageProduct = "mail"
	if !errors.Is(drive.UpdateBindings(next), ErrIdentity) {
		t.Fatal("binding update changed product")
	}
	bad := storageConfig(t, server, "drive")
	bad.Bootstrap.Channels = []RoomBinding{{ChannelID: "channel", RoomID: "!channel:matrix.test"}}
	if _, err := newClient(bad); !errors.Is(err, ErrIdentity) {
		t.Fatal("mixed storage/channel authority", err)
	}
}

func TestPrivateStorageAndDraftsRejectAnotherHumanBeforeKeySharing(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	for _, product := range []string{"drive", "mail", "rooms", ""} {
		t.Run(product, func(t *testing.T) {
			cfg := storageConfig(t, server, "drive")
			if product == "" {
				cfg = configFor(t, server)
				cfg.Bootstrap.DraftRoomID = "!private:matrix.test"
				cfg.Bootstrap.DraftRoomIDs = []string{cfg.Bootstrap.DraftRoomID}
			} else {
				cfg.Bootstrap.StorageProduct = product
			}
			client, err := newClient(cfg)
			if err != nil {
				t.Fatal(err)
			}
			room := id.RoomID(cfg.Bootstrap.StorageRoomID)
			if product == "" {
				room = id.RoomID(cfg.Bootstrap.DraftRoomID)
			}
			state := mautrix.NewMemoryStateStore()
			client.cli.StateStore = state
			for _, user := range []string{cfg.Bootstrap.MatrixUserID, cfg.Bootstrap.ControllerUserID, "@trusted-coworker:matrix.test"} {
				if err := state.SetMember(t.Context(), room, id.UserID(user), &event.MemberEventContent{Membership: event.MembershipJoin}); err != nil {
					t.Fatal(err)
				}
			}
			// No crypto helper is installed: the guard must reject even a potentially
			// trusted coworker before consulting trust or sharing any group key.
			if err := client.checkRoomIdentities(t.Context(), room); !errors.Is(err, ErrIdentity) {
				t.Fatal("private data reached another human", err)
			}
		})
	}
}

func TestStorageRecordPinsOwnerProductObjectAndRole(t *testing.T) {
	scope := storage.Scope{Tenant: "org", Subject: "person", Product: "drive"}
	record := storage.Record{Version: 1, Scope: scope, ObjectID: "opaque-object", RevisionID: "opaque-revision", Role: "content", Payload: json.RawMessage(`{"title":"private filename"}`)}
	raw, _ := json.Marshal(map[string]any{"msgtype": "m.text", "body": "", "org.sytue.storage": record})
	evt := DecryptedEvent{Sender: "@person:matrix.test", Type: event.EventMessage.Type, Content: raw}
	if _, err := decodeStorageRecord(evt, scope, evt.Sender, record.ObjectID, record.Role); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"sender", "tenant", "subject", "product", "object", "role"} {
		t.Run(field, func(t *testing.T) {
			foreign := evt
			other := scope
			object, role := record.ObjectID, record.Role
			switch field {
			case "sender":
				foreign.Sender = "@coworker:matrix.test"
			case "tenant":
				other.Tenant = "other"
			case "subject":
				other.Subject = "other"
			case "product":
				other.Product = "mail"
			case "object":
				object = "other"
			case "role":
				role = "preview"
			}
			if _, err := decodeStorageRecord(foreign, other, evt.Sender, object, role); err == nil {
				t.Fatal("scope substitution accepted")
			}
		})
	}
	record.Previous = []string{"$a", "$a"}
	if record.Validate(scope) == nil {
		t.Fatal("duplicate revision branch accepted")
	}
	record.Previous = slices.Clip([]string{"$a", "$b"})
	if record.Validate(scope) != nil {
		t.Fatal("concurrent revision branches lost")
	}
}

func TestStorageSDKInventoryNeverOffersAnotherHumansDevices(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	cfg := storageConfig(t, server, "drive")
	client, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	own, controller := id.UserID(cfg.Bootstrap.MatrixUserID), id.UserID(cfg.Bootstrap.ControllerUserID)
	store := mxcrypto.NewMemoryStore(nil)
	policy := client.devicePolicy(store)
	for _, user := range []id.UserID{own, controller, "@verified_coworker:matrix.test"} {
		if err := store.PutDevice(t.Context(), user, &id.Device{UserID: user, DeviceID: "TRUSTED", Trust: id.TrustStateVerified}); err != nil {
			t.Fatal(err)
		}
		devices, err := policy.GetDevices(t.Context(), user)
		if err != nil {
			t.Fatal(err)
		}
		if user == own && len(devices) != 1 {
			t.Fatal("own verified device excluded")
		}
		if user != own && (devices == nil || len(devices) != 0) {
			t.Fatal("private room offered a foreign device or a fetch fallback")
		}
	}
}

func TestStorageSDKDecodingPreservesPrivatePayloadExactly(t *testing.T) {
	scope := storage.Scope{Tenant: "org", Subject: "person", Product: "erp"}
	record := storage.Record{Version: 1, Scope: scope, ObjectID: "record", RevisionID: "revision", Role: "content", Payload: json.RawMessage(`{"amount":9007199254740993,"title":"private"}`)}
	wire, err := json.Marshal(map[string]any{"type": event.EventMessage.Type, "content": map[string]any{"msgtype": "m.text", "body": "", "org.sytue.storage": record}})
	if err != nil {
		t.Fatal(err)
	}
	var parsed event.Event
	if err := json.Unmarshal(wire, &parsed); err != nil {
		t.Fatal(err)
	}
	if err := parsed.Content.ParseRaw(event.EventMessage); err != nil {
		t.Fatal(err)
	}
	// A generic SDK map (like JSON.parse in a browser) reserializes numbers as
	// doubles. The application payload must remain an opaque, lossless byte value.
	content, err := json.Marshal(&parsed.Content)
	if err != nil {
		t.Fatal(err)
	}
	value, err := decodeStorageRecord(DecryptedEvent{Type: event.EventMessage.Type, Sender: "@person:matrix.test", Content: content}, scope, "@person:matrix.test", record.ObjectID, record.Role)
	if err != nil || !bytes.Equal(value.Payload, record.Payload) {
		t.Fatal("SDK reserialization changed precise private data", err)
	}
}
