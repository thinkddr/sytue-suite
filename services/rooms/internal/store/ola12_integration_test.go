// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 12 (profundidad de mensaje) contra PostgreSQL
// real (pgtest): mensajes PROGRAMADOS (scheduled_messages, mig. 0010) —el drenaje
// del worker publica el vencido UNA sola vez (sin doble envío)— y PRIORIDAD +
// ACUSE de recibo (messages.priority/requires_ack + message_acks, mig. 0011) —el
// conteo agregado y su idempotencia—. Detrás del build-tag `integration`. Sigue el
// patrón de ola9_integration_test.go.
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestScheduledDuePublishesOnce(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	// Programar en el pasado (ya vencido): el drenaje debe recogerlo.
	if _, err := st.CreateScheduled(ctx, store.NewScheduledRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice",
		Body: "hola futuro", SendAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("CreateScheduled: %v", err)
	}

	var delivered []domain.Message
	publish := func(_ context.Context, _ pg.Querier, m domain.Message) error {
		delivered = append(delivered, m)
		return nil
	}

	// Primer drenaje: reclama y publica exactamente uno.
	n, err := st.ClaimDueScheduled(context.Background(), time.Now(), 100, publish)
	if err != nil || n != 1 {
		t.Fatalf("claim1 n=%d err=%v, quiero 1", n, err)
	}
	if len(delivered) != 1 || delivered[0].Body != "hola futuro" {
		t.Fatalf("entregados=%+v, quiero 1", delivered)
	}

	// Segundo drenaje: la fila ya se borró, no hay doble envío.
	n2, err := st.ClaimDueScheduled(context.Background(), time.Now(), 100, publish)
	if err != nil || n2 != 0 {
		t.Fatalf("claim2 n=%d err=%v, quiero 0", n2, err)
	}
	if len(delivered) != 1 {
		t.Fatalf("doble envío: %d entregas", len(delivered))
	}

	// El mensaje quedó publicado en el canal.
	msgs, err := st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	found := false
	for _, m := range msgs {
		if m.Body == "hola futuro" {
			found = true
		}
	}
	if !found {
		t.Fatal("el mensaje programado no aparece en el canal tras el drenaje")
	}

	// Ya no quedan programados del autor.
	if list, err := st.ListScheduled(ctx, "alice", nil); err != nil || len(list) != 0 {
		t.Fatalf("programados restantes=%d err=%v, quiero 0", len(list), err)
	}
}

func TestScheduledFutureNotClaimed(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	sm, err := st.CreateScheduled(ctx, store.NewScheduledRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice",
		Body: "para más tarde", SendAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateScheduled: %v", err)
	}
	// No vencido: no se reclama.
	n, err := st.ClaimDueScheduled(context.Background(), time.Now(), 100,
		func(context.Context, pg.Querier, domain.Message) error { return nil })
	if err != nil || n != 0 {
		t.Fatalf("claim n=%d err=%v, quiero 0 (aún futuro)", n, err)
	}
	// Editable y cancelable por el autor.
	newAt := time.Now().Add(2 * time.Hour)
	if _, err := st.UpdateScheduled(ctx, sm.ID, "alice", store.UpdateScheduledRecord{SendAt: &newAt}); err != nil {
		t.Fatalf("UpdateScheduled: %v", err)
	}
	if err := st.CancelScheduled(ctx, sm.ID, "alice"); err != nil {
		t.Fatalf("CancelScheduled: %v", err)
	}
	if err := st.CancelScheduled(ctx, sm.ID, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cancelar dos veces debe ser ErrNotFound, got %v", err)
	}
}

func TestMessageAckCountAndIdempotent(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	for _, sub := range []string{"bob", "carol"} {
		if err := st.AddTeamMember(ctx, tm.ID, "alice", sub, sub, domain.RoleMember); err != nil {
			t.Fatalf("AddTeamMember %s: %v", sub, err)
		}
	}

	m, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice",
		Body: "confirmad, por favor", Priority: domain.PriorityUrgent, RequiresAck: true,
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if m.Priority != domain.PriorityUrgent || !m.RequiresAck {
		t.Fatalf("mensaje priority=%q requires_ack=%v, quiero urgent/true", m.Priority, m.RequiresAck)
	}

	// bob confirma; el conteo sube a 1. Idempotente: repetir no lo cambia.
	if _, count, err := st.AckMessage(ctx, m.ID, "bob"); err != nil || count != 1 {
		t.Fatalf("ack bob count=%d err=%v, quiero 1", count, err)
	}
	if _, count, err := st.AckMessage(ctx, m.ID, "bob"); err != nil || count != 1 {
		t.Fatalf("ack bob idempotente count=%d err=%v, quiero 1", count, err)
	}
	// alice también confirma → 2.
	if _, count, err := st.AckMessage(ctx, m.ID, "alice"); err != nil || count != 2 {
		t.Fatalf("ack alice count=%d err=%v, quiero 2", count, err)
	}

	// La lectura agrega AckCount y marca Acked per-visor (bob confirmó).
	bobView := findMessage(t, listMsgs(t, st, ctx, ch.ID, "bob"), m.ID)
	if bobView.AckCount != 2 || !bobView.Acked {
		t.Fatalf("bob ve AckCount=%d Acked=%v, quiero 2/true", bobView.AckCount, bobView.Acked)
	}
	// carol NO confirmó: ve el conteo agregado pero Acked=false (per-visor).
	carolView := findMessage(t, listMsgs(t, st, ctx, ch.ID, "carol"), m.ID)
	if carolView.AckCount != 2 || carolView.Acked {
		t.Fatalf("carol ve AckCount=%d Acked=%v, quiero 2/false", carolView.AckCount, carolView.Acked)
	}

	// Confirmar un mensaje que NO pide acuse → ErrInvalidInput.
	plain, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: "sin acuse"})
	if err != nil {
		t.Fatalf("CreateMessage plano: %v", err)
	}
	if _, _, err := st.AckMessage(ctx, plain.ID, "bob"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("ack a mensaje sin requires_ack debe ser ErrInvalidInput, got %v", err)
	}
}

func listMsgs(t *testing.T, st *store.Store, ctx context.Context, channelID uuid.UUID, subject string) []domain.Message {
	t.Helper()
	msgs, err := st.ListMessages(ctx, channelID, subject, store.MessagePage{})
	if err != nil {
		t.Fatalf("ListMessages %s: %v", subject, err)
	}
	return msgs
}

func findMessage(t *testing.T, msgs []domain.Message, id uuid.UUID) domain.Message {
	t.Helper()
	for _, m := range msgs {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("el mensaje %s no está en la lista", id)
	return domain.Message{}
}
