// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Un puente guarda su URL cifrada, solo lo gestiona el admin del canal, no pasa
// del tope por canal, anota el error solo cuando cambia y no se ve desde otra
// organización.
func TestBridgeLifecycle(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}

	const url = "https://hooks.slack.com/services/T000/B000/secretoXYZ9"
	b, err := st.CreateBridge(ctx, ch.ID, "alice", domain.BridgeSlack, url)
	if err != nil {
		t.Fatalf("CreateBridge: %v", err)
	}
	if b.WebhookURL != url || b.Target != "hooks.slack.com/…XYZ9" || b.Platform != domain.BridgeSlack {
		t.Fatalf("puente = %+v", b)
	}
	var enBase string
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT webhook_url FROM channel_bridges WHERE id=$1`, b.ID).Scan(&enBase)
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enBase, "encv1:") || strings.Contains(enBase, "secreto") {
		t.Fatalf("la URL del puente no está cifrada en la base: %q", enBase)
	}

	if _, err := st.CreateBridge(ctx, ch.ID, "bob", domain.BridgeSlack, url); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("un miembro normal no conecta puentes: %v", err)
	}
	if _, err := st.ListBridges(ctx, ch.ID, "bob"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("un miembro normal no ve los puentes: %v", err)
	}
	for i := 1; i < domain.MaxBridgesPerChannel; i++ {
		if _, err := st.CreateBridge(ctx, ch.ID, "alice", domain.BridgeTeams, "https://acme.webhook.office.com/webhookb2/x"); err != nil {
			t.Fatalf("puente %d: %v", i+1, err)
		}
	}
	if _, err := st.CreateBridge(ctx, ch.ID, "alice", domain.BridgeTeams, "https://acme.webhook.office.com/webhookb2/x"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("el puente %d supera el tope: %v", domain.MaxBridgesPerChannel+1, err)
	}

	if err := st.SetBridgeError(ctx, b.ID, "HTTP 404: no_service"); err != nil {
		t.Fatal(err)
	}
	lista, err := st.BridgesForChannel(ctx, ch.ID)
	if err != nil || len(lista) != domain.MaxBridgesPerChannel {
		t.Fatalf("BridgesForChannel = %d, %v", len(lista), err)
	}
	if lista[0].LastError != "HTTP 404: no_service" || lista[0].LastErrorAt == nil {
		t.Fatalf("el error no quedó anotado: %+v", lista[0])
	}
	if err := st.SetBridgeError(ctx, b.ID, ""); err != nil {
		t.Fatal(err)
	}
	if lista, _ = st.ListBridges(ctx, ch.ID, "alice"); lista[0].LastError != "" {
		t.Fatalf("una entrega buena limpia el error: %+v", lista[0])
	}

	if got, _ := st.BridgesForChannel(tenantCtx("otra"), ch.ID); len(got) != 0 {
		t.Fatalf("otra organización ve los puentes de acme: %d", len(got))
	}
	if err := st.DeleteBridge(ctx, b.ID, "bob"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("un miembro normal no desconecta: %v", err)
	}
	if err := st.DeleteBridge(ctx, b.ID, "alice"); err != nil {
		t.Fatalf("DeleteBridge: %v", err)
	}
}
