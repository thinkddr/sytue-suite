// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"encoding/json"
	"testing"
)

func TestAppMigrationCapabilitiesRoundtripAndValidate(t *testing.T) {
	scope := Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	want := AppMigrationState{Version: 1, LegacyMigrationState: LegacyMigrationState{Scope: scope, Mode: "legacy"}, MaxBodyBytes: 100 << 20, PrivateIngressReady: true}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got AppMigrationState
	if json.Unmarshal(raw, &got) != nil || got.Validate(scope) != nil || got != want {
		t.Fatal("capability roundtrip changed", got)
	}
	for _, bad := range []AppMigrationState{{Version: 2, LegacyMigrationState: want.LegacyMigrationState}, {Version: 1, LegacyMigrationState: want.LegacyMigrationState, MaxBodyBytes: -1}, {Version: 1, LegacyMigrationState: want.LegacyMigrationState, MaxBodyBytes: MaxFileBytes + 1}} {
		if bad.Validate(scope) == nil {
			t.Fatal("invalid capability accepted", bad)
		}
	}
}
