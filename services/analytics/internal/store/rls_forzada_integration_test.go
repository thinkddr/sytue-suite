// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Invariante de aislamiento: TODA tabla por organización va con RLS habilitada Y FORZADA.
// El porqué está entero en pgtest.ExigirRLSForzada —en resumen: el rol de la aplicación es
// el PROPIETARIO de las tablas y ENABLE no se aplica al propietario, así que sin FORCE la
// política no llega a evaluarse—. Se comprueba por CATÁLOGO, que dice lo que la base hace,
// y no leyendo el SQL, que es lo que el fichero parece decir.
//
// Sin excepciones: analytics no tiene bandeja de salida de eventos. Y es el servicio por el
// que el invariante tuvo que mirar también las tablas PARTICIONADAS: `analytics_events` lo
// es, y con el filtro que solo aceptaba `relkind='r'` el catálogo ni la devolvía —la tabla
// con más escrituras de la plataforma se quedaba fuera de la comprobación—.
package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/analytics/db"
)

func TestRLSForzadaEnTodasLasTablas(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	pgtest.ExigirRLSForzada(t, h, nil)
}
