// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
)

// dosRoles ejecuta la prueba dos veces: con el rol de aplicación (la RLS se aplica) y con
// el rol dueño de la base, que es superusuario y para el que PostgreSQL NO evalúa ninguna
// política. Así se conectan hoy los servicios en producción (auditoría 2026-10-06): un
// aislamiento que solo aguanta con el primero no aguanta. `super` es siempre el pool
// privilegiado, para sembrar o mirar por fuera de lo que se prueba.
func dosRoles(t *testing.T, fn func(t *testing.T, s *Store, super *pg.Pool)) {
	t.Helper()
	for _, rol := range []string{"rol de aplicación", "rol que elude la RLS"} {
		t.Run(rol, func(t *testing.T) {
			h := pgtest.New(t, db.Migrations, db.MigrationsDir)
			super := h.PoolSinRLS(t)
			if rol == "rol de aplicación" {
				fn(t, New(h.Pool), super)
				return
			}
			fn(t, New(super), super)
		})
	}
}
