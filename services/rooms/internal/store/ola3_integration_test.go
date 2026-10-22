// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 3 (sidebar y canales) contra PostgreSQL real
// (pgtest): preferencias per-usuario favorito/silenciado (mig. 0006), explorar
// canales públicos no-miembro y abrir DM 1:1 idempotente. Detrás del build-tag
// `integration`. Sigue el patrón de actions_integration_test.go.
package store_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// findChannel localiza un canal por ID en una lista o falla el test.
func findChannel(t *testing.T, list []domain.Channel, id uuid.UUID) domain.Channel {
	t.Helper()
	for _, c := range list {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("canal %s no está en la lista", id)
	return domain.Channel{}
}

// TestChannelPrefsPersistAndList verifica que SetChannelPref persiste favorito y
// silenciado (punteros nil = sin cambio) y que ListChannels los refleja vía el
// LEFT JOIN a la propia membresía.
func TestChannelPrefsPersistAndList(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	yes := true
	got, err := st.SetChannelPref(ctx, ch.ID, "alice", "Alice", &yes, nil, nil)
	if err != nil {
		t.Fatalf("SetChannelPref favorito: %v", err)
	}
	if !got.Favorite || got.Muted {
		t.Fatalf("tras favorito: favorite=%v muted=%v, quiero true/false", got.Favorite, got.Muted)
	}
	// Silenciar sin tocar favorito (nil no cambia el otro campo).
	got, err = st.SetChannelPref(ctx, ch.ID, "alice", "Alice", nil, &yes, nil)
	if err != nil {
		t.Fatalf("SetChannelPref silenciar: %v", err)
	}
	if !got.Favorite || !got.Muted {
		t.Fatalf("tras silenciar: favorite=%v muted=%v, quiero true/true", got.Favorite, got.Muted)
	}

	// ListChannels refleja ambas preferencias.
	list, err := st.ListChannels(ctx, tm.ID, "alice", false)
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if c := findChannel(t, list, ch.ID); !c.Favorite || !c.Muted {
		t.Fatalf("ListChannels: favorite=%v muted=%v, quiero true/true", c.Favorite, c.Muted)
	}

	// Desmarcar favorito (false, no nil).
	no := false
	if _, err := st.SetChannelPref(ctx, ch.ID, "alice", "Alice", &no, nil, nil); err != nil {
		t.Fatalf("SetChannelPref desmarcar: %v", err)
	}
	list, _ = st.ListChannels(ctx, tm.ID, "alice", false)
	if c := findChannel(t, list, ch.ID); c.Favorite || !c.Muted {
		t.Fatalf("tras desmarcar: favorite=%v muted=%v, quiero false/true", c.Favorite, c.Muted)
	}

	// Un no-miembro del team no puede fijar preferencias sobre el canal público.
	if _, err := st.SetChannelPref(ctx, ch.ID, "intruso", "X", &yes, nil, nil); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("no-miembro fijando prefs debe ser ErrForbidden, got %v", err)
	}
}

// TestListJoinableChannels verifica que explorar excluye los canales de los que ya
// eres miembro, omite privados y filtra por nombre (ILIKE).
func TestListJoinableChannels(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, canal := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	random, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "random", Name: "Random", Kind: domain.ChannelPublic, CreatedBy: "alice",
	})
	if err != nil {
		t.Fatalf("CreateChannel random: %v", err)
	}
	if _, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "priv", Name: "Privado", Kind: domain.ChannelPrivate, CreatedBy: "alice",
	}); err != nil {
		t.Fatalf("CreateChannel priv: %v", err)
	}

	// bob (miembro del team; solo de los canales por defecto, a los que se auto-unió
	// al entrar) ve los dos públicos restantes, no el privado ni los por defecto.
	joinable, err := st.ListJoinableChannels(ctx, tm.ID, "bob", "", 50)
	if err != nil {
		t.Fatalf("ListJoinableChannels bob: %v", err)
	}
	slugs := map[string]bool{}
	for _, c := range joinable {
		slugs[c.Slug] = true
	}
	if !slugs["canal"] || !slugs["random"] || slugs["priv"] || slugs["general"] || slugs["aleatorio"] {
		t.Fatalf("unibles inesperados: %+v", slugs)
	}

	// Filtro por nombre.
	filtered, err := st.ListJoinableChannels(ctx, tm.ID, "bob", "rand", 50)
	if err != nil || len(filtered) != 1 || filtered[0].Slug != "random" {
		t.Fatalf("filtro 'rand' = %+v err=%v", filtered, err)
	}

	// alice ES miembro de canal y random → no le aparecen.
	aliceJoinable, err := st.ListJoinableChannels(ctx, tm.ID, "alice", "", 50)
	if err != nil {
		t.Fatalf("ListJoinableChannels alice: %v", err)
	}
	for _, c := range aliceJoinable {
		if c.ID == canal.ID || c.ID == random.ID {
			t.Fatalf("alice no debería ver canales de los que es miembro: %s", c.Slug)
		}
	}
}

// TestOpenDirectIdempotent verifica que abrir un DM 1:1 dos veces (en cualquier
// orden de los subs) devuelve el MISMO canal kind='direct', y que el nombre a
// mostrar en ListChannels se resuelve al del OTRO participante (per-visor).
func TestOpenDirectIdempotent(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, _ := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember bob: %v", err)
	}

	dm1, err := st.OpenDirect(ctx, tm.ID, "alice", "bob")
	if err != nil {
		t.Fatalf("OpenDirect alice→bob: %v", err)
	}
	if dm1.Kind != domain.ChannelDirect {
		t.Fatalf("kind = %q, quiero direct", dm1.Kind)
	}
	// Idempotente: mismo par en orden inverso → mismo canal.
	dm2, err := st.OpenDirect(ctx, tm.ID, "bob", "alice")
	if err != nil {
		t.Fatalf("OpenDirect bob→alice: %v", err)
	}
	if dm2.ID != dm1.ID {
		t.Fatalf("DM no idempotente por orden: %s != %s", dm2.ID, dm1.ID)
	}
	// Y de nuevo en el orden original.
	if dm3, err := st.OpenDirect(ctx, tm.ID, "alice", "bob"); err != nil || dm3.ID != dm1.ID {
		t.Fatalf("tercera apertura no idempotente: %s err=%v", dm3.ID, err)
	}

	// Display per-visor: alice ve "Bob"; bob ve "Creador" (nombre de alice en el team).
	aliceList, err := st.ListChannels(ctx, tm.ID, "alice", false)
	if err != nil {
		t.Fatalf("ListChannels alice: %v", err)
	}
	if c := findChannel(t, aliceList, dm1.ID); c.Name != "Bob" {
		t.Fatalf("alice ve el DM como %q, quiero Bob", c.Name)
	}
	bobList, err := st.ListChannels(ctx, tm.ID, "bob", false)
	if err != nil {
		t.Fatalf("ListChannels bob: %v", err)
	}
	if c := findChannel(t, bobList, dm1.ID); c.Name != "Creador" {
		t.Fatalf("bob ve el DM como %q, quiero Creador (nombre de alice)", c.Name)
	}

	// No-miembro del team no puede abrir un DM (ni como destinatario).
	if _, err := st.OpenDirect(ctx, tm.ID, "alice", "carol"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("DM con no-miembro debe ser ErrForbidden, got %v", err)
	}
	// DM consigo mismo → entrada inválida.
	if _, err := st.OpenDirect(ctx, tm.ID, "alice", "alice"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("DM consigo mismo debe ser ErrInvalidInput, got %v", err)
	}
}
