// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"encoding/json"
	"testing"
)

func frozenBindingFixture() FrozenBinding {
	return FrozenBinding{Binding: Binding{Scope: Scope{Tenant: "acme", Subject: "alice", Product: "drive"}, Target: PublicationTarget{RoomID: "!current:test", DeviceID: "SOURCE"}, Rooms: []string{"!current:test", "!previous:test"}}, Fences: []Reference{{RoomID: "!current:test", EventID: "$current-fence"}, {RoomID: "!previous:test", EventID: "$previous-fence"}}}
}

func TestFrozenBindingKeepsEveryRoomAndDistinctImmutableFence(t *testing.T) {
	original := frozenBindingFixture()
	if !original.Valid() {
		t.Fatal("valid fixture rejected")
	}
	for _, fault := range []string{"scope", "target", "missing-room", "missing-fence", "foreign-fence", "duplicate-room", "duplicate-fence", "reused-event", "bad-event", "bad-room"} {
		t.Run(fault, func(t *testing.T) {
			value := frozenBindingFixture()
			switch fault {
			case "scope":
				value.Binding.Scope.Product = "foreign"
			case "target":
				value.Binding.Target.DeviceID = ""
			case "missing-room":
				value.Binding.Rooms = value.Binding.Rooms[:1]
			case "missing-fence":
				value.Fences = value.Fences[:1]
			case "foreign-fence":
				value.Fences[0].RoomID = "!foreign:test"
			case "duplicate-room":
				value.Binding.Rooms[1] = value.Binding.Rooms[0]
			case "duplicate-fence":
				value.Fences[1] = value.Fences[0]
			case "reused-event":
				value.Fences[1].EventID = value.Fences[0].EventID
			case "bad-event":
				value.Fences[0].EventID = "$bad id"
			case "bad-room":
				value.Fences[0].RoomID = "!bad room"
			}
			if value.Valid() || original.Same(value) || value.Same(original) {
				t.Fatal("invalid frozen contract accepted", fault)
			}
		})
	}
}

func TestFrozenBindingAllowsAnotherReaderButNotAFenceOrScopeChange(t *testing.T) {
	original := frozenBindingFixture()
	for _, change := range []string{"device", "ordering", "fence", "tenant", "subject", "product", "target-room", "rooms"} {
		t.Run(change, func(t *testing.T) {
			value := frozenBindingFixture()
			switch change {
			case "device":
				value.Binding.Target.DeviceID = "ANOTHER_VERIFIED_READER"
			case "ordering":
				value.Binding.Rooms[0], value.Binding.Rooms[1] = value.Binding.Rooms[1], value.Binding.Rooms[0]
				value.Fences[0], value.Fences[1] = value.Fences[1], value.Fences[0]
			case "fence":
				value.Fences[0].EventID = "$re-frozen-after-being-writable"
			case "tenant":
				value.Binding.Scope.Tenant = "beta"
			case "subject":
				value.Binding.Scope.Subject = "bob"
			case "product":
				value.Binding.Scope.Product = "mail"
			case "target-room":
				value.Binding.Target.RoomID = "!previous:test"
			case "rooms":
				value.Binding.Rooms[1] = "!other:test"
				value.Fences[1].RoomID = "!other:test"
			}
			want := change == "device" || change == "ordering"
			if !value.Valid() || original.Same(value) != want || value.Same(original) != want {
				t.Fatal("frozen routing equality changed", change)
			}
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var restored FrozenBinding
			if json.Unmarshal(raw, &restored) != nil || !value.Same(restored) {
				t.Fatal("checkpoint did not roundtrip", change)
			}
		})
	}
}
