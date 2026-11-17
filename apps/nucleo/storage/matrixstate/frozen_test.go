// SPDX-License-Identifier: Apache-2.0
package matrixstate

import (
	"encoding/json"
	"errors"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const (
	frozenRoom       = "!source:matrix.test"
	frozenOwner      = id.UserID("@sytue_person:matrix.test")
	frozenController = id.UserID("@rooms_controller:matrix.test")
)

func frozenEvent(t *testing.T, kind event.Type, eventID string, content map[string]any) *event.Event {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"type": kind.Type, "event_id": eventID, "room_id": frozenRoom, "sender": frozenController, "state_key": "", "content": content})
	if err != nil {
		t.Fatal(err)
	}
	var out event.Event
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	// Exactly StateAsArray's boundary: the SDK does not parse Content here.
	if out.Content.Parsed != nil {
		t.Fatal("fixture unexpectedly parsed state")
	}
	return &out
}

func frozenState(t *testing.T, version string) ([]*event.Event, map[string]any, map[string]any) {
	t.Helper()
	creation := map[string]any{"room_version": version, "m.federate": false}
	users := map[string]any{}
	if version == "11" {
		users[frozenController.String()] = 100
	}
	powers := map[string]any{"users": users, "users_default": 0, "events_default": 100, "state_default": 100, "invite": 100, "kick": 100, "ban": 100, "redact": 100, "events": map[string]any{"m.room.encrypted": 100, "m.room.message": 100}}
	return []*event.Event{frozenEvent(t, event.StateCreate, "$created", creation), frozenEvent(t, event.StatePowerLevels, "$frozen", powers)}, creation, powers
}

func TestFrozenStorageRoomDecodesUnparsedSDKStateAndPreservesItsReceipt(t *testing.T) {
	for _, version := range []string{"11", "12"} {
		t.Run(version, func(t *testing.T) {
			state, _, _ := frozenState(t, version)
			ref, err := FrozenRoom(state, frozenRoom, frozenOwner, frozenController)
			if err != nil || ref.RoomID != frozenRoom || ref.EventID != "$frozen" {
				t.Fatal("unparsed read-only state rejected", ref, err)
			}
			for _, value := range state {
				value.RoomID = ""
			}
			if _, err := FrozenRoom(state, frozenRoom, frozenOwner, frozenController); err != nil {
				t.Fatal("standard state without room_id rejected", err)
			}
		})
	}
}

func TestFrozenStorageRoomRejectsWritableOrMutableSourcePolicy(t *testing.T) {
	for _, fault := range []string{"encrypted", "message", "custom", "events-default", "state-default", "invite", "kick", "ban", "redact", "owner-power", "implicit-creator", "missing-invite"} {
		t.Run(fault, func(t *testing.T) {
			_, creation, powers := frozenState(t, "12")
			switch fault {
			case "encrypted":
				powers["events"].(map[string]any)["m.room.encrypted"] = 0
			case "message":
				powers["events"].(map[string]any)["m.room.message"] = 0
			case "custom":
				powers["events"].(map[string]any)["org.sytue.storage"] = 0
			case "events-default":
				powers["events_default"] = 0
			case "state-default":
				powers["state_default"] = 0
			case "invite", "kick", "ban", "redact":
				powers[fault] = 0
			case "owner-power":
				powers["users"].(map[string]any)[frozenOwner.String()] = 100
			case "implicit-creator":
				creation["additional_creators"] = []string{frozenOwner.String()}
			case "missing-invite":
				delete(powers, "invite")
			}
			state := []*event.Event{frozenEvent(t, event.StateCreate, "$created", creation), frozenEvent(t, event.StatePowerLevels, "$frozen", powers)}
			if ref, err := FrozenRoom(state, frozenRoom, frozenOwner, frozenController); !errors.Is(err, ErrStorageNotFrozen) || ref.Valid() {
				t.Fatal("mutable source accepted", fault, ref, err)
			}
		})
	}
}

func TestFrozenStorageRoomRejectsMalformedForeignAndAmbiguousState(t *testing.T) {
	for _, fault := range []string{"missing-create", "missing-powers", "duplicate-create", "duplicate-powers", "foreign-create-room", "foreign-powers-room", "foreign-controller", "owner-controller", "empty-owner", "empty-controller", "missing-power-id", "missing-create-id", "malformed-power-id", "nonempty-create-key", "nonempty-powers-key", "create-type", "power-type", "unknown-version", "malformed-creators", "partially-parsed-creators", "null-create", "null-powers", "null-state", "extra-invalid-key"} {
		t.Run(fault, func(t *testing.T) {
			state, creation, powers := frozenState(t, "12")
			owner, controller := frozenOwner, frozenController
			switch fault {
			case "missing-create":
				state = state[1:]
			case "missing-powers":
				state = state[:1]
			case "duplicate-create":
				state = append(state, state[0])
			case "duplicate-powers":
				state = append(state, state[1])
			case "foreign-create-room":
				state[0].RoomID = "!foreign:matrix.test"
			case "foreign-powers-room":
				state[1].RoomID = "!foreign:matrix.test"
			case "foreign-controller":
				state[0].Sender = "@foreign:matrix.test"
			case "owner-controller":
				controller = owner
			case "empty-owner":
				owner = ""
			case "empty-controller":
				controller = ""
			case "missing-power-id":
				state[1].ID = ""
			case "missing-create-id":
				state[0].ID = ""
			case "malformed-power-id":
				state[1].ID = "$bad id"
			case "nonempty-create-key":
				key := "other"
				state[0].StateKey = &key
			case "nonempty-powers-key":
				key := "other"
				state[1].StateKey = &key
			case "create-type":
				creation["room_version"] = 12
				state[0] = frozenEvent(t, event.StateCreate, "$created", creation)
			case "power-type":
				powers["users_default"] = "invalid"
				state[1] = frozenEvent(t, event.StatePowerLevels, "$frozen", powers)
			case "unknown-version":
				creation["room_version"] = "13"
				state[0] = frozenEvent(t, event.StateCreate, "$created", creation)
			case "malformed-creators", "partially-parsed-creators":
				creation["additional_creators"] = 1
				state[0] = frozenEvent(t, event.StateCreate, "$created", creation)
				if fault == "partially-parsed-creators" {
					// ParseRaw leaves an allocated, partially populated Parsed on error.
					if err := state[0].Content.ParseRaw(event.StateCreate); err == nil || state[0].Content.Parsed == nil {
						t.Fatal("fixture did not reproduce SDK partial parse")
					}
					if err := state[1].Content.ParseRaw(event.StatePowerLevels); err != nil {
						t.Fatal(err)
					}
				}
			case "null-state":
				state = append(state, nil)
			case "extra-invalid-key":
				extra := frozenEvent(t, event.StatePowerLevels, "$invalid", powers)
				key := "other"
				extra.StateKey = &key
				state = append(state, extra)
			case "null-create":
				state[0].Content = event.Content{VeryRaw: json.RawMessage("null")}
			case "null-powers":
				state[1].Content = event.Content{VeryRaw: json.RawMessage("null")}
			}
			if ref, err := FrozenRoom(state, frozenRoom, owner, controller); !errors.Is(err, ErrStorageNotFrozen) || ref.Valid() {
				t.Fatal("invalid state accepted", fault, ref, err)
			}
		})
	}
}
