// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
)

// TestElSQLLlevaLaOrganizacion: toda sentencia de Sytue Hub que lee o modifica una
// tabla con `tenant` lo nombra en su predicado. La política RLS no basta —un rol que
// se la salta lee la fila de cualquiera con un `WHERE id = $1`—, así que el filtro
// tiene que estar escrito en la consulta. Ver platform/pkg/sqlorg.
func TestElSQLLlevaLaOrganizacion(t *testing.T) {
	sqlorg.Exigir(t, sqlorg.Opciones{
		Migraciones: hubdb.Migrations,
		Dir:         hubdb.MigrationsDir,
		Paquetes:    []string{"."},
		Excepciones: map[string]string{
			"notifications.go:DeLaPersona":      "cuenta los avisos de UNA persona en todas sus organizaciones para el tope por destinatario (ADR 0053); solo devuelve un número",
			"notifications.go:PrefsDeLaPersona": "lee las reglas de silencio de UNA persona en todas sus organizaciones para decidir si se le entrega un aviso (ADR 0053); no sale hacia fuera",
			"notifications.go:OlvidarBuzon":     "Sytue ID retira una dirección y sus avisos se borran en todas las organizaciones; ruta interna, en transacción de mantenimiento",
			"notifications.go:PurgeReadBefore":  "retención: purga los avisos leídos antiguos de todas las organizaciones, en segundo plano",
			"presence.go:PurgeStalePresence":    "retención: purga la presencia caducada de todas las organizaciones, en segundo plano",
		},
	})
}
