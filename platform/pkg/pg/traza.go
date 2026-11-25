// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	_ "embed"
	"io/fs"
)

//go:embed audit_log_v1.sql
var auditLogV1 []byte

// MarcadorTraza es la línea que una migración escribe para recibir la tabla
// `audit_log` común (audit_log_v1.sql): la traza de acceso y de cumplimiento de
// docs/security.md §1.7, con su RLS, su política de fusión, su retención y su
// disparador de solo añadir. La escribe platform/pkg/auditlog.
const MarcadorTraza = "-- sytue:audit-log-v1"

// WithAuditLogDDL expande MarcadorTraza en las migraciones de un servicio.
func WithAuditLogDDL(source fs.FS) fs.FS {
	return withMigrationDDL(source, migrationDDLFragment{marker: MarcadorTraza, sql: auditLogV1})
}
