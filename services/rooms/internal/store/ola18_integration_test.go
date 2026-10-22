// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 18 (Web Push) contra PostgreSQL real (pgtest): alta/baja de
// suscripciones (mig. 0016 + RLS) y selección de destinatarios del enviador —@menciones y
// peer de DM, con el filtrado por notify_level (Ola 13), autor excluido y limpieza—. Detrás
// del build-tag `integration`. Sigue el patrón de ola13/ola16_integration_test.go.
package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func subFor(sub string) domain.PushSubscription {
	return domain.PushSubscription{
		Endpoint: "https://push.example.com/ep/" + sub,
		P256dh:   "p256dh-" + sub,
		Auth:     "auth-" + sub,
	}
}

// TestPushSubscribeAndMentionTargets: un @mencionado miembro del canal recibe target; el
// autor NO; un canal silenciado ('none') suprime la mención; y desuscribir limpia.
func TestPushSubscribeAndMentionTargets(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	// bob entra al team y se une al canal; alice (autora) también suscribe.
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember bob: %v", err)
	}
	if _, err := st.JoinChannel(ctx, ch.ID, "bob", "Bob"); err != nil {
		t.Fatalf("JoinChannel bob: %v", err)
	}
	if err := st.SavePushSubscription(ctx, "bob", subFor("bob")); err != nil {
		t.Fatalf("SavePushSubscription bob: %v", err)
	}
	if err := st.SavePushSubscription(ctx, "alice", subFor("alice")); err != nil {
		t.Fatalf("SavePushSubscription alice: %v", err)
	}

	// alice menciona a bob: solo bob es target (el autor se excluye).
	targets, err := st.PushTargetsForMessage(ctx, ch.ID, "alice", []string{"bob"})
	if err != nil {
		t.Fatalf("PushTargetsForMessage: %v", err)
	}
	if len(targets) != 1 || targets[0].Subject != "bob" || targets[0].Endpoint != subFor("bob").Endpoint {
		t.Fatalf("mención: targets=%+v, quiero solo bob", targets)
	}

	// bob silencia el canal ('none'): la mención deja de generar push.
	none := domain.NotifyLevelNone
	if _, err := st.SetChannelPref(ctx, ch.ID, "bob", "Bob", nil, nil, &none); err != nil {
		t.Fatalf("SetChannelPref none: %v", err)
	}
	if targets, _ := st.PushTargetsForMessage(ctx, ch.ID, "alice", []string{"bob"}); len(targets) != 0 {
		t.Fatalf("canal silenciado: quiero 0 targets, got %+v", targets)
	}

	// 'mentions' vuelve a permitir la mención (no es 'none').
	mentions := domain.NotifyLevelMentions
	if _, err := st.SetChannelPref(ctx, ch.ID, "bob", "Bob", nil, nil, &mentions); err != nil {
		t.Fatalf("SetChannelPref mentions: %v", err)
	}
	if targets, _ := st.PushTargetsForMessage(ctx, ch.ID, "alice", []string{"bob"}); len(targets) != 1 {
		t.Fatalf("nivel 'mentions': quiero 1 target, got %+v", targets)
	}

	// Desuscribir (limpieza) elimina el target.
	if err := st.DeletePushSubscription(ctx, "bob", subFor("bob").Endpoint); err != nil {
		t.Fatalf("DeletePushSubscription: %v", err)
	}
	if targets, _ := st.PushTargetsForMessage(ctx, ch.ID, "alice", []string{"bob"}); len(targets) != 0 {
		t.Fatalf("tras desuscribir: quiero 0 targets, got %+v", targets)
	}
}

// TestPushDMTargets: en un DM el otro miembro recibe push solo con notify_level='all'; a
// 'mentions' un DM plano (sin @) no notifica —igual que la política del cliente—.
func TestPushDMTargets(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	tm, _ := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	// bob debe pertenecer al team para abrir un DM con él.
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember bob: %v", err)
	}
	dm, err := st.OpenDirect(ctx, tm.ID, "alice", "bob")
	if err != nil {
		t.Fatalf("OpenDirect: %v", err)
	}
	if err := st.SavePushSubscription(ctx, "bob", subFor("bob")); err != nil {
		t.Fatalf("SavePushSubscription bob: %v", err)
	}

	// DM de alice→bob sin menciones: bob (peer, 'all' por defecto) es target.
	if targets, _ := st.PushTargetsForMessage(ctx, dm.ID, "alice", nil); len(targets) != 1 || targets[0].Subject != "bob" {
		t.Fatalf("DM 'all': quiero 1 target bob, got %+v", targets)
	}

	// bob pone el DM en 'mentions': un mensaje sin @ ya no le notifica.
	mentions := domain.NotifyLevelMentions
	if _, err := st.SetChannelPref(ctx, dm.ID, "bob", "Bob", nil, nil, &mentions); err != nil {
		t.Fatalf("SetChannelPref DM mentions: %v", err)
	}
	if targets, _ := st.PushTargetsForMessage(ctx, dm.ID, "alice", nil); len(targets) != 0 {
		t.Fatalf("DM 'mentions' sin @: quiero 0 targets, got %+v", targets)
	}
	// Pero si alice MENCIONA a bob, sí notifica ('mentions' != 'none').
	if targets, _ := st.PushTargetsForMessage(ctx, dm.ID, "alice", []string{"bob"}); len(targets) != 1 {
		t.Fatalf("DM 'mentions' con @bob: quiero 1 target, got %+v", targets)
	}
}
