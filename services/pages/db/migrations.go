// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package db embebe las migraciones SQL versionadas de Sytue Web para que el
// runner de platform/pkg/pg las aplique al arrancar la API.
//
// Numeración (PAGES-ENCARGO §5): 0001_nucleo es la 0001; cada migración nueva toma
// max(origin/main) + 1 AL FUSIONAR, no al escribirla, porque varios agentes crean
// las suyas a la vez. Las fusionadas no se reescriben nunca.
package db

import "embed"

// Migrations contiene los ficheros *.up.sql / *.down.sql de db/migrations.
//
//go:embed all:migrations
var Migrations embed.FS

// MigrationsDir es la subcarpeta dentro de Migrations donde viven los .sql.
const MigrationsDir = "migrations"
