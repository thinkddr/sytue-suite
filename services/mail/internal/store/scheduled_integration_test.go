// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Integración del ENVÍO DIFERIDO (programado + deshacer) sobre PostgreSQL real
// (pgtest). Verifican: una fila retenida con release_at futuro no se entrega ni
// se libera antes de tiempo; el worker la libera al vencer (copia en Sent + fila
// de entrega); cancelar/editar antes de la ventana; idempotencia (no doble
// envío); y aislamiento por dueño (RLS + account_id). Reusa los helpers de
// store_integration_test.go (mismo paquete store_test).
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// rawEML es un .eml mínimo válido que devuelve el getBlob de los tests (el worker
// lo parsea para crear la copia en Sent al liberar).
var rawEML = []byte("From: ada@sytue.eu\r\nTo: bob@example.com\r\nSubject: Programado\r\nMessage-Id: <sched@sytue.eu>\r\n\r\nhola")

func fixedBlob(context.Context, string, string) ([]byte, error) { return rawEML, nil }

func heldMsg(acc domain.Account) *domain.OutboundMessage {
	return &domain.OutboundMessage{
		AccountID: acc.ID, From: "ada@sytue.eu",
		Recipients: []string{"bob@example.com"}, BlobKey: "messages/x.eml", Subject: "Programado",
	}
}

func TestScheduledSendReleaseAndIdempotency(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")

	future := time.Now().Add(time.Hour)
	id, err := st.EnqueueHeld(ctx, heldMsg(acc), future, domain.HoldScheduled)
	if err != nil {
		t.Fatalf("EnqueueHeld: %v", err)
	}

	// La entrega NO reclama una fila retenida, y no ha vencido: nada se envía antes.
	if claimed, _ := st.ClaimDue(context.Background(), 10); len(claimed) != 0 {
		t.Fatalf("ClaimDue reclamó una fila retenida: %d", len(claimed))
	}
	if n, err := st.PromoteDueHeld(context.Background(), time.Now(), 10, fixedBlob); err != nil || n != 0 {
		t.Fatalf("liberó antes de vencer: n=%d err=%v (quiero 0)", n, err)
	}

	// Aparece en «Programados» (keyset).
	sched, err := st.ListScheduled(ctx, acc.ID, 50, time.Time{}, uuid.Nil)
	if err != nil {
		t.Fatalf("ListScheduled: %v", err)
	}
	if len(sched) != 1 || sched[0].ID != id || sched[0].Subject != "Programado" {
		t.Fatalf("programados = %+v", sched)
	}

	// Al VENCER, el worker lo libera: crea copia en Sent + fila(s) de entrega.
	if n, err := st.PromoteDueHeld(context.Background(), future.Add(time.Minute), 10, fixedBlob); err != nil || n != 1 {
		t.Fatalf("liberar al vencer: n=%d err=%v (quiero 1)", n, err)
	}

	// Ya no está en «Programados»; la copia en Sent existe.
	if sched, _ := st.ListScheduled(ctx, acc.ID, 50, time.Time{}, uuid.Nil); len(sched) != 0 {
		t.Fatalf("sigue programado tras liberar: %d", len(sched))
	}
	sent, _ := st.MailboxByRole(ctx, acc.ID, domain.RoleSent)
	inSent, _ := st.ListMessages(ctx, sent.ID, 10, time.Time{}, uuid.Nil)
	if len(inSent) != 1 || inSent[0].Subject != "Programado" {
		t.Fatalf("Sent = %+v, quiero la copia liberada", inSent)
	}
	// Ahora SÍ hay una fila de entrega normal reclamable (hold='').
	if claimed, _ := st.ClaimDue(context.Background(), 10); len(claimed) != 1 {
		t.Fatalf("tras liberar, ClaimDue = %d, quiero 1", len(claimed))
	}

	// Idempotencia: liberar otra vez no reenvía ni duplica la copia en Sent.
	if n, _ := st.PromoteDueHeld(context.Background(), future.Add(time.Hour), 10, fixedBlob); n != 0 {
		t.Fatalf("doble liberación: %d, quiero 0", n)
	}
	if inSent2, _ := st.ListMessages(ctx, sent.ID, 10, time.Time{}, uuid.Nil); len(inSent2) != 1 {
		t.Fatalf("doble copia en Sent: %d", len(inSent2))
	}
}

