// SPDX-License-Identifier: Apache-2.0
package storage

// IdentityCutoverState contains only durable public routing. Its encrypted
// completion receipt and all application contents remain in verified clients.
type IdentityCutoverState struct {
	Version     int        `json:"v"`
	Scope       Scope      `json:"scope"`
	Phase       string     `json:"phase"`
	Epoch       string     `json:"epoch,omitempty"`
	SourceRooms []string   `json:"source_rooms"`
	Receipt     *Reference `json:"receipt,omitempty"`
}

func (s IdentityCutoverState) Validate(scope Scope) error {
	if s.Version != 1 || s.Scope != scope || !scope.Valid() || len(s.SourceRooms) > 100 {
		return ErrRecord
	}
	seen := map[string]bool{}
	for _, room := range s.SourceRooms {
		if !matrixID(room, '!') || seen[room] {
			return ErrRecord
		}
		seen[room] = true
	}
	if s.Phase == "legacy" {
		if s.Epoch != "" || s.Receipt != nil {
			return ErrRecord
		}
		return nil
	}
	if !MigrationEpoch(s.Epoch) {
		return ErrRecord
	}
	switch s.Phase {
	case "freezing", "frozen":
		if s.Receipt != nil {
			return ErrRecord
		}
	case "isolated":
		if s.Receipt == nil || !s.Receipt.Valid() || seen[s.Receipt.RoomID] {
			return ErrRecord
		}
	default:
		return ErrRecord
	}
	return nil
}

// RequiresLegacy is the common device-routing plan. It checks public metadata
// only: a completed encrypted receipt still has to be verified by the original
// application. Empty source-room metadata never proves a migration complete.
func (s IdentityCutoverState) RequiresLegacy(scope Scope) (bool, error) {
	if err := s.Validate(scope); err != nil {
		return false, err
	}
	return s.Phase != "isolated", nil
}
