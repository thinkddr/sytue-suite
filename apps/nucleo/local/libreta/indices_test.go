// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// Las consultas de cada operación van por índice: ninguna recorre la tabla fila ni la de
// contactos entera salvo la búsqueda, que recorre el índice de los que se ven (en orden,
// y para en cuanto tiene los que pide). Es lo que hace que dar de alta, editar o pedir
// una página no crezca con la libreta (la prueba de carga lo mide).
func TestLasConsultasVanPorIndice(t *testing.T) {
	dir := t.TempDir()
	x, err := libreta.Abrir(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, libreta.Fichero)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for q, quiero := range libreta.ConsultasConIndice {
		plan := planDe(t, db, q)
		todo := strings.Join(plan, " | ")
		for _, p := range plan {
			if strings.HasPrefix(p, "SCAN ") && !strings.Contains(p, "USING") && !strings.Contains(p, "CONSTANT ROW") {
				t.Errorf("%s\n  recorre una tabla entera: %s", q, todo)
			}
		}
		if !strings.Contains(todo, quiero) {
			t.Errorf("%s\n  no usa %s: %s", q, quiero, todo)
		}
	}
}

// planDe es el plan de q, paso a paso.
func planDe(t *testing.T, db *sql.DB, q string) []string {
	t.Helper()
	rs, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+q, make([]any, strings.Count(q, "?"))...)
	if err != nil {
		t.Fatal(q, err)
	}
	defer rs.Close()
	var plan []string
	for rs.Next() {
		var id, padre, nada int
		var detalle string
		if err := rs.Scan(&id, &padre, &nada, &detalle); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detalle)
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}
	return plan
}
