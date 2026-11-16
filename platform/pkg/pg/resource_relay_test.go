// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"bytes"
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestResourceRelayMigrationExpansionPreservesDirectionAndBytes(t *testing.T) {
	const up = "-- sytue:resource-relay-v1"
	const down = "-- sytue:resource-relay-down-v1"
	original := []byte("SELECT 'preserved';\n")
	source := fstest.MapFS{"migrations/0001_old.up.sql": &fstest.MapFile{Data: original}, "migrations/0002_resource.up.sql": &fstest.MapFile{Data: []byte(up)}, "migrations/0002_resource.down.sql": &fstest.MapFile{Data: []byte(down)}, "migrations/0031_capture.up.sql": &fstest.MapFile{Data: []byte("-- sytue:resource-roster-capture-v1")}, "migrations/0031_capture.down.sql": &fstest.MapFile{Data: []byte("-- sytue:resource-roster-capture-down-v1")}, "migrations/0033_evidence.up.sql": &fstest.MapFile{Data: []byte("-- sytue:resource-roster-evidence-v1")}, "migrations/0033_evidence.down.sql": &fstest.MapFile{Data: []byte("-- sytue:resource-roster-evidence-down-v1")}}
	wrapped := WithResourceRelayDDL(source)
	for name, want := range map[string][]byte{"migrations/0001_old.up.sql": original, "migrations/0002_resource.up.sql": resourceRelayV1, "migrations/0002_resource.down.sql": resourceRelayDownV1, "migrations/0031_capture.up.sql": resourceRosterCaptureV1, "migrations/0031_capture.down.sql": resourceRosterCaptureDownV1, "migrations/0033_evidence.up.sql": resourceRosterEvidenceV1, "migrations/0033_evidence.down.sql": resourceRosterEvidenceDownV1} {
		got, err := fs.ReadFile(wrapped, name)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("direction/bytes changed", name, err)
		}
	}
	for name, raw := range map[string]string{"migrations/0003_wrong.down.sql": up, "migrations/0004_wrong.up.sql": down, "migrations/0005_duplicate.down.sql": down + "\n" + down, "migrations/0032_capture_wrong.down.sql": "-- sytue:resource-roster-capture-v1", "migrations/0033_capture_wrong.up.sql": "-- sytue:resource-roster-capture-down-v1"} {
		source[name] = &fstest.MapFile{Data: []byte(raw)}
		if _, err := fs.ReadFile(wrapped, name); err == nil {
			t.Fatal("invalid resource migration reached SQL", name)
		}
	}
	if _, err := fs.ReadFile(withMigrationDDL(source, migrationDDLFragment{marker: down, sql: resourceRelayDownV1}), "migrations/0002_resource.down.sql"); err == nil {
		t.Fatal("legacy default up fragment expanded into down")
	}
	if _, err := fs.ReadFile(withMigrationDDL(source, migrationDDLFragment{marker: up, sql: resourceRelayV1, suffix: ".sql"}), "migrations/0002_resource.up.sql"); err == nil {
		t.Fatal("unrestricted suffix accepted")
	}
}
