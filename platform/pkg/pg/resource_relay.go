// SPDX-License-Identifier: Apache-2.0
package pg

import (
	_ "embed"
	"io/fs"
)

//go:embed resource_relay_v1.sql
var resourceRelayV1 []byte

//go:embed resource_relay_down_v1.sql
var resourceRelayDownV1 []byte

//go:embed resource_bootstrap_v1.sql
var resourceBootstrapV1 []byte

//go:embed resource_bootstrap_down_v1.sql
var resourceBootstrapDownV1 []byte

//go:embed resource_roster_capture_v1.sql
var resourceRosterCaptureV1 []byte

//go:embed resource_roster_capture_down_v1.sql
var resourceRosterCaptureDownV1 []byte

//go:embed resource_roster_evidence_v1.sql
var resourceRosterEvidenceV1 []byte

//go:embed resource_roster_evidence_down_v1.sql
var resourceRosterEvidenceDownV1 []byte

// WithResourceRelayDDL expands the same immutable routing/CAS and retained
// reservation schema in each participating product database. Grants stay local.
func WithResourceRelayDDL(source fs.FS) fs.FS {
	return withMigrationDDL(source,
		migrationDDLFragment{marker: "-- sytue:resource-relay-v1", sql: resourceRelayV1},
		migrationDDLFragment{marker: "-- sytue:resource-relay-down-v1", sql: resourceRelayDownV1, suffix: ".down.sql"},
		migrationDDLFragment{marker: "-- sytue:resource-bootstrap-v1", sql: resourceBootstrapV1},
		migrationDDLFragment{marker: "-- sytue:resource-bootstrap-down-v1", sql: resourceBootstrapDownV1, suffix: ".down.sql"},
		migrationDDLFragment{marker: "-- sytue:resource-roster-capture-v1", sql: resourceRosterCaptureV1},
		migrationDDLFragment{marker: "-- sytue:resource-roster-capture-down-v1", sql: resourceRosterCaptureDownV1, suffix: ".down.sql"},
		migrationDDLFragment{marker: "-- sytue:resource-roster-evidence-v1", sql: resourceRosterEvidenceV1},
		migrationDDLFragment{marker: "-- sytue:resource-roster-evidence-down-v1", sql: resourceRosterEvidenceDownV1, suffix: ".down.sql"})
}
