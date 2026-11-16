// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// «También enviar al canal» y el indicador de hilo con avatares (D-ROOM-05) contra
// PostgreSQL real: la respuesta difundida entra en el feed sin dejar de ser del hilo,
// y las raíces traen quién respondió y cuándo fue la última respuesta.
package store_test

import (
	"context"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestRespuestaTambienAlCanalEntraEnElFeed(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	_, ch := seedTeamChannel(ctx, t, st, "alice", domain.ChannelPublic)

	root, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "tema"})
	if err != nil {
		t.Fatalf("raíz: %v", err)
	}
	solo, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, ParentID: &root.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "solo en el hilo"})
	if err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	if solo.AlsoInChannel {
		t.Fatal("una respuesta normal no va al canal")
	}
	difundida, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, ParentID: &root.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "también al canal", AlsoInChannel: true})
	if err != nil {
		t.Fatalf("respuesta difundida: %v", err)
	}
	if !difundida.AlsoInChannel {
		t.Fatal("la respuesta difundida debe volver marcada")
	}
	// En un mensaje raíz la marca no significa nada y no se guarda.
	otraRaiz, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "otra", AlsoInChannel: true})
	if err != nil {
		t.Fatalf("otra raíz: %v", err)
	}
	if otraRaiz.AlsoInChannel {
		t.Fatal("una raíz no lleva also_in_channel")
	}

	feed, err := st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	ids := map[string]bool{}
	for _, m := range feed {
		ids[m.ID.String()] = true
	}
	if !ids[root.ID.String()] || !ids[difundida.ID.String()] || !ids[otraRaiz.ID.String()] {
		t.Fatalf("el feed debe traer las raíces y la respuesta difundida: %+v", feed)
	}
	if ids[solo.ID.String()] || len(feed) != 3 {
		t.Fatalf("la respuesta normal no va al feed (len=%d)", len(feed))
	}

	// Sigue siendo del hilo, una sola vez, y cuenta en reply_count.
	r, replies, err := st.ListThread(ctx, root.ID, "alice")
	if err != nil {
		t.Fatalf("ListThread: %v", err)
	}
	if r.ReplyCount != 2 || len(replies) != 2 || !replies[1].AlsoInChannel {
		t.Fatalf("hilo: reply_count=%d replies=%+v", r.ReplyCount, replies)
	}

	// El permalink a la respuesta difundida ancla en su raíz y la ventana la incluye.
	around, raiz, err := st.MessagesAround(ctx, ch.ID, "alice", difundida.ID, 10)
	if err != nil {
		t.Fatalf("MessagesAround: %v", err)
	}
	if raiz != root.ID {
		t.Fatalf("raíz del permalink=%s, quiero %s", raiz, root.ID)
	}
	encontrada := false
	for _, m := range around {
		encontrada = encontrada || m.ID == difundida.ID
	}
	if !encontrada {
		t.Fatalf("la ventana alrededor debe traer la respuesta difundida: %+v", around)
	}

	// Borrarla la quita del feed y del hilo a la vez: es una sola fila.
	if _, err := st.DeleteMessage(ctx, difundida.ID, "alice"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	feed, err = st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 50})
	if err != nil || len(feed) != 2 {
		t.Fatalf("tras borrar, feed=%d err=%v", len(feed), err)
	}

	// Borrar la RAÍZ no borra sus respuestas, pero la difundida sale del feed: su
	// «Ver el hilo» llevaría a un hilo que ya no existe.
	huerfana, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, ParentID: &otraRaiz.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "también", AlsoInChannel: true})
	if err != nil {
		t.Fatalf("difundida en otra raíz: %v", err)
	}
	if _, err := st.DeleteMessage(ctx, otraRaiz.ID, "alice"); err != nil {
		t.Fatalf("DeleteMessage raíz: %v", err)
	}
	feed, err = st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 50})
	if err != nil || len(feed) != 1 || feed[0].ID != root.ID {
		t.Fatalf("con la raíz borrada su difundida sigue en el feed (%s): %+v err=%v", huerfana.ID, feed, err)
	}
}

