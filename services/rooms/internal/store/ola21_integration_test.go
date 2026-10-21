// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 21 (objetos ricos: encuesta y tarea) contra
// PostgreSQL real (pgtest). Aquí está lo que un doble de prueba SIEMPRE da por
// bueno y solo el motor puede desmentir:
//
//   - que la migración 0031 aplica y que `card` viaja de ida y vuelta como jsonb
//     (una tarjeta guardada y releída conserva opciones, `multi`, responsable y fecha);
//   - que en una encuesta de respuesta ÚNICA cambiar de voto deja UN solo voto, y
//     que eso lo resuelve la MISMA transacción del store, no dos viajes del cliente;
//   - que votar es idempotente (el ON CONFLICT de la PK, no el código Go);
//   - que la agregación de una PÁGINA de mensajes reparte los votos en su tarjeta
//     y no mezcla los de un mensaje con los de otro;
//   - que la RLS de `message_card_votes` aísla por organización: los votos de una
//     no se ven ni se tocan desde otra.
//
// Detrás del build-tag `integration`. Sigue el patrón de ola9…ola18.
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// encuestaSembrada publica una encuesta con tres opciones en un canal nuevo.
func encuestaSembrada(t *testing.T, st *store.Store, multi bool) (domain.Team, domain.Channel, domain.Message) {
	t.Helper()
	ctx := tenantCtx("acme")
	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	card := domain.Card{Kind: domain.CardPoll, Options: []string{"lunes", "martes", "miércoles"}, Multi: multi}
	m, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice",
		Body: "¿Qué día quedamos?", Card: &card,
	})
	if err != nil {
		t.Fatalf("CreateMessage con tarjeta: %v", err)
	}
	return tm, ch, m
}

// alEquipo mete a alguien en el equipo. Hace falta para votar: en un canal PÚBLICO
// el acceso es la pertenencia al equipo (requireChannelAccess), no la del canal.
func alEquipo(t *testing.T, ctx context.Context, st *store.Store, teamID uuid.UUID, sub, nombre string) {
	t.Helper()
	if err := st.AddTeamMember(ctx, teamID, "alice", sub, nombre, domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember(%s): %v", sub, err)
	}
}

func TestTarjetaViajaDeIdaYVuelta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	_, ch, m := encuestaSembrada(t, st, true)
	if m.Card == nil {
		t.Fatal("el mensaje devuelto al crear ya debe traer su tarjeta")
	}
	if len(m.Card.Options) != 3 || m.Card.Options[2] != "miércoles" || !m.Card.Multi {
		t.Fatalf("tarjeta devuelta al crear: %+v", m.Card)
	}

	// Y al RELEERLA del feed (que es el camino real: otra sesión, otra consulta).
	msgs, err := st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Card == nil {
		t.Fatalf("el feed debe traer la tarjeta: %+v", msgs)
	}
	if len(msgs[0].Card.Options) != 3 || !msgs[0].Card.Multi {
		t.Fatalf("tarjeta releída: %+v", msgs[0].Card)
	}
	// El cuerpo sigue siendo la pregunta: es lo que ven la búsqueda y el push.
	if msgs[0].Body != "¿Qué día quedamos?" {
		t.Fatalf("cuerpo = %q", msgs[0].Body)
	}

	// Una TAREA con responsable y fecha: los tres campos sobreviven al jsonb.
	vence := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	tarea := domain.Card{Kind: domain.CardTask, AssigneeSub: "bob", AssigneeName: "Bob", Due: &vence}
	tm, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice",
		Body: "Enviar el informe", Card: &tarea,
	})
	if err != nil {
		t.Fatalf("CreateMessage tarea: %v", err)
	}
	if tm.Card == nil || tm.Card.AssigneeSub != "bob" || tm.Card.Due == nil || !tm.Card.Due.Equal(vence) {
		t.Fatalf("tarea devuelta: %+v", tm.Card)
	}

	// Un mensaje SIN tarjeta no se inventa una (la columna vale '{}').
	plano, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "hola",
	})
	if err != nil {
		t.Fatalf("CreateMessage plano: %v", err)
	}
	if plano.Card != nil {
		t.Fatalf("un mensaje normal no debe traer tarjeta: %+v", plano.Card)
	}
}

