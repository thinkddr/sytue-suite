// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	"git.sytue.eu/sytue/one/services/drive/db"
)

// TestElSQLLlevaLaOrganizacion: toda sentencia que lee o modifica una tabla con
// `tenant` lo nombra en su predicado. La política RLS no basta —un rol que se la
// salta lee la fila de cualquiera con un `WHERE id = $1`—, así que el filtro tiene
// que estar escrito en la consulta. Ver platform/pkg/sqlorg.
func TestElSQLLlevaLaOrganizacion(t *testing.T) {
	sqlorg.Exigir(t, sqlorg.Opciones{
		Migraciones: db.Migrations,
		Dir:         db.MigrationsDir,
		Paquetes:    []string{"."},
		Excepciones: map[string]string{
			"blobs.go:PurgarBlobsCandidatos":      "un blob compartido (deduplicado entre organizaciones) solo se borra si NINGUNA lo usa: la pregunta es global a propósito (app.maintenance)",
			"docssync.go:AplazarPendientesDocs":   "el trabajador de la papelera hacia Docs vacía la cola de todas las organizaciones (app.maintenance)",
			"subidas.go:ReclamarSubidasCaducadas": "el barrido recoge las URL de subida caducadas de todas las organizaciones (app.maintenance)",
			"wopilocks.go:PurgeExpiredWopiLocks":  "el barrido periódico borra los bloqueos WOPI vencidos de todas las organizaciones (app.maintenance)",
		},
	})
}
