// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package events

import (
	"embed"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate installs the shared outbox without copying its schema into a service.
// It owns its own migration ledger; service migration versions remain separate.
func Migrate(dsn string) error {
	return pg.MigrateUpTable(migrations, "migrations", dsn, "events_schema_migrations")
}
