// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package merge

import (
	"context"
	"io/fs"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	castdb "git.sytue.eu/sytue/one/services/cast/db"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
	drivedb "git.sytue.eu/sytue/one/services/drive/db"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	roomsdb "git.sytue.eu/sytue/one/services/rooms/db"
)

var esquemas = map[string]struct {
	fsys fs.FS
	dir  string
}{
	"hub":   {hubdb.Migrations, hubdb.MigrationsDir},
	"rooms": {roomsdb.Migrations, roomsdb.MigrationsDir},
	"drive": {drivedb.Migrations, drivedb.MigrationsDir},
	"docs":  {docsdb.Migrations, docsdb.MigrationsDir},
	"cast":  {castdb.Migrations, castdb.MigrationsDir},
}

// Cada tabla de DatosDeLaPersona existe, tiene su `subject` y su política de
// fusión. Sin esto un renombrado dejaría el borrado de «Borrar mi cuenta» en un
// no-op mudo: el DELETE fallaría —y el barrido reintentaría para siempre— o, sin
// política, borraría cero filas diciendo que ha terminado.
func TestDatosDeLaPersonaExistenYSeBorran(t *testing.T) {
	// Y al revés: un servicio cuyo esquema está aquí porque guarda datos de la
	// persona no puede desaparecer de la lista sin que esto se ponga en rojo.
	for svc := range esquemas {
		if !slices.ContainsFunc(DatosDeLaPersona, func(d struct {
			Servicio string
			Tablas   []string
		},
		) bool {
			return d.Servicio == svc
		}) {
			t.Errorf("%s ya no borra nada con la cuenta", svc)
		}
	}
	for _, d := range DatosDeLaPersona {
		t.Run(d.Servicio, func(t *testing.T) {
			e, ok := esquemas[d.Servicio]
			if !ok {
				t.Fatalf("falta el esquema de %s en este test", d.Servicio)
			}
			h := pgtest.New(t, e.fsys, e.dir)
			ctx := context.Background()
			tablas, sinPolitica, err := tenantTables(ctx, h.Pool)
			if err != nil {
				t.Fatal(err)
			}
			for _, tb := range d.Tablas {
				// Con `tenant`, la misma comprobación que la fusión; sin él (una tabla de
				// la persona, como los bloqueos de Cast), su política por nombre.
				var politica bool
				if err := h.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_policies
					WHERE schemaname = current_schema() AND tablename = $1 AND policyname = $1 || '_platform_merge')`, tb).Scan(&politica); err != nil {
					t.Fatal(err)
				}
				if slices.Contains(sinPolitica, tb) || (!slices.Contains(tablas, tb) && !politica) {
					t.Fatalf("%s.%s no existe con su política de fusión", d.Servicio, tb)
				}
				var tiene bool
				if err := h.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema() AND table_name = $1 AND column_name = 'subject')`, tb).Scan(&tiene); err != nil || !tiene {
					t.Fatalf("%s.%s sin columna subject (%v)", d.Servicio, tb, err)
				}
			}
			if d.Servicio != "hub" && d.Servicio != "cast" {
				return
			}
			// En el Hub y en Cast, además, se borra lo de una persona y solo lo suyo.
			// La siembra va como superusuario; el borrado, con el rol de la aplicación
			// (sin BYPASSRLS), que es donde un DELETE sin app.merge borraría cero filas.
			admin := h.PoolSinRLS(t)
			if d.Servicio == "cast" {
				// Sus bloqueos se van; el que otra persona le puso a ella es de esa
				// persona y se queda en su lista.
				if _, err := admin.Exec(ctx, `INSERT INTO person_blocks (blocker_sub, blocked_sub)
					VALUES ('se-va', 'otra'), ('se-va', 'otra-mas'), ('otra', 'se-va')`); err != nil {
					t.Fatalf("sembrar: %v", err)
				}
				if n, err := BorrarPersona(ctx, h.Pool, d.Tablas, "se-va"); err != nil || n != 2 {
					t.Fatalf("BorrarPersona en Cast: %d filas, %v; quiero 2", n, err)
				}
				var suyos, deOtra int
				_ = admin.QueryRow(ctx, `SELECT count(*) FILTER (WHERE blocker_sub = 'se-va'),
					count(*) FILTER (WHERE blocker_sub = 'otra') FROM person_blocks`).Scan(&suyos, &deOtra)
				if suyos != 0 || deOtra != 1 {
					t.Fatalf("tras borrar: %d suyos (quiero 0), %d de otra persona (quiero 1)", suyos, deOtra)
				}
				return
			}
			for _, s := range []string{"se-va", "se-queda"} {
				if _, err := admin.Exec(ctx, `INSERT INTO notifications (tenant, subject, source, type, title)
					VALUES ('acme', $1, 'mail', 'x', 't'), ('otra', $1, 'mail', 'x', 't')`, s); err != nil {
					t.Fatalf("sembrar: %v", err)
				}
			}
			n, err := BorrarPersona(ctx, h.Pool, d.Tablas, "se-va")
			if err != nil || n != 2 {
				t.Fatalf("BorrarPersona: %d filas, %v; quiero 2", n, err)
			}
			var quedan, suyas int
			_ = admin.QueryRow(ctx, `SELECT count(*) FILTER (WHERE subject = 'se-queda'),
				count(*) FILTER (WHERE subject = 'se-va') FROM notifications`).Scan(&quedan, &suyas)
			if quedan != 2 || suyas != 0 {
				t.Fatalf("tras borrar: %d de otra persona (quiero 2), %d suyas (quiero 0)", quedan, suyas)
			}
		})
	}
}
