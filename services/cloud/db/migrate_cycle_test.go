// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test desechable de ciclo de migraciones: up completo → down 1 → up. Verifica
// que el down de la última migración es aplicable (rollback operativo).
package db_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
)

func TestMigrationDownUpCycle(t *testing.T) {
	h, err := pgtest.Start(context.Background(), clouddb.Migrations, clouddb.MigrationsDir)
	if err != nil {
		t.Fatalf("pgtest: %v", err)
	}
	defer h.Teardown()

	mg, err := pg.NewMigrator(clouddb.Migrations, clouddb.MigrationsDir, h.DSN)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	defer mg.Close()

	if err := mg.Steps(-1); err != nil {
		t.Fatalf("down 1: %v", err)
	}
	if err := mg.Up(); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	v, dirty, err := mg.Version()
	if err != nil || dirty {
		t.Fatalf("versión tras ciclo: v=%d dirty=%v err=%v", v, dirty, err)
	}
}