func TestEncuestaDeRespuestaUnicaSustituyeElVoto(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, _, m := encuestaSembrada(t, st, false)

	// Quien no está en el equipo NO vota en un canal público, y esa verja es la
	// misma que la de leer el canal: si esto dejara de fallar, cualquiera de la
	// organización podría votar en la encuesta de un equipo al que no pertenece.
	if _, _, err := st.SetCardVote(ctx, m.ID, "ajeno", "0"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("voto de alguien de fuera del equipo: err=%v, quiero ErrForbidden", err)
	}

	if _, votes, err := st.SetCardVote(ctx, m.ID, "alice", "0"); err != nil || len(votes) != 1 {
		t.Fatalf("primer voto: votes=%+v err=%v", votes, err)
	}
	// Votar OTRA opción no acumula: sustituye, y en la misma transacción.
	_, votes, err := st.SetCardVote(ctx, m.ID, "alice", "2")
	if err != nil {
		t.Fatalf("cambio de voto: %v", err)
	}
	if len(votes) != 1 || votes[0].Value != "2" || votes[0].Count != 1 {
		t.Fatalf("tras cambiar de voto quiero SOLO la opción 2: %+v", votes)
	}

	// Idempotencia: repetir el mismo voto no cuenta dos veces (ON CONFLICT de la PK).
	if _, votes, err = st.SetCardVote(ctx, m.ID, "alice", "2"); err != nil || votes[0].Count != 1 {
		t.Fatalf("voto repetido: %+v err=%v", votes, err)
	}

	// Otra persona SÍ suma, y los subs viajan para saber quién votó qué.
	alEquipo(t, ctx, st, tm.ID, "bob", "Bob")
	if _, votes, err = st.SetCardVote(ctx, m.ID, "bob", "2"); err != nil {
		t.Fatalf("voto de bob: %v", err)
	}
	if votes[0].Count != 2 || len(votes[0].Subs) != 2 {
		t.Fatalf("dos votantes en la opción 2: %+v", votes)
	}

	// Desvotar retira solo el suyo.
	if _, votes, err = st.ClearCardVote(ctx, m.ID, "bob", "2"); err != nil || votes[0].Count != 1 {
		t.Fatalf("tras desvotar bob: %+v err=%v", votes, err)
	}
}

func TestEncuestaDeVariasRespuestasAcumula(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	_, _, m := encuestaSembrada(t, st, true)
	if _, _, err := st.SetCardVote(ctx, m.ID, "alice", "0"); err != nil {
		t.Fatalf("voto 0: %v", err)
	}
	_, votes, err := st.SetCardVote(ctx, m.ID, "alice", "1")
	if err != nil {
		t.Fatalf("voto 1: %v", err)
	}
	if len(votes) != 2 {
		t.Fatalf("con multi los dos votos conviven: %+v", votes)
	}
}

func TestValorInvalidoNoSeVota(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	_, ch, m := encuestaSembrada(t, st, false)

	// Opción que no existe (la encuesta tiene 3).
	if _, _, err := st.SetCardVote(ctx, m.ID, "alice", "7"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("opción fuera de rango: err=%v, quiero ErrInvalidInput", err)
	}
	// «done» es de las tareas, no de las encuestas.
	if _, _, err := st.SetCardVote(ctx, m.ID, "alice", domain.CardDone); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("done en una encuesta: err=%v, quiero ErrInvalidInput", err)
	}
	// Un mensaje SIN tarjeta no es votable.
	plano, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "hola",
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if _, _, err := st.SetCardVote(ctx, plano.ID, "alice", "0"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("mensaje sin tarjeta: err=%v, quiero ErrInvalidInput", err)
	}
}

