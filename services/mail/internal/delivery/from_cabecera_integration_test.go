// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package delivery

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// COM-04 contra la base de verdad: por `Outbound.Submit` —el camino del SMTP y de
// `/send`— un mensaje cuyo `From:` es de otra cuenta no llega ni a firmarse ni a
// guardarse. El almacén de objetos va a nil a propósito: si la verja no cortara antes,
// la prueba reventaría al subir el mensaje.
func TestSubmitRechazaUnFromQueNoEsDeLaCuenta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	acme := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	globex := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "globex"})
	st := store.New(h.Pool)

	ana, err := st.EnsureAccount(acme, "sub-ana", "ana@acme.sytue.eu", "Ana", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureAccount(acme, "sub-bea", "bea@acme.sytue.eu", "Bea", 1<<30); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureAccount(globex, "sub-eva", "eva@globex.sytue.eu", "Eva", 1<<30); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(acme, func(tx pgx.Tx) error {
		_, e := tx.Exec(acme, `INSERT INTO aliases (tenant, account_id, address) VALUES ('acme', $1, 'ana.garcia@acme.sytue.eu')`, ana.ID)
		return e
	}); err != nil {
		t.Fatal(err)
	}

	out := NewOutbound(st, nil, nil, slog.New(slog.DiscardHandler), 1<<20, 0)
	mensaje := func(from string) SubmitMessage {
		return SubmitMessage{
			Subject: "sub-ana", From: "ana@acme.sytue.eu", Recipients: []string{"cliente@fuera.example"},
			Body: strings.NewReader("From: " + from + "\r\nTo: cliente@fuera.example\r\nSubject: Factura\r\n\r\nhola\r\n"),
		}
	}
	for _, from := range []string{
		"Bea <bea@acme.sytue.eu>",    // una compañera
		"Eva <eva@globex.sytue.eu>",  // otra organización
		"Soporte <soporte@sytue.eu>", // una dirección que no es de nadie
		"jefa@otra-empresa.example",  // fuera de la plataforma
	} {
		if err := out.Submit(acme, mensaje(from)); !errors.Is(err, domain.ErrFromNoAutorizado) {
			t.Errorf("Submit con From %q: err = %v, quiero ErrFromNoAutorizado", from, err)
		}
	}
	var encolados int
	if err := h.Pool.InTenantTx(acme, func(tx pgx.Tx) error {
		return tx.QueryRow(acme, `SELECT count(*) FROM outbound`).Scan(&encolados)
	}); err != nil {
		t.Fatal(err)
	}
	if encolados != 0 {
		t.Fatalf("se han encolado %d mensajes con remitente ajeno", encolados)
	}

	// Lo que SÍ es suyo pasa la verja: su dirección y su alias, resuelto contra la base.
	for _, from := range []string{"Ana <ana@acme.sytue.eu>", "ana.garcia@acme.sytue.eu"} {
		raw := []byte("From: " + from + "\r\n\r\nhola\r\n")
		if err := exigirFromDeLaCuenta(acme, st, raw, "ana@acme.sytue.eu", ana.ID); err != nil {
			t.Errorf("From %q rechazado: %v", from, err)
		}
	}
}
