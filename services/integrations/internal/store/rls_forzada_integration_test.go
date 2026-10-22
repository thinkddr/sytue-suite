// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Invariante de aislamiento: TODA tabla por organización va con RLS habilitada Y FORZADA.
// El porqué está entero en pgtest.ExigirRLSForzada —en resumen: el rol de la aplicación es
// el PROPIETARIO de las tablas y ENABLE no se aplica al propietario, así que sin FORCE la
// política no llega a evaluarse—. Se comprueba por CATÁLOGO, que dice lo que la base hace,
// y no leyendo el SQL, que es lo que el fichero parece decir.
//
// Aquí la columna se llama `org_id`, no `tenant` (las políticas la comparan igual contra
// `app.tenant`). Es el servicio por el que el invariante tuvo que aprender los dos nombres:
// mirando solo `tenant` cubría UNA de sus quince tablas y las once por organización —con
// los secretos de las instancias entre ellas— no las miraba nadie.
package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
)

func TestRLSForzadaEnTodasLasTablas(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	pgtest.ExigirRLSForzada(t, h, map[string]string{
		// El despachador de eventos la recorre CROSS-TENANT a propósito: un solo worker
		// publica los eventos de todas las organizaciones (platform/pkg/events). Con
		// FORCE dejaría de ver las filas y se pararía el despacho de toda la suite.
		"events_outbox": "la lee el despachador de eventos cross-tenant, por diseño",
	})
}
