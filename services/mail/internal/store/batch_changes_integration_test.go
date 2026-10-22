// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de las operaciones en LOTE (MoveMessages / SetFlagsBatch) y
// del delta incremental (Changes) contra un PostgreSQL real (pgtest). Verifican:
// mover/marcar N mensajes en una operación; el aislamiento por dueño (ids ajenos
// omitidos, destino ajeno rechazado); y que Changes reporta SOLO lo del usuario,
// de forma incremental desde un cursor de estado (cambios y borrados).
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

func TestBatchMoveAndFlagsOwnerScoped(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	ada := seedAccount(t, ctx, st, "ada@sytue.eu")
	bob := seedAccount(t, ctx, st, "bob@sytue.eu")
	adaInbox, _ := st.MailboxByRole(ctx, ada.ID, domain.RoleInbox)
	adaArchive, _ := st.MailboxByRole(ctx, ada.ID, domain.RoleArchive)
	bobInbox, _ := st.MailboxByRole(ctx, bob.ID, domain.RoleInbox)

	m1 := seedMessage(t, ctx, st, ada, adaInbox.ID, "Uno", "x@y.eu", nil)
	m2 := seedMessage(t, ctx, st, ada, adaInbox.ID, "Dos", "x@y.eu", nil)
	bobMsg := seedMessage(t, ctx, st, bob, bobInbox.ID, "Ajeno", "z@y.eu", nil)

	// Mover en lote m1+m2 a Archivo, incluyendo el mensaje AJENO de Bob: solo se
	// mueven los de Ada (owner-scoped por account_id); el de Bob se omite.
	moved, err := st.MoveMessages(ctx, ada.ID, []uuid.UUID{m1.ID, m2.ID, bobMsg.ID}, adaArchive.ID)
	if err != nil {
		t.Fatalf("MoveMessages: %v", err)
	}
	if moved != 2 {
		t.Fatalf("movidos = %d, quiero 2 (el ajeno se omite)", moved)
	}
	// Ada: Inbox queda vacío, Archivo con 2. Con UIDs distintos (índice único).
	if got := countMessages(t, ctx, st, adaInbox.ID); got != 0 {
		t.Fatalf("Inbox de Ada = %d, quiero 0", got)
	}
	if got := countMessages(t, ctx, st, adaArchive.ID); got != 2 {
		t.Fatalf("Archivo de Ada = %d, quiero 2", got)
	}
	// El mensaje de Bob NO se movió.
	if got := countMessages(t, ctx, st, bobInbox.ID); got != 1 {
		t.Fatalf("Inbox de Bob = %d, quiero 1 (intacto)", got)
	}

	// Destino AJENO (buzón de Bob): rechazado, no mueve nada.
	if _, err := st.MoveMessages(ctx, ada.ID, []uuid.UUID{m1.ID}, bobInbox.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mover a destino ajeno = %v, quiero ErrNotFound", err)
	}

	// Marcar leídos en lote m1+m2 (+ el ajeno de Bob, que se omite).
	n, err := st.SetFlagsBatch(ctx, ada.ID, []uuid.UUID{m1.ID, m2.ID, bobMsg.ID}, domain.FlagSeen, true)
	if err != nil {
		t.Fatalf("SetFlagsBatch: %v", err)
	}
	if n != 2 {
		t.Fatalf("marcados = %d, quiero 2 (el ajeno se omite)", n)
	}
	// El de Bob sigue sin leer (no lo tocó el lote de Ada).
	if bm, _ := st.GetMessage(ctx, bobMsg.ID); bm.HasFlag(domain.FlagSeen) {
		t.Fatalf("el mensaje de Bob no debía marcarse leído")
	}
	// Quitar el flag en lote (add=false).
	if _, err := st.SetFlagsBatch(ctx, ada.ID, []uuid.UUID{m1.ID}, domain.FlagSeen, false); err != nil {
		t.Fatalf("SetFlagsBatch(remove): %v", err)
	}
	if am, _ := st.GetMessage(ctx, m1.ID); am.HasFlag(domain.FlagSeen) {
		t.Fatalf("m1 debía quedar sin leer")
	}
}

