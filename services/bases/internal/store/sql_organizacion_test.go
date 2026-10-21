// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	"git.sytue.eu/sytue/one/services/bases/db"
)

// TestElSQLLlevaLaOrganizacion: toda sentencia de Sytue Bases que lee o modifica una tabla
// con `tenant` lo nombra en su predicado. Bases ya lo hacía (la auditoría del 2026-10-06
// pasó su batería con un rol que elude la RLS); este test lo deja como red para lo que se
// escriba después. Ver platform/pkg/sqlorg.
func TestElSQLLlevaLaOrganizacion(t *testing.T) {
	sqlorg.Exigir(t, sqlorg.Opciones{
		Migraciones: db.Migrations,
		Dir:         db.MigrationsDir,
		Paquetes:    []string{".", "../http"},
		Excepciones: map[string]string{
			// La credencial ES el token: se busca por su hash, sin saber aún de qué organización
			// es, y todo lo demás se lee ya con la organización de la fila encontrada.
			"conector.go:LeerPorConector":  "el token de una conexión se resuelve por su hash antes de saber de qué organización es",
			"formularios.go:resolverToken": "el token de un formulario público se resuelve por su hash antes de saber de qué organización es",
			// La única lectura entre organizaciones de Bases (ADR 0068): las entradas `external` de UNA persona.
			"externos.go:anfitrionas": "en qué organizaciones es invitada una persona: sus entradas `external`, de todas",
			// Colas de mantenimiento en segundo plano.
			"adjuntos.go:PurgarAdjuntos":       "la purga de adjuntos recorre la cola de todas las organizaciones",
			"formulas.go:RecalcularPendientes": "el recálculo de fórmulas recorre la cola de todas las organizaciones",
		},
	})
}