func TestTareaHechaYReabierta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	alEquipo(t, ctx, st, tm.ID, "bob", "Bob")
	card := domain.Card{Kind: domain.CardTask, AssigneeSub: "bob", AssigneeName: "Bob"}
	m, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "Enviar el informe", Card: &card,
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	if _, votes, err := st.SetCardVote(ctx, m.ID, "bob", domain.CardDone); err != nil || len(votes) != 1 {
		t.Fatalf("marcar hecha: %+v err=%v", votes, err)
	}
	// Releída del feed: la tarea sale hecha y se sabe QUIÉN la cerró.
	msgs, err := st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Card == nil || !msgs[0].Card.Done() {
		t.Fatalf("la tarea debe salir hecha: %+v", msgs[0].Card)
	}
	if len(msgs[0].Card.Votes[0].Subs) != 1 || msgs[0].Card.Votes[0].Subs[0] != "bob" {
		t.Fatalf("quién la cerró: %+v", msgs[0].Card.Votes)
	}

	if _, votes, err := st.ClearCardVote(ctx, m.ID, "bob", domain.CardDone); err != nil || len(votes) != 0 {
		t.Fatalf("reabrir: %+v err=%v", votes, err)
	}
}

func TestVotosDeVariosMensajesNoSeMezclan(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	alEquipo(t, ctx, st, tm.ID, "bob", "Bob")
	nueva := func(pregunta string) domain.Message {
		card := domain.Card{Kind: domain.CardPoll, Options: []string{"sí", "no"}}
		m, err := st.CreateMessage(ctx, store.NewMessageRecord{
			ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: pregunta, Card: &card,
		})
		if err != nil {
			t.Fatalf("CreateMessage: %v", err)
		}
		return m
	}
	a, b := nueva("¿A?"), nueva("¿B?")
	if _, _, err := st.SetCardVote(ctx, a.ID, "alice", "0"); err != nil {
		t.Fatalf("voto A: %v", err)
	}
	if _, _, err := st.SetCardVote(ctx, b.ID, "alice", "1"); err != nil {
		t.Fatalf("voto B: %v", err)
	}
	if _, _, err := st.SetCardVote(ctx, b.ID, "bob", "1"); err != nil {
		t.Fatalf("voto B de bob: %v", err)
	}

	msgs, err := st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	porID := map[string]*domain.Card{}
	for i := range msgs {
		porID[msgs[i].ID.String()] = msgs[i].Card
	}
	ca, cb := porID[a.ID.String()], porID[b.ID.String()]
	if ca == nil || len(ca.Votes) != 1 || ca.Votes[0].Value != "0" || ca.Votes[0].Count != 1 {
		t.Fatalf("votos de A: %+v", ca)
	}
	if cb == nil || len(cb.Votes) != 1 || cb.Votes[0].Value != "1" || cb.Votes[0].Count != 2 {
		t.Fatalf("votos de B: %+v", cb)
	}
}

// TestVotosAisladosPorOrganizacion comprueba la RLS de message_card_votes: los votos
// de una organización no se leen ni se tocan desde otra. Va contra el motor porque la
// política la evalúa PostgreSQL, no el código Go.
func TestVotosAisladosPorOrganizacion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)

	acme := tenantCtx("acme")
	_, ch := seedTeamChannel(t, acme, st, "alice", domain.ChannelPublic)
	card := domain.Card{Kind: domain.CardPoll, Options: []string{"sí", "no"}}
	m, err := st.CreateMessage(acme, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "¿Vamos?", Card: &card,
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if _, _, err := st.SetCardVote(acme, m.ID, "alice", "0"); err != nil {
		t.Fatalf("SetCardVote: %v", err)
	}

	// Desde OTRA organización, ese mensaje no existe: ni se vota ni se desvota.
	otra := tenantCtx("otra")
	if _, _, err := st.SetCardVote(otra, m.ID, "mallory", "1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("votar cross-tenant: err=%v, quiero ErrNotFound", err)
	}
	if _, _, err := st.ClearCardVote(otra, m.ID, "alice", "0"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("desvotar cross-tenant: err=%v, quiero ErrNotFound", err)
	}

	// Y el voto de acme sigue intacto.
	msgs, err := st.ListMessages(acme, ch.ID, "alice", store.MessagePage{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Card == nil || len(msgs[0].Card.Votes) != 1 {
		t.Fatalf("el voto propio debe seguir ahí: %+v", msgs)
	}
}
