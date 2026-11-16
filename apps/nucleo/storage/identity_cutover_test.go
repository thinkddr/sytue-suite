// SPDX-License-Identifier: Apache-2.0
package storage

import "testing"

func TestIdentityCutoverPinsScopeEpochSourceAndReceipt(t *testing.T) {
	scope := Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	valid := IdentityCutoverState{Version: 1, Scope: scope, Phase: "frozen", Epoch: "df75d053-b8c8-4eb3-8454-235ef9c532c4", SourceRooms: []string{"!source:matrix.test"}}
	if valid.Validate(scope) != nil {
		t.Fatal("valid frozen route rejected")
	}
	for _, fault := range []string{"scope", "version", "epoch", "phase", "duplicate", "source", "premature", "missing", "same-room", "receipt"} {
		t.Run(fault, func(t *testing.T) {
			state := valid
			state.SourceRooms = append([]string(nil), valid.SourceRooms...)
			switch fault {
			case "scope":
				state.Scope.Product = "mail"
			case "version":
				state.Version = 2
			case "epoch":
				state.Epoch = "00000000-0000-0000-0000-000000000000"
			case "phase":
				state.Phase = "complete"
			case "duplicate":
				state.SourceRooms = append(state.SourceRooms, state.SourceRooms[0])
			case "source":
				state.SourceRooms[0] = "foreign"
			case "premature":
				state.Receipt = &Reference{RoomID: "!isolated:matrix.test", EventID: "$receipt"}
			case "missing":
				state.Phase = "isolated"
			case "same-room":
				state.Phase = "isolated"
				state.Receipt = &Reference{RoomID: state.SourceRooms[0], EventID: "$receipt"}
			case "receipt":
				state.Phase = "isolated"
				state.Receipt = &Reference{RoomID: "!isolated:matrix.test", EventID: "invalid"}
			}
			if state.Validate(scope) == nil {
				t.Fatal("invalid cutover metadata accepted", fault)
			}
		})
	}
	valid.Phase = "isolated"
	valid.Receipt = &Reference{RoomID: "!isolated:matrix.test", EventID: "$receipt"}
	if valid.Validate(scope) != nil {
		t.Fatal("valid complete route rejected")
	}
}

func TestIdentityDevicePlanIsUniformAndCannotInferEmptyLegacyCompletion(t *testing.T) {
	for _, product := range Products() {
		t.Run(product, func(t *testing.T) {
			scope := Scope{Tenant: "acme", Subject: "alice", Product: product}
			for _, phase := range []string{"legacy", "freezing", "frozen", "isolated"} {
				state := IdentityCutoverState{Version: 1, Scope: scope, Phase: phase, SourceRooms: []string{}}
				if phase != "legacy" {
					state.Epoch = "df75d053-b8c8-4eb3-8454-235ef9c532c4"
				}
				if phase == "isolated" {
					state.Receipt = &Reference{RoomID: "!isolated:matrix.test", EventID: "$receipt"}
				}
				required, err := state.RequiresLegacy(scope)
				if err != nil || required != (phase != "isolated") {
					t.Fatal("device plan changed existing migration semantics", phase, err)
				}
				bad := scope
				bad.Subject = "other"
				if _, err := state.RequiresLegacy(bad); err == nil {
					t.Fatal("crossed scope selected a device plan")
				}
				state.Version = 2
				if _, err := state.RequiresLegacy(scope); err == nil {
					t.Fatal("invalid state became an isolated-only plan")
				}
			}
		})
	}
}
