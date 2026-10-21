// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del ciclo de vida de canales contra PostgreSQL real
// (pgtest): salir de un canal público y que no vuelva a la barra, la verja del
// último admin de un privado, borrar un canal (DeleteChannel) y «borrar el chat»
// de un directo solo para uno (HideDirect, mig 0058).
package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// enLista dice si un canal sale en la barra lateral de subject.
func enLista(t *testing.T, list []domain.Channel, id string) bool {
	t.Helper()
	for _, c := range list {
		if c.ID.String() == id {
			return true
		}
	}
	return false
}

// TestSalirDeUnPublicoNoVuelve: un canal público del que alguien se va deja de
// salir en su barra (antes reaparecía al recargar) pero sigue en «explorar».
func TestSalirDeUnPublicoNoVuelve(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	// bob no es miembro de «canal»: no sale en su barra, sí en explorar.
	if enLista(t, mustList(t, st, ctx, tm.ID, "bob"), ch.ID.String()) {
		t.Fatal("un público del que no es miembro no debe salir en la barra")
	}
	if _, err := st.JoinChannel(ctx, ch.ID, "bob", "Bob"); err != nil {
		t.Fatalf("JoinChannel: %v", err)
	}
	if !enLista(t, mustList(t, st, ctx, tm.ID, "bob"), ch.ID.String()) {
		t.Fatal("tras unirse, el canal debe salir en la barra")
	}
	if err := st.LeaveChannel(ctx, ch.ID, "bob"); err != nil {
		t.Fatalf("LeaveChannel: %v", err)
	}
	if enLista(t, mustList(t, st, ctx, tm.ID, "bob"), ch.ID.String()) {
		t.Fatal("tras salir, el canal público no debe volver a la barra")
	}
	joinable, err := st.ListJoinableChannels(ctx, tm.ID, "bob", "", 50)
	if err != nil {
		t.Fatalf("ListJoinableChannels: %v", err)
	}
	if !enLista(t, joinable, ch.ID.String()) {
		t.Fatal("el canal del que salió debe seguir en explorar")
	}
}

// TestUltimoAdminDeUnPrivado: el único admin de un privado con más miembros no
// puede irse (ErrLastAdmin, un ErrConflict); con otro admin, sí.
func TestUltimoAdminDeUnPrivado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, err := st.CreateTeam(ctx, store.NewTeamRecord{Slug: "eq", Name: "Equipo", CreatedBy: "alice", CreatorName: "Alice"})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	priv, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "priv", Name: "Priv", Kind: domain.ChannelPrivate, CreatedBy: "alice", Members: []string{"bob"},
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if err := st.LeaveChannel(ctx, priv.ID, "alice"); !errors.Is(err, domain.ErrLastAdmin) || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("último admin saliendo: err=%v, quiero ErrLastAdmin", err)
	}
	if err := st.RemoveChannelMember(ctx, priv.ID, "alice", "alice"); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("último admin por autoservicio: err=%v, quiero ErrLastAdmin", err)
	}
	if _, err := st.SetChannelMemberRole(ctx, priv.ID, "alice", "bob", domain.RoleAdmin); err != nil {
		t.Fatalf("SetChannelMemberRole: %v", err)
	}
	if err := st.LeaveChannel(ctx, priv.ID, "alice"); err != nil {
		t.Fatalf("con otro admin debe poder salir: %v", err)
	}
}

// TestDeleteChannel: un admin borra el canal y desaparece de la barra y de
// GetChannel; un miembro raso no puede; un directo no se borra así.
func TestDeleteChannel(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	if _, err := st.JoinChannel(ctx, ch.ID, "bob", "Bob"); err != nil {
		t.Fatalf("JoinChannel: %v", err)
	}
	if err := st.DeleteChannel(ctx, ch.ID, "bob"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("miembro raso borrando: err=%v, quiero ErrForbidden", err)
	}
	dm, err := st.OpenDirect(ctx, tm.ID, "alice", "bob")
	if err != nil {
		t.Fatalf("OpenDirect: %v", err)
	}
	if err := st.DeleteChannel(ctx, dm.ID, "alice"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("borrar un directo: err=%v, quiero ErrForbidden", err)
	}

	if err := st.DeleteChannel(ctx, ch.ID, "alice"); err != nil {
		t.Fatalf("DeleteChannel admin: %v", err)
	}
	for _, sub := range []string{"alice", "bob"} {
		if enLista(t, mustList(t, st, ctx, tm.ID, sub), ch.ID.String()) {
			t.Fatalf("el canal borrado sigue en la barra de %s", sub)
		}
	}
	if _, err := st.GetChannel(ctx, ch.ID, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetChannel de un borrado: err=%v, quiero ErrNotFound", err)
	}
	if err := st.DeleteChannel(ctx, ch.ID, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("borrar dos veces: err=%v, quiero ErrNotFound", err)
	}
}

