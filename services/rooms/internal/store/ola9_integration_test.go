// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 9 (hilos) contra PostgreSQL real (pgtest):
// seguimiento de hilos (thread_follows, mig. 0008), auto-follow al responder, el
// contador de respuestas NO leídas por hilo, marcar visto al abrir, el filtro de
// acceso al canal en la vista global y el orden por última actividad. Detrás del
// build-tag `integration`. Sigue el patrón de actions_integration_test.go.
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

func TestThreadAutoFollowAndUnread(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	for _, sub := range []string{"bob", "carol"} {
		if err := st.AddTeamMember(ctx, tm.ID, "alice", sub, sub, domain.RoleMember); err != nil {
			t.Fatalf("AddTeamMember %s: %v", sub, err)
		}
	}

	// alice inicia el hilo. Crear la raíz NO crea seguimiento todavía.
	root, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "tema"})
	if err != nil {
		t.Fatalf("CreateMessage raíz: %v", err)
	}
	if threads, _, err := st.ListFollowedThreads(ctx, "alice", nil, 50); err != nil || len(threads) != 0 {
		t.Fatalf("crear la raíz no debe seguir: %+v err=%v", threads, err)
	}

	// bob responde → auto-follow de bob (el que responde) y de alice (autor de la raíz).
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, ParentID: &root.ID, AuthorSub: "bob", AuthorName: "Bob", Body: "respuesta 1"}); err != nil {
		t.Fatalf("CreateMessage respuesta bob: %v", err)
	}

	// alice sigue el hilo y tiene 1 respuesta no leída (la de bob).
	al := findThread(t, listThreads(t, st, ctx, "alice"), root.ID)
	if al.UnreadReplies != 1 {
		t.Fatalf("alice unread=%d, quiero 1", al.UnreadReplies)
	}
	if al.Root.ReplyCount != 1 {
		t.Fatalf("alice root.ReplyCount=%d, quiero 1", al.Root.ReplyCount)
	}
	// bob sigue el hilo pero su propia respuesta no le cuenta como no leída.
	bo := findThread(t, listThreads(t, st, ctx, "bob"), root.ID)
	if bo.UnreadReplies != 0 {
		t.Fatalf("bob unread=%d, quiero 0 (su propia respuesta no cuenta)", bo.UnreadReplies)
	}

	// carol responde → alice acumula 2 no leídas; bob 1 (la de carol).
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, ParentID: &root.ID, AuthorSub: "carol", AuthorName: "Carol", Body: "respuesta 2"}); err != nil {
		t.Fatalf("CreateMessage respuesta carol: %v", err)
	}
	if al := findThread(t, listThreads(t, st, ctx, "alice"), root.ID); al.UnreadReplies != 2 {
		t.Fatalf("alice unread=%d tras carol, quiero 2", al.UnreadReplies)
	}
	if bo := findThread(t, listThreads(t, st, ctx, "bob"), root.ID); bo.UnreadReplies != 1 {
		t.Fatalf("bob unread=%d tras carol, quiero 1", bo.UnreadReplies)
	}

	// alice ABRE el hilo (ListThread) → se marca visto y su contador cae a 0.
	if _, _, err := st.ListThread(ctx, root.ID, "alice"); err != nil {
		t.Fatalf("ListThread alice: %v", err)
	}
	if al := findThread(t, listThreads(t, st, ctx, "alice"), root.ID); al.UnreadReplies != 0 {
		t.Fatalf("alice unread=%d tras abrir, quiero 0", al.UnreadReplies)
	}
}