func TestChangesIncrementalOwnerScoped(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	ada := seedAccount(t, ctx, st, "ada2@sytue.eu")
	bob := seedAccount(t, ctx, st, "bob2@sytue.eu")
	adaInbox, _ := st.MailboxByRole(ctx, ada.ID, domain.RoleInbox)
	adaTrash, _ := st.MailboxByRole(ctx, ada.ID, domain.RoleTrash)
	bobInbox, _ := st.MailboxByRole(ctx, bob.ID, domain.RoleInbox)

	// Estado inicial (bootstrap): sin cambios, fija el cursor «ahora».
	changed, destroyed, ts0, id0, more, err := st.Changes(ctx, ada.ID, time.Time{}, uuid.Nil, 100)
	if err != nil {
		t.Fatalf("Changes bootstrap: %v", err)
	}
	if len(changed) != 0 || len(destroyed) != 0 || more || ts0.IsZero() {
		t.Fatalf("bootstrap = changed %d, destroyed %d, more %v, ts0 %v", len(changed), len(destroyed), more, ts0)
	}

	// Tras el bootstrap: llega correo a Ada y a Bob; Ada marca uno leído y borra otro.
	m1 := seedMessage(t, ctx, st, ada, adaInbox.ID, "Nuevo", "x@y.eu", nil)
	m2 := seedMessage(t, ctx, st, ada, adaInbox.ID, "Otro", "x@y.eu", nil)
	seedMessage(t, ctx, st, bob, bobInbox.ID, "Ajeno", "z@y.eu", nil) // no debe aparecer
	if err := st.SetFlags(ctx, ada.ID, m1.ID, domain.FlagSeen, true); err != nil {
		t.Fatalf("SetFlags: %v", err)
	}
	// Borrar m2 (soft-delete) → debe aparecer como destroyed en el delta.
	if _, err := st.MoveMessages(ctx, ada.ID, []uuid.UUID{m2.ID}, adaTrash.ID); err != nil {
		t.Fatalf("mover m2 a papelera: %v", err)
	}
	if err := st.DeleteMessage(ctx, ada.ID, m2.ID); err != nil {
		t.Fatalf("DeleteMessage(m2): %v", err)
	}

	// Delta desde el cursor del bootstrap: SOLO cambios de Ada, incrementales.
	changed, destroyed, ts1, id1, _, err := st.Changes(ctx, ada.ID, ts0, id0, 100)
	if err != nil {
		t.Fatalf("Changes delta: %v", err)
	}
	// m1 (vivo, cambió al insertarse y al marcarse) aparece en changed una vez.
	seenM1 := false
	for _, c := range changed {
		if c.AccountID != ada.ID {
			t.Fatalf("Changes filtró mal: apareció un mensaje de otra cuenta %v", c.AccountID)
		}
		if c.ID == m1.ID {
			seenM1 = true
		}
		if c.ID == m2.ID {
			t.Fatalf("m2 borrado no debe estar en changed")
		}
	}
	if !seenM1 {
		t.Fatalf("m1 debía aparecer en changed: %+v", changed)
	}
	// m2 borrado aparece en destroyed.
	foundM2 := false
	for _, d := range destroyed {
		if d == m2.ID {
			foundM2 = true
		}
	}
	if !foundM2 {
		t.Fatalf("m2 borrado debía aparecer en destroyed: %v", destroyed)
	}
	// El nuevo cursor avanzó respecto al del bootstrap.
	if !ts1.After(ts0) {
		t.Fatalf("el cursor no avanzó: ts1=%v ts0=%v", ts1, ts0)
	}

	// Volver a sondear desde el cursor nuevo (ts1,id1): nada cambió → delta vacío.
	changed2, destroyed2, ts2, _, _, err := st.Changes(ctx, ada.ID, ts1, id1, 100)
	if err != nil {
		t.Fatalf("Changes segundo delta: %v", err)
	}
	if len(changed2) != 0 || len(destroyed2) != 0 {
		t.Fatalf("segundo delta debía ser vacío: changed %d destroyed %d", len(changed2), len(destroyed2))
	}
	_ = ts2
}

// countMessages cuenta los mensajes vivos de un buzón (helper de estos tests).
func countMessages(t *testing.T, ctx context.Context, st *store.Store, mbox uuid.UUID) int {
	t.Helper()
	msgs, err := st.ListMessages(ctx, mbox, 200, time.Time{}, uuid.Nil)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	return len(msgs)
}
