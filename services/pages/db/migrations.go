// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package db embebe las migraciones SQL versionadas de Sytue Web para que el
// runner de platform/pkg/pg las aplique al arrancar la API.
//
// Numeración (PAGES-ENCARGO §5): 0001_nucleo es la 0001; cada migración nueva toma
// max(origin/main) + 1 AL FUSIONAR, no al escribirla, porque varios agentes crean
// las suyas a la vez. Las fusionadas no se reescriben nunca.
package db

import (
	"embed"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// migrationFiles contiene los ficheros *.up.sql / *.down.sql de db/migrations.
//
//go:embed all:migrations
var migrationFiles embed.FS

// Migrations expande el SQL común de la cola de purga de tenant (ADR 0054).
var Migrations = pg.WithTenantPurgeDDL(migrationFiles)

// MigrationsDir es la subcarpeta dentro de Migrations donde viven los .sql.
const MigrationsDir = "migrations"
