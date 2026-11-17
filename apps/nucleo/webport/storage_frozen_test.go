// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestFrozenStoragePortPinsActualSourceIdentityRoutingAndFences(t *testing.T) {
	identity := fixtureIdentity()
	binding := storage.Binding{Scope: identity.Scope, Target: storage.PublicationTarget{RoomID: "!source:test", DeviceID: identity.Binding.DeviceID}, Rooms: []string{"!source:test"}}
	for _, fault := range []string{"none", "missing-fence", "foreign-scope", "changed-binding", "isolated"} {
		t.Run(fault, func(t *testing.T) {
			current := identity
			if fault == "isolated" {
				current.Binding.MatrixUserID = storage.IsolatedMatrixUserPrefix(identity.Scope) + "test:matrix.test"
			}
			bindingReads, hostCalls := 0, 0
			port := storagePort{sourcePort: sourcePort{id: "port", identity: current}, handle: func(method Method, _ []byte) ([]byte, error) {
				hostCalls++
				switch method {
				case StorageBindings:
					bindingReads++
					value := binding
					if fault == "changed-binding" && bindingReads > 1 {
						value.Rooms = append([]string{}, binding.Rooms...)
						value.Rooms = append(value.Rooms, "!added:test")
					}
					return json.Marshal(value)
				case StorageFrozen:
					value := storage.FrozenBinding{Binding: binding, Fences: []storage.Reference{{RoomID: binding.Target.RoomID, EventID: "$powers"}}}
					if fault == "missing-fence" {
						value.Fences = nil
					}
					if fault == "foreign-scope" {
						value.Binding.Scope.Subject = "foreign"
					}
					return json.Marshal(value)
				default:
					return nil, ErrRejected
				}
			}}
			owner, err := OpenStorage(t.Context(), port, NewSources())
			if err != nil {
				t.Fatal(err)
			}
			value, err := owner.FrozenStorageBindings(t.Context())
			if fault == "none" {
				if err != nil || !value.Valid() || value.Fences[0].EventID != "$powers" {
					t.Fatal("exact frozen source changed", err)
				}
			} else if !errors.Is(err, ErrRejected) || value.Valid() {
				t.Fatal("foreign or changed frozen source accepted", fault, value, err)
			}
			if fault == "isolated" && hostCalls != 0 {
				t.Fatal("isolated identity reached legacy host")
			}
		})
	}
}
