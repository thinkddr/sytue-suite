// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Invariante de aislamiento: TODA tabla por organización va con RLS habilitada Y FORZADA.
// El porqué está entero en pgtest.ExigirRLSForzada —en resumen: el rol de la aplicación es
// el PROPIETARIO de las tablas y ENABLE no se aplica al propietario, así que sin FORCE la
// política no llega a evaluarse—. Se comprueba por CATÁLOGO, que dice lo que la base hace,
// y no leyendo el SQL, que es lo que el fichero parece decir.
//
// Sin excepciones: hub no tiene bandeja de salida de eventos —es el consumidor, no el
// productor—, así que las tres tablas entran al invariante sin exención ninguna.
package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
)

func TestRLSForzadaEnTodasLasTablas(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	pgtest.ExigirRLSForzada(t, h, nil)
}
