// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/db"
)

// «Compartido contigo» se reconstruye solo si la persona SIGUE pudiendo abrirlo: ni
// revocado, ni caducado, ni en la papelera, ni un enlace, ni a quien lo compartió.
func TestAvisosHistoricosDeDrive(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	cuando := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.owner_sub', 'ana', true)"); err != nil {
			return err
		}
		var raiz string
		if err := tx.QueryRow(ctx, `INSERT INTO folders (tenant, name, is_root, owner_sub) VALUES ('acme', '', true, 'ana') RETURNING id::text`).Scan(&raiz); err != nil {
			return err
		}
		fichero := func(nombre string, papelera bool) (string, error) {
			var id string
			err := tx.QueryRow(ctx, `INSERT INTO files (tenant, folder_id, name, storage_key, owner_sub, deleted_at)
				VALUES ('acme', $1, $2, $2, 'ana', CASE WHEN $3 THEN now() END) RETURNING id::text`, raiz, nombre, papelera).Scan(&id)
			return id, err
		}
		vivo, err := fichero("plan.pdf", false)
		if err != nil {
			return err
		}
		tirado, err := fichero("viejo.pdf", true)
		if err != nil {
			return err
		}
		for _, s := range []struct{ fichero, kind, para, extra string }{
			{vivo, "user", "bea", ""},
			{vivo, "user", "carla", "revocado"},
			{vivo, "user", "dani", "caducado"},
			{vivo, "user", "ana", ""}, // consigo misma
			{vivo, "group", "equipo", ""},
			{tirado, "user", "eva", ""},
		} {
			if _, err := tx.Exec(ctx, `INSERT INTO shares (tenant, file_id, kind, subject, created_by, created_at, revoked_at, expires_at)
				VALUES ('acme', $1, $2, $3, 'ana', $4, CASE WHEN $5 = 'revocado' THEN now() END,
				        CASE WHEN $5 = 'caducado' THEN now() - interval '1 day' END)`,
				s.fichero, s.kind, s.para, cuando, s.extra); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	avisos, err := AvisosHistoricos(ctx, h.Pool, "https://drive.sytue.eu")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(avisos))
	for _, a := range avisos {
		got = append(got, a.Subject+" "+a.Params["name"])
		if !a.Fecha.Equal(cuando) {
			t.Errorf("sin la fecha en que se compartió: %+v", a)
		}
	}
	if !slices.Equal(got, []string{"bea plan.pdf"}) {
		t.Fatalf("avisos %v, quiero solo el de Bea", got)
	}
}
