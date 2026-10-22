// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package delivery

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/db"
)

// Los avisos de correo que se reconstruyen: solo lo que ENTRÓ por SMTP (el outbox),
// fuera de Spam, sin borrar, y a quien es dueño de la cuenta que lo recibió. Nadie
// recibe el aviso de un correo de otro, ni de otra organización.
func TestAvisosHistoricosDeCorreo(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	acme := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	globex := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "globex"})
	entro := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)

	type cuenta struct{ id, inbox, junk string }
	crear := func(ctx context.Context, tenant, sub string) (c cuenta) {
		t.Helper()
		err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, `INSERT INTO accounts (tenant, subject, address) VALUES ($1, $2, $2 || '@x.es') RETURNING id::text`,
				tenant, sub).Scan(&c.id); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO mailboxes (tenant, account_id, name, role) VALUES ($1, $2, 'INBOX', 'inbox') RETURNING id::text`,
				tenant, c.id).Scan(&c.inbox); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `INSERT INTO mailboxes (tenant, account_id, name, role) VALUES ($1, $2, 'Junk', 'junk') RETURNING id::text`,
				tenant, c.id).Scan(&c.junk)
		})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	// mensaje guarda un correo en `ahora` y, si `entrega` no está vacío, el apunte de
	// su entrega SMTP en ese buzón y a nombre de la cuenta `a`.
	mensaje := func(ctx context.Context, tenant string, dueno cuenta, ahora string, a cuenta, entrega string, borrado bool) string {
		t.Helper()
		var id string
		err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, `
				INSERT INTO messages (tenant, account_id, mailbox_id, uid, blob_key, subject, from_addr, deleted_at)
				VALUES ($1, $2, $3, (SELECT count(*) + 1 FROM messages), 'b', 'hola', 'bea@y.es',
				        CASE WHEN $4 THEN now() END)
				RETURNING id::text`, tenant, dueno.id, ahora, borrado).Scan(&id); err != nil {
				return err
			}
			if entrega == "" {
				return nil
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO events_outbox (id, channel, type, tenant, payload, created_at)
				VALUES (gen_random_uuid(), 'mail', 'mail.message.received', $1,
				        jsonb_build_object('account_id', $2::text, 'message_id', $3::text, 'mailbox_id', $4::text), $5)`,
				tenant, a.id, id, entrega, entro)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	ana, bea := crear(acme, "acme", "ana"), crear(acme, "acme", "bea")
	carla := crear(globex, "globex", "carla")
	bueno := mensaje(acme, "acme", ana, ana.inbox, ana, ana.inbox, false)
	mensaje(acme, "acme", ana, ana.junk, ana, ana.junk, false)   // entró en Spam
	mensaje(acme, "acme", ana, ana.junk, ana, ana.inbox, false)  // y este se movió a Spam
	mensaje(acme, "acme", ana, ana.inbox, ana, "", false)        // importado: no entró por SMTP
	mensaje(acme, "acme", ana, ana.inbox, ana, ana.inbox, true)  // borrado
	mensaje(acme, "acme", ana, ana.inbox, bea, bea.inbox, false) // apunte que no casa con su cuenta
	deBea := mensaje(acme, "acme", bea, bea.inbox, bea, bea.inbox, false)
	mensaje(globex, "globex", carla, carla.inbox, carla, carla.inbox, false)

	avisos, err := AvisosHistoricos(acme, h.Pool, "https://mail.sytue.eu")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(avisos))
	for _, a := range avisos {
		got = append(got, a.Subject+" "+a.Clave)
		if !a.Fecha.Equal(entro) || a.Type != "mail.message.received" {
			t.Errorf("aviso sin la fecha de entrada o con otro tipo: %+v", a)
		}
	}
	quiero := []string{"ana mail:" + bueno, "bea mail:" + deBea}
	slices.Sort(got)
	slices.Sort(quiero)
	if !slices.Equal(got, quiero) {
		t.Fatalf("avisos %v, quiero %v", got, quiero)
	}
}
