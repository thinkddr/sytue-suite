// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Invariante de aislamiento: TODA tabla con columna `tenant` va con RLS habilitada Y
// FORZADA. El porqué está en pgtest.ExigirRLSForzada: el rol de la aplicación es el
// PROPIETARIO de las tablas —las migraciones corren con la misma DatabaseURL que sirve
// las peticiones— y ENABLE no se aplica al propietario, así que sin FORCE la política de
// aislamiento no llega a evaluarse.
//
// Por CATÁLOGO —lo que la base hace— y no leyendo el SQL, que es lo que parece decir:
// auditando a mano con un grep salieron tablas «sin proteger» que sí lo estaban, porque
// la declaración llevaba dos espacios («FORCE  ROW») y el patrón no casaba.
package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
)

func TestRLSForzadaEnTodasLasTablas(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	pgtest.ExigirRLSForzada(t, h, map[string]string{
		// El despachador de eventos la recorre CROSS-TENANT a propósito: un solo worker
		// publica los eventos de todas las organizaciones (platform/pkg/events). Con
		// FORCE dejaría de ver las filas y se pararía el despacho de toda la suite.
		"events_outbox": "la lee el despachador de eventos cross-tenant, por diseño",
	})
}