func TestIndicadorDeHiloConParticipantesYUltimaRespuesta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	tm, ch := seedTeamChannel(ctx, t, st, "alice", domain.ChannelPublic)
	for _, sub := range []string{"bob", "carol", "dave"} {
		if err := st.AddTeamMember(ctx, tm.ID, "alice", sub, sub, domain.RoleMember); err != nil {
			t.Fatalf("AddTeamMember %s: %v", sub, err)
		}
	}
	root, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "tema"})
	if err != nil {
		t.Fatalf("raíz: %v", err)
	}
	sinHilo, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "nadie responde"})
	if err != nil {
		t.Fatalf("raíz sin hilo: %v", err)
	}
	// bob, carol, dave y otra vez bob: los avatares van del que respondió más
	// recientemente hacia atrás, sin repetir, y como mucho tres.
	var ultima domain.Message
	for _, a := range []struct{ sub, name string }{{"bob", "Bob"}, {"carol", "Carol"}, {"dave", "Dave"}, {"bob", "Bob B."}} {
		ultima, err = st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, ParentID: &root.ID, AuthorSub: a.sub, AuthorName: a.name, Body: "respuesta de " + a.sub})
		if err != nil {
			t.Fatalf("respuesta %s: %v", a.sub, err)
		}
		time.Sleep(5 * time.Millisecond) // created_at estrictamente creciente
	}

	feed, err := st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	var conHilo, solo *domain.Message
	for i := range feed {
		switch feed[i].ID {
		case root.ID:
			conHilo = &feed[i]
		case sinHilo.ID:
			solo = &feed[i]
		}
	}
	if conHilo == nil || solo == nil {
		t.Fatalf("faltan raíces en el feed: %+v", feed)
	}
	got := conHilo.ThreadParticipants
	if len(got) != 3 || got[0].Sub != "bob" || got[1].Sub != "dave" || got[2].Sub != "carol" {
		t.Fatalf("participantes=%+v, quiero bob, dave, carol", got)
	}
	if got[0].Name != "Bob B." {
		t.Fatalf("el nombre es el de su última respuesta: %q", got[0].Name)
	}
	if conHilo.LastReplyAt == nil || !conHilo.LastReplyAt.Equal(ultima.CreatedAt) {
		t.Fatalf("last_reply_at=%v, quiero %v", conHilo.LastReplyAt, ultima.CreatedAt)
	}
	if solo.LastReplyAt != nil || len(solo.ThreadParticipants) != 0 {
		t.Fatalf("una raíz sin respuestas no lleva indicador: %+v", solo)
	}

	// El hilo abierto trae el mismo indicador en su raíz.
	r, _, err := st.ListThread(ctx, root.ID, "alice")
	if err != nil {
		t.Fatalf("ListThread: %v", err)
	}
	if len(r.ThreadParticipants) != 3 || r.LastReplyAt == nil {
		t.Fatalf("raíz del hilo sin indicador: %+v", r)
	}

	// Una respuesta borrada deja de contar: la última pasa a ser la de dave.
	if _, err := st.DeleteMessage(ctx, ultima.ID, "bob"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	feed, err = st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for _, m := range feed {
		if m.ID == root.ID && (len(m.ThreadParticipants) != 3 || m.ThreadParticipants[0].Sub != "dave" || m.ThreadParticipants[2].Name != "Bob") {
			t.Fatalf("tras borrar, participantes=%+v", m.ThreadParticipants)
		}
	}
}

// Una respuesta PROGRAMADA con «También enviar al canal» guarda la casilla y, al vencer, se
// publica también en el feed; la marca no se guarda en una raíz programada.
func TestRespuestaProgramadaTambienAlCanal(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	_, ch := seedTeamChannel(ctx, t, st, "alice", domain.ChannelPublic)
	root, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "tema"})
	if err != nil {
		t.Fatalf("raíz: %v", err)
	}
	sm, err := st.CreateScheduled(ctx, store.NewScheduledRecord{
		ChannelID: ch.ID, ParentID: &root.ID, AuthorSub: "alice", AuthorName: "Alice",
		Body: "luego, también al canal", SendAt: time.Now().Add(-time.Minute), AlsoInChannel: true,
	})
	if err != nil || !sm.AlsoInChannel {
		t.Fatalf("CreateScheduled: %+v %v", sm, err)
	}
	raiz, err := st.CreateScheduled(ctx, store.NewScheduledRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "raíz", SendAt: time.Now().Add(time.Hour), AlsoInChannel: true,
	})
	if err != nil || raiz.AlsoInChannel {
		t.Fatalf("una raíz programada no lleva also_in_channel: %+v %v", raiz, err)
	}
	var publicados []domain.Message
	if _, err := st.ClaimDueScheduled(context.Background(), time.Now(), 100, func(_ context.Context, _ pg.Querier, m domain.Message) error {
		publicados = append(publicados, m)
		return nil
	}); err != nil {
		t.Fatalf("ClaimDueScheduled: %v", err)
	}
	if len(publicados) != 1 || !publicados[0].AlsoInChannel || publicados[0].ParentID == nil {
		t.Fatalf("publicados = %+v", publicados)
	}
	feed, err := st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	enFeed := false
	for _, m := range feed {
		enFeed = enFeed || m.ID == publicados[0].ID
	}
	if !enFeed {
		t.Fatal("la respuesta programada «también al canal» no entra en el feed")
	}
}
