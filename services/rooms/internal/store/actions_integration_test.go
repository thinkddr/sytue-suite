// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 2 (acciones sobre mensajes) contra PostgreSQL
// real (pgtest): fijados (pinned), guardados (saved) y marcar como no leído.
// Verifican el SQL de las migraciones 0004/0005, sus índices, la autorización por
// acceso al canal y el recálculo de no-leídos. Detrás del build-tag `integration`.
package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// seedPrivateChannel crea un team aparte (slug "eq2") con un canal privado del
// que `creator` es admin. Evita el choque de slug con seedTeamChannel ("eq").
func seedPrivateChannel(t *testing.T, ctx context.Context, st *store.Store, creator string) (domain.Team, domain.Channel) {
	t.Helper()
	tm, err := st.CreateTeam(ctx, store.NewTeamRecord{Slug: "eq2", Name: "Equipo 2", CreatedBy: creator, CreatorName: "Creador"})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	ch, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "priv", Name: "Privado", Kind: domain.ChannelPrivate, CreatedBy: creator,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	return tm, ch
}

// unreadOf devuelve el contador de no leídos de un canal concreto para el usuario.
func unreadOf(t *testing.T, st *store.Store, ctx context.Context, teamID, channelID uuid.UUID, subject string) int {
	t.Helper()
	states, err := st.UnreadByChannel(ctx, teamID, subject)
	if err != nil {
		t.Fatalf("UnreadByChannel: %v", err)
	}
	for _, s := range states {
		if s.ChannelID == channelID {
			return s.Unread
		}
	}
	t.Fatalf("canal %s no está en los estados de lectura de %s", channelID, subject)
	return 0
}

func TestPinsFlowAndAuthorization(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	m, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: "fíjame"})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	pinned, err := st.PinMessage(ctx, m.ID, "alice")
	if err != nil {
		t.Fatalf("PinMessage: %v", err)
	}
	if pinned.ID != m.ID || pinned.ChannelID != ch.ID {
		t.Fatalf("PinMessage devolvió el mensaje/canal equivocado: %+v", pinned)
	}
	// Idempotente.
	if _, err := st.PinMessage(ctx, m.ID, "alice"); err != nil {
		t.Fatalf("PinMessage idempotente: %v", err)
	}
	pins, err := st.ListPins(ctx, ch.ID, "alice")
	if err != nil {
		t.Fatalf("ListPins: %v", err)
	}
	if len(pins) != 1 || pins[0].ID != m.ID {
		t.Fatalf("ListPins = %+v, quiero 1 con id %s", pins, m.ID)
	}

	// Desfijar y volver a listar (vacío).
	if _, err := st.UnpinMessage(ctx, m.ID, "alice"); err != nil {
		t.Fatalf("UnpinMessage: %v", err)
	}
	if pins, err = st.ListPins(ctx, ch.ID, "alice"); err != nil || len(pins) != 0 {
		t.Fatalf("ListPins tras desfijar = %+v, err=%v", pins, err)
	}
	// Desfijar lo ya no fijado → ErrNotFound.
	if _, err := st.UnpinMessage(ctx, m.ID, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UnpinMessage no-fijado debe ser ErrNotFound, got %v", err)
	}

	// Autorización: no-miembro de canal privado no puede fijar ni listar.
	tm2, priv := seedPrivateChannel(t, ctx, st, "owner")
	if err := st.AddTeamMember(ctx, tm2.ID, "owner", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	pm, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: priv.ID, AuthorSub: "owner", Body: "secreto"})
	if err != nil {
		t.Fatalf("CreateMessage priv: %v", err)
	}
	if _, err := st.PinMessage(ctx, pm.ID, "bob"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("bob no-miembro no debe fijar, got %v", err)
	}
	if _, err := st.ListPins(ctx, priv.ID, "bob"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("bob no-miembro no debe listar fijados, got %v", err)
	}
}

