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
	roomsdb "git.sytue.eu/sytue/one/services/rooms/db"
)

// Los avisos de Rooms que se reconstruyen van a quien los habría recibido en vivo —el
// otro miembro de un DM y los mencionados que son miembros—, respetando el nivel de
// aviso de cada canal, y nunca al autor, a quien no es miembro, a quien entró después
// ni a nadie de otra organización.
func TestAvisosHistoricosDeRooms(t *testing.T) {
	h := pgtest.New(t, roomsdb.Migrations, roomsdb.MigrationsDir)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	escrito := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	antes, despues := escrito.Add(-time.Hour), escrito.Add(time.Hour)

	var dm, general, mudo string
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var team string
		if err := tx.QueryRow(ctx, `INSERT INTO teams (tenant, slug, name) VALUES ('acme', 't', 'T') RETURNING id::text`).Scan(&team); err != nil {
			return err
		}
		canal := func(slug, kind string, miembros map[string]time.Time, nivel map[string]string) (string, error) {
			var id string
			if err := tx.QueryRow(ctx, `INSERT INTO channels (tenant, team_id, slug, name, kind) VALUES ('acme', $1, $2, $2, $3) RETURNING id::text`,
				team, slug, kind).Scan(&id); err != nil {
				return "", err
			}
			for sub, desde := range miembros {
				lvl := nivel[sub]
				if lvl == "" {
					lvl = "all"
				}
				if _, err := tx.Exec(ctx, `INSERT INTO channel_members (tenant, channel_id, subject, joined_at, notify_level) VALUES ('acme', $1, $2, $3, $4)`,
					id, sub, desde, lvl); err != nil {
					return "", err
				}
			}
			return id, nil
		}
		var err error
		if dm, err = canal("dm", "direct", map[string]time.Time{"ana": antes, "bea": antes}, nil); err != nil {
			return err
		}
		if general, err = canal("general", "public", map[string]time.Time{"ana": antes, "bea": antes, "carla": antes, "dani": despues}, nil); err != nil {
			return err
		}
		if mudo, err = canal("mudo", "public", map[string]time.Time{"ana": antes, "bea": antes}, map[string]string{"bea": "none"}); err != nil {
			return err
		}
		mensaje := func(canal, autor string, menciones []string, borrado bool) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO messages (tenant, channel_id, author_sub, author_name, body, mentions, created_at, deleted_at)
				VALUES ('acme', $1, $2, $2, 'hola', coalesce($3::text[], '{}'), $4, CASE WHEN $5 THEN now() END)`, canal, autor, menciones, escrito, borrado)
			return err
		}
		for _, m := range []struct {
			canal, autor string
			menciones    []string
			borrado      bool
		}{
			{dm, "ana", nil, false},                                   // → bea (DM)
			{general, "ana", []string{"carla", "dani", "eva"}, false}, // → carla; dani entró después, eva no es miembro
			{general, "ana", []string{"ana"}, false},                  // mencionarse a sí mismo: nada
			{general, "bea", nil, false},                              // canal sin mención: nada
			{mudo, "ana", []string{"bea"}, false},                     // bea lo tiene silenciado
			{dm, "bea", nil, true},                                    // borrado
			{dm, "bot:alertas", nil, false},                           // integración
		} {
			if err := mensaje(m.canal, m.autor, m.menciones, m.borrado); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Otra organización con su propio DM: no puede salir aquí.
	globex := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "globex"})
	if err := h.Pool.InTenantTx(globex, func(tx pgx.Tx) error {
		_, e := tx.Exec(globex, `
			WITH t AS (INSERT INTO teams (tenant, slug, name) VALUES ('globex', 't', 'T') RETURNING id),
			     c AS (INSERT INTO channels (tenant, team_id, slug, name, kind) SELECT 'globex', id, 'dm', 'dm', 'direct' FROM t RETURNING id),
			     m AS (INSERT INTO channel_members (tenant, channel_id, subject, joined_at) SELECT 'globex', id, 'zoe', $1 FROM c)
			INSERT INTO messages (tenant, channel_id, author_sub, body, created_at) SELECT 'globex', id, 'yago', 'hola', $2 FROM c`,
			antes, escrito)
		return e
	}); err != nil {
		t.Fatal(err)
	}

	avisos, err := AvisosHistoricos(ctx, h.Pool, "https://rooms.sytue.eu")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(avisos))
	for _, a := range avisos {
		got = append(got, a.Subject+" "+a.Type)
		if !a.Fecha.Equal(escrito) {
			t.Errorf("sin la fecha del mensaje: %+v", a)
		}
	}
	slices.Sort(got)
	if quiero := []string{"bea rooms.message", "carla rooms.message"}; !slices.Equal(got, quiero) {
		t.Fatalf("avisos %v, quiero %v", got, quiero)
	}
}