func TestThreadFollowedOrderingAndAccessFilter(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember bob: %v", err)
	}

	// Dos hilos; bob responde a ambos (los sigue). El segundo queda con más actividad.
	r1, _ := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: "hilo 1"})
	r2, _ := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: "hilo 2"})
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, ParentID: &r1.ID, AuthorSub: "bob", Body: "a"}); err != nil {
		t.Fatalf("reply r1: %v", err)
	}
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, ParentID: &r2.ID, AuthorSub: "bob", Body: "b"}); err != nil {
		t.Fatalf("reply r2: %v", err)
	}
	// r2 fue el último con actividad → primero en la lista de bob.
	list := listThreads(t, st, ctx, "bob")
	if len(list) != 2 || list[0].Root.ID != r2.ID || list[1].Root.ID != r1.ID {
		t.Fatalf("orden por actividad inesperado: %+v", threadIDs(list))
	}
	// Nueva respuesta a r1 lo sube al principio (bombeo de last_activity).
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, ParentID: &r1.ID, AuthorSub: "alice", Body: "c"}); err != nil {
		t.Fatalf("reply r1 de nuevo: %v", err)
	}
	if list := listThreads(t, st, ctx, "bob"); list[0].Root.ID != r1.ID {
		t.Fatalf("r1 debería subir al principio tras nueva actividad: %+v", threadIDs(list))
	}

	// Keyset: página de 1 recorre ambos sin repetir.
	page1, next, err := st.ListFollowedThreads(ctx, "bob", nil, 1)
	if err != nil || len(page1) != 1 || next == nil {
		t.Fatalf("page1 = %+v next=%v err=%v", threadIDs(page1), next, err)
	}
	page2, _, err := st.ListFollowedThreads(ctx, "bob", next, 1)
	if err != nil || len(page2) != 1 || page2[0].Root.ID == page1[0].Root.ID {
		t.Fatalf("page2 = %+v err=%v (no debe repetir)", threadIDs(page2), err)
	}

	// Filtro de acceso: bob sigue un hilo de un canal privado y luego lo abandona →
	// ya no aparece en su vista global (no revela contenido perdido).
	priv, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "priv", Name: "Privado", Kind: domain.ChannelPrivate,
		CreatedBy: "alice", Members: []string{"bob"},
	})
	if err != nil {
		t.Fatalf("CreateChannel priv: %v", err)
	}
	pr, _ := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: priv.ID, AuthorSub: "alice", Body: "secreto"})
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: priv.ID, ParentID: &pr.ID, AuthorSub: "bob", Body: "sigo"}); err != nil {
		t.Fatalf("reply priv bob: %v", err)
	}
	if _, ok := tryFindThread(listThreads(t, st, ctx, "bob"), pr.ID); !ok {
		t.Fatal("bob debe ver su hilo privado siendo miembro")
	}
	if err := st.LeaveChannel(ctx, priv.ID, "bob"); err != nil {
		t.Fatalf("LeaveChannel: %v", err)
	}
	if _, ok := tryFindThread(listThreads(t, st, ctx, "bob"), pr.ID); ok {
		t.Fatal("tras abandonar el canal, el hilo debe filtrarse de la vista global")
	}
}

func TestFollowUnfollowAndValidation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	root, _ := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: "raíz"})
	reply, _ := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, ParentID: &root.ID, AuthorSub: "alice", Body: "resp"})

	// bob sigue explícitamente (aún no había respondido) — idempotente.
	if err := st.FollowThread(ctx, root.ID, "bob"); err != nil {
		t.Fatalf("FollowThread bob: %v", err)
	}
	if err := st.FollowThread(ctx, root.ID, "bob"); err != nil {
		t.Fatalf("FollowThread idempotente: %v", err)
	}
	if ok, err := st.IsFollowingThread(ctx, root.ID, "bob"); err != nil || !ok {
		t.Fatalf("IsFollowingThread bob = %v err=%v, quiero true", ok, err)
	}

	// Seguir una RESPUESTA (no raíz) → ErrInvalidInput.
	if err := st.FollowThread(ctx, reply.ID, "bob"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("seguir una respuesta debe ser ErrInvalidInput, got %v", err)
	}
	// Seguir un mensaje inexistente → ErrNotFound.
	if err := st.FollowThread(ctx, uuid.New(), "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("seguir inexistente debe ser ErrNotFound, got %v", err)
	}

	// No-miembro de un canal privado no puede seguir su hilo → ErrForbidden.
	_, priv := seedPrivateChannel(t, ctx, st, "owner")
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "owner", "Owner", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember owner: %v", err)
	}
	pr, _ := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: priv.ID, AuthorSub: "owner", Body: "priv"})
	if err := st.FollowThread(ctx, pr.ID, "bob"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("no-miembro siguiendo hilo privado debe ser ErrForbidden, got %v", err)
	}

	// Unfollow y volver a dejar de seguir → ErrNotFound.
	if err := st.UnfollowThread(ctx, root.ID, "bob"); err != nil {
		t.Fatalf("UnfollowThread: %v", err)
	}
	if ok, _ := st.IsFollowingThread(ctx, root.ID, "bob"); ok {
		t.Fatal("bob no debería seguir tras UnfollowThread")
	}
	if err := st.UnfollowThread(ctx, root.ID, "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unfollow de no-seguido debe ser ErrNotFound, got %v", err)
	}
}

// listThreads lista los hilos seguidos del usuario (primera página) o falla el test.
func listThreads(t *testing.T, st *store.Store, ctx context.Context, subject string) []domain.ThreadSummary {
	t.Helper()
	list, _, err := st.ListFollowedThreads(ctx, subject, nil, 50)
	if err != nil {
		t.Fatalf("ListFollowedThreads %s: %v", subject, err)
	}
	return list
}

func findThread(t *testing.T, list []domain.ThreadSummary, rootID uuid.UUID) domain.ThreadSummary {
	t.Helper()
	if ts, ok := tryFindThread(list, rootID); ok {
		return ts
	}
	t.Fatalf("el hilo %s no está en la lista %+v", rootID, threadIDs(list))
	return domain.ThreadSummary{}
}

func tryFindThread(list []domain.ThreadSummary, rootID uuid.UUID) (domain.ThreadSummary, bool) {
	for _, ts := range list {
		if ts.Root.ID == rootID {
			return ts, true
		}
	}
	return domain.ThreadSummary{}, false
}

func threadIDs(list []domain.ThreadSummary) []uuid.UUID {
	out := make([]uuid.UUID, len(list))
	for i, ts := range list {
		out[i] = ts.Root.ID
	}
	return out
}
