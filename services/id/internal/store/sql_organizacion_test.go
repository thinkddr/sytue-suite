// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	"git.sytue.eu/sytue/one/services/id/db"
)

// TestElDirectorioYSusAvisosLlevanLaOrganizacion: el listado del directorio de
// la consola (con su filtro de estado) y los recuentos de la portada que enlazan
// a él nombran la organización en el predicado. Sytue ID todavía no pasa entero
// por sqlorg —sesiones, dispositivos y tokens son del usuario, no de una
// organización, y cada uno pide su porqué—, así que aquí se vigilan por nombre
// las funciones que leen a la gente de UNA organización. Ver platform/pkg/sqlorg.
func TestElDirectorioYSusAvisosLlevanLaOrganizacion(t *testing.T) {
	hallazgos, err := sqlorg.Revisar(sqlorg.Opciones{
		Migraciones: db.Migrations,
		Dir:         db.MigrationsDir,
		Columna:     "org_id",
		Paquetes:    []string{"."},
	})
	if err != nil {
		t.Fatalf("sqlorg: %v", err)
	}
	vigiladas := map[string]bool{
		"users.go:ListOrgUsersPage":  true,
		"users.go:condicionFiltro":   true,
		"insights.go:OrgInsightsFor": true,
	}
	for _, h := range hallazgos {
		if vigiladas[h.Clave()] {
			t.Error(h)
		}
	}
}
