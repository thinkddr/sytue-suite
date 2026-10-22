// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	"git.sytue.eu/sytue/one/services/calendar/db"
)

// TestElSQLLlevaLaOrganizacion: toda sentencia de Sytue Calendar que lee o modifica una
// tabla con `tenant` lo nombra en su predicado. La política RLS no basta —un rol que
// se la salta lee la fila de cualquiera con un `WHERE id = $1`—, así que el filtro
// tiene que estar escrito en la consulta. Ver platform/pkg/sqlorg.
func TestElSQLLlevaLaOrganizacion(t *testing.T) {
	sqlorg.Exigir(t, sqlorg.Opciones{
		Migraciones: db.Migrations,
		Dir:         db.MigrationsDir,
		Paquetes:    []string{"."},
		Excepciones: map[string]string{
			"reminders.go:ClaimDueReminders":         "el trabajador de recordatorios reclama los vencidos de todas las organizaciones, en segundo plano",
			"subscriptions.go:ClaimDueSubscriptions": "el trabajador de suscripciones ICS reclama las que toca refrescar de todas las organizaciones, en segundo plano",
			// COM-15: solo lectura, siempre por las direcciones personales y el `sub` del token.
			"invitaciones_de_fuera.go:InvitacionesDeFuera":      "la bandeja de una persona busca sus invitaciones en las OTRAS organizaciones",
			"invitaciones_de_fuera.go:OrganizacionDeInvitacion": "responder una invitación de otra organización busca en cuál vive",
		},
	})
}
