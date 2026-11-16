// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package db embebe las migraciones SQL versionadas de Sytue Mail para que el
// runner de migraciones de platform/pkg/pg las aplique al arrancar (o vía CLI).
package db

import (
	"embed"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Migrations contiene los ficheros *.up.sql / *.down.sql de db/migrations.
//
//go:embed all:migrations
var migrationFiles embed.FS

// Migrations expands only explicitly versioned shared private-storage SQL.
var Migrations = pg.WithScopeAdmissionDDL(pg.WithScopeBoundMergeDDL(pg.WithPrivateIngressDDL(migrationFiles)))

// MigrationsDir es la subcarpeta dentro de Migrations donde viven los .sql.
const MigrationsDir = "migrations"
