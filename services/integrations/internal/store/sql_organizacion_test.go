// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
)

// TestElSQLLlevaLaOrganizacion (NEG-22): toda sentencia de Integraciones que lee o
// modifica una tabla con `org_id` lo nombra en su predicado. La política RLS no
// basta —un rol que se la salta lee la fila de cualquiera con un `WHERE id = $1`—,
// así que el filtro tiene que estar escrito en la consulta. Ver platform/pkg/sqlorg.
func TestElSQLLlevaLaOrganizacion(t *testing.T) {
	sqlorg.Exigir(t, sqlorg.Opciones{
		Migraciones: integrationsdb.Migrations,
		Dir:         integrationsdb.MigrationsDir,
		Columna:     "org_id",
		Paquetes:    []string{"."},
		Excepciones: map[string]string{
			"scim.go:ListSyncCandidates": "el barrido de sincronización recorre las conexiones activas de todas las organizaciones, en segundo plano (puerta de mantenimiento)",
		},
	})
}
