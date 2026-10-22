// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// La función SQL plegar() dice lo mismo que buscar.Plegar (Fyne y servicios) y que
// plegar de @sytue/i18n (web), con los MISMOS casos. Si divergen, una búsqueda
// encuentra en la web lo que no encuentra en el servidor.
func TestPlegarSQLComoGo(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	b, err := os.ReadFile("../../../../apps/nucleo/buscar/testdata/casos.json")
	if err != nil {
		t.Fatal(err)
	}
	var casos [][2]string
	if err := json.Unmarshal(b, &casos); err != nil {
		t.Fatal(err)
	}
	for _, c := range casos {
		var got string
		if err := h.Pool.QueryRow(context.Background(), `SELECT plegar($1)`, c[0]).Scan(&got); err != nil {
			t.Fatalf("plegar(%q): %v", c[0], err)
		}
		if got != c[1] || got != buscar.Plegar(c[0]) {
			t.Errorf("SQL plegar(%q) = %q; Go = %q; quiero %q", c[0], got, buscar.Plegar(c[0]), c[1])
		}
	}
}

// Buscar en Drive sin acentos ni mayúsculas, en los dos sentidos, y POR ÍNDICE: la
// expresión del índice y la de la consulta tienen que ser la misma, o la búsqueda
// recorre todos los ficheros de la organización.
func TestSearchFilesSinAcentos(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	acta := seedFile(t, ctx, st, root.ID, "Acta de la reunión.pdf", 1)
	seedFile(t, ctx, st, root.ID, "Informe Müller.docx", 1)
	seedFile(t, ctx, st, root.ID, "cafe pequeno.txt", 1)
	for q, quiero := range map[string]string{
		"reunion":      "Acta de la reunión.pdf",
		"REUNIÓN":      "Acta de la reunión.pdf",
		"muller":       "Informe Müller.docx",
		"Müller":       "Informe Müller.docx",
		"Café Pequeño": "cafe pequeno.txt",
	} {
		res, err := st.SearchFiles(ctx, q, 10)
		if err != nil {
			t.Fatalf("SearchFiles(%q): %v", q, err)
		}
		if len(res) != 1 || res[0].Name != quiero {
			t.Fatalf("SearchFiles(%q) = %+v, quiero %q", q, res, quiero)
		}
	}

	// Con tres ficheros cualquier plan vale; con miles del mismo tenant, el índice por
	// tenant deja de acotar y solo el de trigramas plegado sirve. Se clonan filas del
	// acta con otro nombre y otra clave.
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO files
			SELECT (json_populate_record(f, json_build_object(
			          'id', gen_random_uuid(), 'name', 'relleno ' || g || '.txt',
			          'storage_key', 'files/relleno-' || g))).*
			  FROM files f, generate_series(1, 5000) g WHERE f.id = $1`, acta.ID)
		return err
	}); err != nil {
		t.Fatalf("relleno: %v", err)
	}
	// ANALYZE con el rol dueño: el de aplicación no puede y Postgres solo lo avisa.
	dueno, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer dueno.Close(ctx)
	if _, err := dueno.Exec(ctx, `ANALYZE files`); err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}
	// El plan se pide como DUEÑO, sin RLS: con RLS forzada, Postgres no usa un índice
	// de trigramas para el rol de aplicación, porque LIKE no es LEAKPROOF (medido; ver
	// la migración 0028). Lo que se comprueba es que la expresión del índice y la de
	// la consulta son la misma, que es lo que un cambio en una de las dos rompe.
	var plan strings.Builder
	rows, err := dueno.Query(ctx, `EXPLAIN SELECT id FROM files
		WHERE tenant='acme' AND deleted_at IS NULL AND channel_id IS NULL AND plegar(name) LIKE plegar('%reunion%')`)
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(l + "\n")
	}
	rows.Close()
	if !strings.Contains(plan.String(), "files_name_plegado_trgm_idx") {
		t.Fatalf("la búsqueda no usa el índice plegado:\n%s", plan.String())
	}
}
