// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package db embebe las migraciones SQL versionadas de Sytue Cast para que el
// runner de migraciones de platform/pkg/pg las aplique al arrancar (o vía CLI).
package db

import "embed"

// Migrations contiene los ficheros *.up.sql / *.down.sql de db/migrations.
//
//go:embed all:migrations
var Migrations embed.FS

// MigrationsDir es la subcarpeta dentro de Migrations donde viven los .sql.
const MigrationsDir = "migrations"
