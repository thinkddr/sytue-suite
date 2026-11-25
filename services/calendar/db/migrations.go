// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package db embebe las migraciones SQL versionadas de Sytue Calendar para que
// el binario las aplique en arranque (golang-migrate sobre el FS embebido). El
// embed debe vivir junto a los ficheros: //go:embed no admite rutas con "..".
package db

import (
	"embed"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Migrations contiene los ficheros db/migrations/*.sql embebidos.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

var Migrations = pg.WithAuditLogDDL(pg.WithScopeAdmissionDDL(pg.WithScopeBoundMergeDDL(pg.WithPrivateIngressDDL(migrationFiles))))

// MigrationsDir es la subcarpeta dentro de Migrations con los .sql.
const MigrationsDir = "migrations"
