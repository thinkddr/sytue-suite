// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	"git.sytue.eu/sytue/one/services/analytics/db"
)

// TestElSQLLlevaLaOrganizacion: toda sentencia de Sytue Analytics que lee o modifica una
// tabla con `tenant` lo nombra en su predicado. La política RLS no basta —un rol que
// se la salta lee la fila de cualquiera con un `WHERE id = $1`—, así que el filtro
// tiene que estar escrito en la consulta. Ver platform/pkg/sqlorg.
func TestElSQLLlevaLaOrganizacion(t *testing.T) {
	sqlorg.Exigir(t, sqlorg.Opciones{
		Migraciones: db.Migrations,
		Dir:         db.MigrationsDir,
		Paquetes:    []string{"."},
		Excepciones: map[string]string{
			"rollup.go:rollupDay": "el agregador recalcula el día de TODAS las organizaciones a la vez, en segundo plano y en transacción de mantenimiento; cada fila que escribe lleva la organización de su evento",
			"store.go:SiteByHost": "el colector resuelve el sitio por la cabecera Host ANTES de saber de qué organización es; solo lectura y solo con el permiso del colector",
		},
	})
}