// TestHideDirect: borrar un directo para uno lo saca de SU barra y le esconde el
// historial; el otro lo conserva. Un mensaje nuevo lo devuelve con solo lo nuevo,
// y reabrirlo también lo devuelve.
func TestHideDirect(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	if err := st.HideDirect(ctx, ch.ID, "alice"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ocultar un canal que no es directo: err=%v, quiero ErrForbidden", err)
	}
	dm, err := st.OpenDirect(ctx, tm.ID, "alice", "bob")
	if err != nil {
		t.Fatalf("OpenDirect: %v", err)
	}
	if err := st.HideDirect(ctx, dm.ID, "carol"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ocultar sin ser miembro: err=%v, quiero ErrForbidden", err)
	}
	post := func(author, body string) {
		t.Helper()
		if _, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: dm.ID, AuthorSub: author, AuthorName: author, Body: body}); err != nil {
			t.Fatalf("CreateMessage %q: %v", body, err)
		}
	}
	post("bob", "viejo secreto")

	if err := st.HideDirect(ctx, dm.ID, "alice"); err != nil {
		t.Fatalf("HideDirect: %v", err)
	}
	if enLista(t, mustList(t, st, ctx, tm.ID, "alice"), dm.ID.String()) {
		t.Fatal("el directo borrado sigue en la barra de alice")
	}
	if !enLista(t, mustList(t, st, ctx, tm.ID, "bob"), dm.ID.String()) {
		t.Fatal("bob no lo ha borrado: debe seguir viéndolo")
	}
	if msgs, err := st.ListMessages(ctx, dm.ID, "alice", store.MessagePage{}); err != nil || len(msgs) != 0 {
		t.Fatalf("alice debe ver el historial vacío: %d mensajes, err=%v", len(msgs), err)
	}
	if msgs, err := st.ListMessages(ctx, dm.ID, "bob", store.MessagePage{}); err != nil || len(msgs) != 1 {
		t.Fatalf("bob conserva su historial: %d mensajes, err=%v", len(msgs), err)
	}
	if hits, err := st.SearchMessages(ctx, tm.ID, "alice", "secreto", 10); err != nil || len(hits) != 0 {
		t.Fatalf("la búsqueda de alice no debe encontrar lo borrado: %d, err=%v", len(hits), err)
	}

	// bob vuelve a escribir: reaparece para alice, solo con lo nuevo.
	post("bob", "hola de nuevo")
	if !enLista(t, mustList(t, st, ctx, tm.ID, "alice"), dm.ID.String()) {
		t.Fatal("un mensaje nuevo debe devolver el directo a la barra de alice")
	}
	msgs, err := st.ListMessages(ctx, dm.ID, "alice", store.MessagePage{})
	if err != nil || len(msgs) != 1 || msgs[0].Body != "hola de nuevo" {
		t.Fatalf("alice debe ver solo lo nuevo: %+v, err=%v", msgs, err)
	}

	// Reabrirlo también lo devuelve a la barra, aunque alguien hubiera salido.
	if err := st.HideDirect(ctx, dm.ID, "alice"); err != nil {
		t.Fatalf("HideDirect 2: %v", err)
	}
	if err := st.LeaveChannel(ctx, dm.ID, "bob"); err != nil {
		t.Fatalf("LeaveChannel bob: %v", err)
	}
	again, err := st.OpenDirect(ctx, tm.ID, "alice", "bob")
	if err != nil || again.ID != dm.ID {
		t.Fatalf("reabrir: id=%v err=%v", again.ID, err)
	}
	if !enLista(t, mustList(t, st, ctx, tm.ID, "alice"), dm.ID.String()) {
		t.Fatal("reabrir debe devolver el directo a la barra de quien lo abre")
	}
	if _, err := st.OpenDirect(ctx, tm.ID, "bob", "alice"); err != nil {
		t.Fatalf("bob reabriendo tras salir: %v", err)
	}
	if !enLista(t, mustList(t, st, ctx, tm.ID, "bob"), dm.ID.String()) {
		t.Fatal("quien había salido de un directo debe poder reabrirlo")
	}
}
