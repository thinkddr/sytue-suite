// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	"git.sytue.eu/sytue/one/services/mail/db"
)

// TestElSQLLlevaLaOrganizacion: toda sentencia de Sytue Mail que lee o modifica una
// tabla con `tenant` lo nombra en su predicado. La política RLS no basta —un rol que
// se la salta lee la fila de cualquiera con un `WHERE id = $1`—, así que el filtro
// tiene que estar escrito en la consulta. Ver platform/pkg/sqlorg.
func TestElSQLLlevaLaOrganizacion(t *testing.T) {
	sqlorg.Exigir(t, sqlorg.Opciones{
		Migraciones: db.Migrations,
		Dir:         db.MigrationsDir,
		Paquetes:    []string{"."},
		Excepciones: map[string]string{
			"aliases.go:addressTaken": "una dirección es única en TODA la instalación: se mira si la tiene alguien de cualquier organización, y solo se devuelve sí o no",
			"store.go:LookupAddress":  "el MX resuelve a qué cuenta va un destinatario ANTES de saber de qué organización es",
			"store.go:Dominios":       "los dominios de correo que sirve la instalación entera, para Calendar; servicio a servicio",
			"blobs.go:soltarBlobs":    "un mismo mensaje guardado lo comparten buzones de varias organizaciones: antes de borrar el objeto se mira si lo referencia alguno, de cualquiera",
			"queue.go:ClaimDue":       "el trabajador de la cola de salida reclama los envíos pendientes de todas las organizaciones",
			"queue.go:MarkSent":       "el trabajador de la cola marca por id el envío que acaba de reclamar",
			"queue.go:MarkDeferred":   "el trabajador de la cola marca por id el envío que acaba de reclamar",
			"queue.go:MarkBounced":    "el trabajador de la cola marca por id el envío que acaba de reclamar",
			"queue.go:PromoteDueHeld": "el trabajador libera los envíos programados vencidos de todas las organizaciones; fija la organización de cada fila antes de escribir en su buzón",
		},
	})
}
