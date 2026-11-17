// SPDX-License-Identifier: Apache-2.0
package storage

import "slices"

// FrozenBinding contains routing and the exact power-level events checked by a
// verified SDK. It contains no key or content. A changed fence requires a new
// snapshot, even if the room has since become read-only again.
type FrozenBinding struct {
	Binding Binding     `json:"binding"`
	Fences  []Reference `json:"fences"`
}

func (f FrozenBinding) Valid() bool {
	if !f.Binding.Valid() || len(f.Fences) != len(f.Binding.Rooms) {
		return false
	}
	seen := map[string]bool{}
	events := map[string]bool{}
	for _, fence := range f.Fences {
		if !fence.Valid() || !slices.Contains(f.Binding.Rooms, fence.RoomID) || seen[fence.RoomID] || events[fence.EventID] {
			return false
		}
		seen[fence.RoomID] = true
		events[fence.EventID] = true
	}
	return true
}

// Same permits another verified source reader: only destination publication
// requires the original device. Source room membership and fences stay fixed.
func (f FrozenBinding) Same(other FrozenBinding) bool {
	if !f.Valid() || !other.Valid() || f.Binding.Scope != other.Binding.Scope || f.Binding.Target.RoomID != other.Binding.Target.RoomID || len(f.Fences) != len(other.Fences) {
		return false
	}
	for _, fence := range f.Fences {
		if !slices.Contains(other.Fences, fence) {
			return false
		}
	}
	return true
}
