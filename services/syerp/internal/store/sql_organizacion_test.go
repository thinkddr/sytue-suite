// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	"git.sytue.eu/sytue/one/services/syerp/db"
)

// TestElSQLLlevaLaOrganizacion: toda sentencia del ERP que lee o modifica una
// tabla con `tenant` lo nombra en su predicado. La política RLS no basta —un rol
// que se la salta lee la factura de cualquiera con un `WHERE id = $1`—, así que
// el filtro tiene que estar escrito en la consulta. Ver platform/pkg/sqlorg.
func TestElSQLLlevaLaOrganizacion(t *testing.T) {
	sqlorg.Exigir(t, sqlorg.Opciones{
		Migraciones: db.Migrations,
		Dir:         db.MigrationsDir,
		Paquetes:    []string{".", "../http"},
		Excepciones: map[string]string{
			"compartido.go:TenantsConPerfil":            "el avisador de obligaciones recorre TODAS las organizaciones con perfil; corre en segundo plano, en transacción de mantenimiento, y solo devuelve slugs",
			"verifactu.go:TenantsConVerifactuPendiente": "el remitente a la AEAT recorre TODAS las organizaciones con registros pendientes; en segundo plano, en transacción de mantenimiento, y solo devuelve slugs",
		},
	})
}
