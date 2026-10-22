// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test de integración del CONTADOR de uso de almacenamiento (accounts.used_bytes)
// que respalda el endpoint /api/v1/quota: verifica que la suma es correcta
// (agregada al insertar, decrementada al borrar) y OWNER-SCOPED (el uso de una
// cuenta no incluye el de otra, ni siquiera en el mismo tenant).
package store_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func TestStorageUsageSumAndOwnerScope(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	// Dos cuentas del MISMO tenant: aísla el alcance por CUENTA (no solo por tenant).
	accA := seedAccount(t, ctx, st, "ada@sytue.eu")
	accB := seedAccount(t, ctx, st, "bob@sytue.eu")
	inboxA, err := st.MailboxByRole(ctx, accA.ID, domain.RoleInbox)
	if err != nil {
		t.Fatalf("MailboxByRole A: %v", err)
	}
	inboxB, err := st.MailboxByRole(ctx, accB.ID, domain.RoleInbox)
	if err != nil {
		t.Fatalf("MailboxByRole B: %v", err)
	}

	// Recién creadas: uso cero.
	if u := usage(t, ctx, st, "sub-ada@sytue.eu"); u != 0 {
		t.Fatalf("uso inicial A = %d, quiero 0", u)
	}

	// seedMessage inserta con SizeBytes=100. Tres en A, dos en B.
	m1 := seedMessage(t, ctx, st, accA, inboxA.ID, "Uno", "a@x.eu", nil)
	seedMessage(t, ctx, st, accA, inboxA.ID, "Dos", "b@x.eu", nil)
	seedMessage(t, ctx, st, accA, inboxA.ID, "Tres", "c@x.eu", nil)
	seedMessage(t, ctx, st, accB, inboxB.ID, "B-uno", "d@x.eu", nil)
	seedMessage(t, ctx, st, accB, inboxB.ID, "B-dos", "e@x.eu", nil)

	// Suma correcta y OWNER-SCOPED: A cuenta solo lo suyo (3×100), no lo de B.
	if u := usage(t, ctx, st, "sub-ada@sytue.eu"); u != 300 {
		t.Fatalf("uso A = %d, quiero 300 (suma de A, sin contar B)", u)
	}
	if u := usage(t, ctx, st, "sub-bob@sytue.eu"); u != 200 {
		t.Fatalf("uso B = %d, quiero 200", u)
	}

	// Borrar (soft-delete) descuenta el tamaño del mensaje de SU cuenta y nada más.
	if err := st.DeleteMessage(ctx, accA.ID, m1.ID); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if u := usage(t, ctx, st, "sub-ada@sytue.eu"); u != 200 {
		t.Fatalf("uso A tras borrar = %d, quiero 200", u)
	}
	if u := usage(t, ctx, st, "sub-bob@sytue.eu"); u != 200 {
		t.Fatalf("borrar en A alteró el uso de B: %d", u)
	}
}

// usage lee accounts.used_bytes de una cuenta por su subject (lo que expone el
// endpoint de cuota). quota_bytes es el sembrado por seedAccount (1 GiB).
func usage(t *testing.T, ctx context.Context, st *store.Store, subject string) int64 {
	t.Helper()
	acc, err := st.AccountBySubject(ctx, subject)
	if err != nil {
		t.Fatalf("AccountBySubject(%q): %v", subject, err)
	}
	if acc.QuotaBytes != 1<<30 {
		t.Fatalf("quota_bytes = %d, quiero 1 GiB (sembrado)", acc.QuotaBytes)
	}
	return acc.UsedBytes
}
