// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Los secretos escritos ANTES de que Rooms cifrara en reposo siguen en claro en
// la base: la lectura los tolera a propósito para no romper integraciones vivas.
// El sellado los cierra. Se simula el estado heredado escribiendo el secreto en
// claro por SQL, que es exactamente como quedó.
func TestSealPlaintextSecrets(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	w, err := st.CreateOutgoingWebhook(ctx, &ch.ID, "alice",
		"https://example.com/hook", "vtok", "sytue", "secreto-heredado", []string{"deploy"})
	if err != nil {
		t.Fatalf("CreateOutgoingWebhook: %v", err)
	}
	// Vuelta atrás al estado heredado: secreto en claro en la columna.
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx,
			`UPDATE outgoing_webhooks SET signing_secret = 'secreto-heredado' WHERE id = $1`, w.ID)
		return e
	}); err != nil {
		t.Fatalf("simular estado heredado: %v", err)
	}

	// Antes de sellar, quien lea la base se lleva el secreto tal cual.
	if leer(t, h, ctx, w.ID) != "secreto-heredado" {
		t.Fatal("el montaje del test no dejó el secreto en claro")
	}

	n, _, err := st.SealPlaintextSecrets(t.Context())
	if err != nil {
		t.Fatalf("SealPlaintextSecrets: %v", err)
	}
	if n != 1 {
		t.Fatalf("sellados = %d, quiero 1", n)
	}

	sellado := leer(t, h, ctx, w.ID)
	if sellado == "secreto-heredado" {
		t.Fatal("el secreto sigue EN CLARO tras sellar")
	}
	if !strings.HasPrefix(sellado, "encv1:") {
		t.Fatalf("no quedó sellado con keycrypto: %q", sellado)
	}

	// Y la integración sigue firmando: el store lo devuelve descifrado.
	list, err := st.ListOutgoingWebhooks(ctx, "alice")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListOutgoingWebhooks: %v (%d)", err, len(list))
	}
	if list[0].SigningSecret != "secreto-heredado" {
		t.Fatalf("tras sellar, el secreto usable = %q", list[0].SigningSecret)
	}

	// Idempotente: una segunda pasada no toca nada (y no re-cifra lo cifrado,
	// que dejaría el secreto irrecuperable).
	n2, _, err := st.SealPlaintextSecrets(t.Context())
	if err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("segunda pasada selló %d, quiero 0", n2)
	}
	if leer(t, h, ctx, w.ID) != sellado {
		t.Fatal("la segunda pasada cambió el valor sellado")
	}
}

// leer devuelve la columna signing_secret tal y como está en la base, sin pasar
// por el store (que descifra).
func leer(t *testing.T, h *pgtest.Harness, ctx context.Context, id uuid.UUID) string {
	t.Helper()
	var v string
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT signing_secret FROM outgoing_webhooks WHERE id = $1`, id).Scan(&v)
	}); err != nil {
		t.Fatalf("leer signing_secret: %v", err)
	}
	return v
}
