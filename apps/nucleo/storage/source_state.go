// SPDX-License-Identifier: Apache-2.0
package storage

import "encoding/json"

// AppMigrationState separates an immutable historical source from collaborative
// data and from the continuing ingress capability used by protocol applications.
// Frozen does not mean that later SMTP/IMAP input is already privately handled.
type AppMigrationState struct {
	Version int `json:"v"`
	LegacyMigrationState
	Shared              bool  `json:"shared"`
	MaxBodyBytes        int64 `json:"max_body_bytes,omitempty"`
	PrivateIngressReady bool  `json:"private_ingress_ready,omitempty"`
}

func (s AppMigrationState) Validate(scope Scope) error {
	if s.Version != 1 || s.MaxBodyBytes < 0 || s.MaxBodyBytes > MaxFileBytes {
		return ErrRecord
	}
	return s.LegacyMigrationState.Validate(scope)
}

// Shared must be explicit on the wire: an older server omitting capability
// information cannot silently be interpreted as a wholly private source.
func (s *AppMigrationState) UnmarshalJSON(raw []byte) error {
	type plain AppMigrationState
	var value struct {
		plain
		Shared *bool `json:"shared"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if value.Shared == nil {
		return ErrRecord
	}
	*s = AppMigrationState(value.plain)
	s.Shared = *value.Shared
	return nil
}