func TestScheduledCancelBeforeRelease(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")

	id, _ := st.EnqueueHeld(ctx, heldMsg(acc), time.Now().Add(time.Hour), domain.HoldScheduled)

	blobKey, hold, err := st.CancelHeld(ctx, id, acc.ID)
	if err != nil {
		t.Fatalf("CancelHeld: %v", err)
	}
	if blobKey != "messages/x.eml" || hold != domain.HoldScheduled {
		t.Fatalf("cancel devolvió %q/%q", blobKey, hold)
	}
	// Un cancelado ni se libera ni se lista.
	if n, _ := st.PromoteDueHeld(context.Background(), time.Now().Add(2*time.Hour), 10, fixedBlob); n != 0 {
		t.Fatalf("liberó un cancelado: %d", n)
	}
	if sched, _ := st.ListScheduled(ctx, acc.ID, 50, time.Time{}, uuid.Nil); len(sched) != 0 {
		t.Fatalf("cancelado sigue en programados: %d", len(sched))
	}
	// Cancelar de nuevo => ErrScheduleGone (ya no está 'queued').
	if _, _, err := st.CancelHeld(ctx, id, acc.ID); !errors.Is(err, domain.ErrScheduleGone) {
		t.Fatalf("segundo cancel = %v, quiero ErrScheduleGone", err)
	}
}

func TestScheduledReschedule(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")

	id, _ := st.EnqueueHeld(ctx, heldMsg(acc), time.Now().Add(time.Hour), domain.HoldScheduled)

	newAt := time.Now().Add(3 * time.Hour)
	if err := st.RescheduleHeld(ctx, id, acc.ID, newAt); err != nil {
		t.Fatalf("RescheduleHeld: %v", err)
	}
	sched, _ := st.ListScheduled(ctx, acc.ID, 50, time.Time{}, uuid.Nil)
	if len(sched) != 1 || !sched[0].NextRetryAt.Truncate(time.Second).Equal(newAt.UTC().Truncate(time.Second)) {
		t.Fatalf("hora no actualizada: %+v", sched)
	}
	// Entre la hora vieja y la nueva NO se libera.
	if n, _ := st.PromoteDueHeld(context.Background(), time.Now().Add(2*time.Hour), 10, fixedBlob); n != 0 {
		t.Fatalf("liberó antes de la nueva hora: %d", n)
	}
	// A la nueva hora sí.
	if n, _ := st.PromoteDueHeld(context.Background(), newAt.Add(time.Minute), 10, fixedBlob); n != 1 {
		t.Fatalf("no liberó a la nueva hora: %d", n)
	}
}

func TestUndoSendCancelWithinWindow(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")

	// Deshacer = fila retenida con release a unos segundos; no aparece en programados.
	id, _ := st.EnqueueHeld(ctx, heldMsg(acc), time.Now().Add(10*time.Second), domain.HoldUndo)
	if sched, _ := st.ListScheduled(ctx, acc.ID, 50, time.Time{}, uuid.Nil); len(sched) != 0 {
		t.Fatalf("un 'undo' no debe listarse como programado: %d", len(sched))
	}
	// Deshacer dentro de la ventana: cancela y devuelve el blob para reabrir borrador.
	if _, hold, err := st.CancelHeld(ctx, id, acc.ID); err != nil || hold != domain.HoldUndo {
		t.Fatalf("CancelHeld(undo): hold=%q err=%v", hold, err)
	}
	// Tras deshacer, jamás se libera (no se envía).
	if n, _ := st.PromoteDueHeld(context.Background(), time.Now().Add(time.Minute), 10, fixedBlob); n != 0 {
		t.Fatalf("liberó un 'undo' cancelado: %d", n)
	}
}

func TestScheduledOwnerIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	accA := seedAccount(t, ctx, st, "ada@sytue.eu")
	accB := seedAccount(t, ctx, st, "bob@sytue.eu")

	future := time.Now().Add(time.Hour)
	id, _ := st.EnqueueHeld(ctx, heldMsg(accA), future, domain.HoldScheduled)

	// B (mismo tenant) no ve, no cancela ni reprograma el diferido de A.
	if schedB, _ := st.ListScheduled(ctx, accB.ID, 50, time.Time{}, uuid.Nil); len(schedB) != 0 {
		t.Fatalf("B ve programados de A: %d", len(schedB))
	}
	if _, _, err := st.CancelHeld(ctx, id, accB.ID); !errors.Is(err, domain.ErrScheduleGone) {
		t.Fatalf("B canceló el de A: %v", err)
	}
	if err := st.RescheduleHeld(ctx, id, accB.ID, future.Add(time.Hour)); !errors.Is(err, domain.ErrScheduleGone) {
		t.Fatalf("B reprogramó el de A: %v", err)
	}
	// A sí lo ve y puede cancelarlo.
	if schedA, _ := st.ListScheduled(ctx, accA.ID, 50, time.Time{}, uuid.Nil); len(schedA) != 1 {
		t.Fatalf("A no ve su propio programado: %d", len(schedA))
	}
	if _, _, err := st.CancelHeld(ctx, id, accA.ID); err != nil {
		t.Fatalf("A no pudo cancelar el suyo: %v", err)
	}
}