func TestSavesFlowAndAccessFilter(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	m1, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: "uno"})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	m2, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: "dos"})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	if err := st.SaveMessage(ctx, m1.ID, "alice"); err != nil {
		t.Fatalf("SaveMessage m1: %v", err)
	}
	if err := st.SaveMessage(ctx, m2.ID, "alice"); err != nil {
		t.Fatalf("SaveMessage m2: %v", err)
	}
	// Idempotente.
	if err := st.SaveMessage(ctx, m2.ID, "alice"); err != nil {
		t.Fatalf("SaveMessage idempotente: %v", err)
	}
	saved, next, err := st.ListSaved(ctx, "alice", nil, 50)
	if err != nil {
		t.Fatalf("ListSaved: %v", err)
	}
	// Orden por fecha de guardado descendente: m2 se guardó después de m1.
	if len(saved) != 2 || saved[0].ID != m2.ID || saved[1].ID != m1.ID {
		t.Fatalf("ListSaved orden inesperado: %+v", saved)
	}
	if next != nil {
		t.Fatalf("página no completa no debe exponer cursor, got %v", next)
	}

	// Keyset: limit 1 → primera página m2 + cursor; segunda página m1.
	page1, cur, err := st.ListSaved(ctx, "alice", nil, 1)
	if err != nil || len(page1) != 1 || page1[0].ID != m2.ID || cur == nil {
		t.Fatalf("ListSaved page1 = %+v cur=%v err=%v", page1, cur, err)
	}
	page2, _, err := st.ListSaved(ctx, "alice", cur, 1)
	if err != nil || len(page2) != 1 || page2[0].ID != m1.ID {
		t.Fatalf("ListSaved page2 = %+v err=%v", page2, err)
	}

	// Unsave.
	if err := st.UnsaveMessage(ctx, m1.ID, "alice"); err != nil {
		t.Fatalf("UnsaveMessage: %v", err)
	}
	if err := st.UnsaveMessage(ctx, m1.ID, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UnsaveMessage no-guardado debe ser ErrNotFound, got %v", err)
	}

	// Filtro de acceso al listar: bob guarda un mensaje de un canal privado y luego
	// lo abandona → ya no aparece en sus guardados (no revela contenido perdido).
	priv, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "priv", Name: "Privado", Kind: domain.ChannelPrivate,
		CreatedBy: "alice", Members: []string{"bob"},
	})
	if err != nil {
		t.Fatalf("CreateChannel priv: %v", err)
	}
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	pmsg, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: priv.ID, AuthorSub: "alice", Body: "privado"})
	if err != nil {
		t.Fatalf("CreateMessage priv: %v", err)
	}
	if err := st.SaveMessage(ctx, pmsg.ID, "bob"); err != nil {
		t.Fatalf("SaveMessage bob priv: %v", err)
	}
	if saved, _, err = st.ListSaved(ctx, "bob", nil, 50); err != nil || len(saved) != 1 {
		t.Fatalf("bob debe ver su guardado siendo miembro: %+v err=%v", saved, err)
	}
	if err := st.LeaveChannel(ctx, priv.ID, "bob"); err != nil {
		t.Fatalf("LeaveChannel: %v", err)
	}
	if saved, _, err = st.ListSaved(ctx, "bob", nil, 50); err != nil || len(saved) != 0 {
		t.Fatalf("tras abandonar el canal, el guardado debe filtrarse: %+v err=%v", saved, err)
	}
}

func TestMarkUnreadRecomputesBadge(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	var msgs []domain.Message
	for _, b := range []string{"uno", "dos", "tres"} {
		m, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: b})
		if err != nil {
			t.Fatalf("CreateMessage %s: %v", b, err)
		}
		msgs = append(msgs, m)
	}

	// bob lee todo el canal → 0 no leídos.
	if err := st.MarkRead(ctx, ch.ID, "bob", nil); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if u := unreadOf(t, st, ctx, tm.ID, ch.ID, "bob"); u != 0 {
		t.Fatalf("tras leer, unread=%d, quiero 0", u)
	}

	// Marca como no leído desde el 2º mensaje → 2 no leídos (m2, m3).
	if err := st.MarkUnread(ctx, msgs[1].ID, "bob"); err != nil {
		t.Fatalf("MarkUnread m2: %v", err)
	}
	if u := unreadOf(t, st, ctx, tm.ID, ch.ID, "bob"); u != 2 {
		t.Fatalf("tras marcar no leído desde m2, unread=%d, quiero 2", u)
	}

	// Marca desde el 1º (predecesor NULL) → 3 no leídos.
	if err := st.MarkUnread(ctx, msgs[0].ID, "bob"); err != nil {
		t.Fatalf("MarkUnread m1: %v", err)
	}
	if u := unreadOf(t, st, ctx, tm.ID, ch.ID, "bob"); u != 3 {
		t.Fatalf("tras marcar no leído desde m1, unread=%d, quiero 3", u)
	}

	// Mensaje inexistente → ErrNotFound.
	if err := st.MarkUnread(ctx, uuid.New(), "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("MarkUnread de inexistente debe ser ErrNotFound, got %v", err)
	}
}
