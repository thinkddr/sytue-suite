// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de las ETIQUETAS (labels) contra un PostgreSQL real
// (pgtest). Verifican: CRUD; aplicar/quitar N etiquetas por mensaje reflejado en
// el listado filtrado por etiqueta (keyset); contadores total/no-leído; que
// borrar una etiqueta la retira de los mensajes SIN borrar el mensaje; y el
// aislamiento por dueño (RLS por tenant + owner por account_id).
package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func TestLabelsCRUDApplyAndFilter(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")
	inbox, _ := st.MailboxByRole(ctx, acc.ID, domain.RoleInbox)

	m1 := seedMessage(t, ctx, st, acc, inbox.ID, "Uno", "a@x.eu", nil)
	m2 := seedMessage(t, ctx, st, acc, inbox.ID, "Dos", "b@x.eu", []string{domain.FlagSeen})

	// Crear una etiqueta.
	work, err := st.CreateLabel(ctx, acc.ID, "Trabajo", "brass")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if work.ID == uuid.Nil || work.Color != "brass" {
		t.Fatalf("etiqueta creada = %+v", work)
	}
	// Nombre duplicado (case-insensitive) => ErrLabelExists.
	if _, err := st.CreateLabel(ctx, acc.ID, "trabajo", ""); !errors.Is(err, domain.ErrLabelExists) {
		t.Fatalf("duplicada = %v, quiero ErrLabelExists", err)
	}
	// Nombre inválido / color fuera de paleta => ErrInvalidAddress.
	if _, err := st.CreateLabel(ctx, acc.ID, "", "brass"); !errors.Is(err, domain.ErrInvalidAddress) {
		t.Fatalf("nombre vacío = %v", err)
	}
	if _, err := st.CreateLabel(ctx, acc.ID, "X", "neon"); !errors.Is(err, domain.ErrInvalidAddress) {
		t.Fatalf("color inválido = %v", err)
	}

	// Segunda etiqueta: un mensaje puede llevar VARIAS (N por mensaje).
	urgent, err := st.CreateLabel(ctx, acc.ID, "Urgente", "clay")
	if err != nil {
		t.Fatalf("CreateLabel(2): %v", err)
	}

	// Aplicar ambas a m1 en lote; solo Trabajo a m2.
	if err := st.ApplyLabel(ctx, acc.ID, work.ID, []uuid.UUID{m1.ID, m2.ID}, true); err != nil {
		t.Fatalf("ApplyLabel(work): %v", err)
	}
	if err := st.ApplyLabel(ctx, acc.ID, urgent.ID, []uuid.UUID{m1.ID}, true); err != nil {
		t.Fatalf("ApplyLabel(urgent): %v", err)
	}
	// Idempotente: reaplicar no duplica el keyword.
	if err := st.ApplyLabel(ctx, acc.ID, work.ID, []uuid.UUID{m1.ID}, true); err != nil {
		t.Fatalf("ApplyLabel idempotente: %v", err)
	}
	got, _ := st.GetMessage(ctx, m1.ID)
	n := 0
	for _, f := range got.Flags {
		if f == work.ID.String() {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("keyword de etiqueta duplicado en flags: %v", got.Flags)
	}

	// Listado filtrado por etiqueta (keyset): Trabajo tiene m1 y m2 (recientes primero).
	byWork, err := st.ListMessagesByLabel(ctx, acc.ID, work.ID, 50, time.Time{}, uuid.Nil)
	if err != nil {
		t.Fatalf("ListMessagesByLabel(work): %v", err)
	}
	if len(byWork) != 2 || byWork[0].ID != m2.ID || byWork[1].ID != m1.ID {
		t.Fatalf("Trabajo = %d msgs (orden received_at DESC esperado)", len(byWork))
	}
	// Urgente solo m1.
	byUrgent, _ := st.ListMessagesByLabel(ctx, acc.ID, urgent.ID, 50, time.Time{}, uuid.Nil)
	if len(byUrgent) != 1 || byUrgent[0].ID != m1.ID {
		t.Fatalf("Urgente = %+v", byUrgent)
	}

	// Keyset: página de 1 en Trabajo + cursor a la siguiente.
	page1, _ := st.ListMessagesByLabel(ctx, acc.ID, work.ID, 1, time.Time{}, uuid.Nil)
	if len(page1) != 1 || page1[0].ID != m2.ID {
		t.Fatalf("página 1 = %+v", page1)
	}
	page2, _ := st.ListMessagesByLabel(ctx, acc.ID, work.ID, 1, page1[0].ReceivedAt, page1[0].ID)
	if len(page2) != 1 || page2[0].ID != m1.ID {
		t.Fatalf("página 2 (keyset) = %+v", page2)
	}

	// Contadores en ListLabels (una consulta, sin N+1): Trabajo total=2 unread=1 (m1
	// sin \Seen); Urgente total=1 unread=1.
	labels, err := st.ListLabels(ctx, acc.ID)
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	byName := map[string]domain.Label{}
	for _, l := range labels {
		byName[l.Name] = l
	}
	if l := byName["Trabajo"]; l.TotalCount != 2 || l.UnreadCount != 1 {
		t.Fatalf("Trabajo total/unread = %d/%d, quiero 2/1", l.TotalCount, l.UnreadCount)
	}
	if l := byName["Urgente"]; l.TotalCount != 1 || l.UnreadCount != 1 {
		t.Fatalf("Urgente total/unread = %d/%d, quiero 1/1", l.TotalCount, l.UnreadCount)
	}

	// Quitar Trabajo de m2 (add=false): desaparece del listado de esa etiqueta.
	if err := st.ApplyLabel(ctx, acc.ID, work.ID, []uuid.UUID{m2.ID}, false); err != nil {
		t.Fatalf("ApplyLabel(quitar): %v", err)
	}
	byWork, _ = st.ListMessagesByLabel(ctx, acc.ID, work.ID, 50, time.Time{}, uuid.Nil)
	if len(byWork) != 1 || byWork[0].ID != m1.ID {
		t.Fatalf("tras quitar, Trabajo = %+v", byWork)
	}

	// Renombrar/recolorear NO toca los mensajes (keyword = id estable).
	if err := st.UpdateLabel(ctx, acc.ID, work.ID, "Curro", "sage"); err != nil {
		t.Fatalf("UpdateLabel: %v", err)
	}
	stillWork, _ := st.ListMessagesByLabel(ctx, acc.ID, work.ID, 50, time.Time{}, uuid.Nil)
	if len(stillWork) != 1 || stillWork[0].ID != m1.ID {
		t.Fatalf("renombrar rompió la membresía: %+v", stillWork)
	}

	// Borrar la etiqueta: se retira de los mensajes SIN borrarlos.
	if err := st.DeleteLabel(ctx, acc.ID, work.ID); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}
	if m, err := st.GetMessage(ctx, m1.ID); err != nil {
		t.Fatalf("el mensaje NO debe borrarse al borrar la etiqueta: %v", err)
	} else if m.HasFlag(work.ID.String()) {
		t.Fatalf("el keyword de la etiqueta sigue en el mensaje: %v", m.Flags)
	}
	// Segundo borrado => ErrNotFound.
	if err := st.DeleteLabel(ctx, acc.ID, work.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("segundo DeleteLabel = %v, quiero ErrNotFound", err)
	}
}

func TestLabelOwnerIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	ada := seedAccount(t, ctx, st, "ada@sytue.eu")
	bob := seedAccount(t, ctx, st, "bob@sytue.eu") // MISMO tenant, otra cuenta
	adaInbox, _ := st.MailboxByRole(ctx, ada.ID, domain.RoleInbox)
	bobInbox, _ := st.MailboxByRole(ctx, bob.ID, domain.RoleInbox)

	adaLabel, err := st.CreateLabel(ctx, ada.ID, "Privada", "plum")
	if err != nil {
		t.Fatalf("CreateLabel(ada): %v", err)
	}
	bobMsg := seedMessage(t, ctx, st, bob, bobInbox.ID, "de Bob", "x@y.eu", nil)
	adaMsg := seedMessage(t, ctx, st, ada, adaInbox.ID, "de Ada", "z@y.eu", nil)

	// Bob NO puede aplicar la etiqueta de Ada (etiqueta ajena => ErrNotFound).
	if err := st.ApplyLabel(ctx, bob.ID, adaLabel.ID, []uuid.UUID{bobMsg.ID}, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aplicar etiqueta ajena = %v, quiero ErrNotFound", err)
	}
	// Ada NO puede etiquetar el mensaje de Bob: la etiqueta es suya pero el mensaje
	// no; el UPDATE está acotado por account_id, así que no afecta a filas ajenas.
	if err := st.ApplyLabel(ctx, ada.ID, adaLabel.ID, []uuid.UUID{bobMsg.ID}, true); err != nil {
		t.Fatalf("ApplyLabel(ada sobre msg de bob): %v", err)
	}
	got, _ := st.GetMessage(ctx, bobMsg.ID)
	if got.HasFlag(adaLabel.ID.String()) {
		t.Fatalf("se etiquetó un mensaje ajeno: %v", got.Flags)
	}
	// El propio mensaje de Ada sí se etiqueta.
	if err := st.ApplyLabel(ctx, ada.ID, adaLabel.ID, []uuid.UUID{adaMsg.ID}, true); err != nil {
		t.Fatalf("ApplyLabel(ada sobre su msg): %v", err)
	}
	list, _ := st.ListMessagesByLabel(ctx, ada.ID, adaLabel.ID, 50, time.Time{}, uuid.Nil)
	if len(list) != 1 || list[0].ID != adaMsg.ID {
		t.Fatalf("etiqueta de Ada = %+v, quiero solo su mensaje", list)
	}

	// Otro TENANT no ve la etiqueta (RLS de mail_labels).
	other, err := st.ListLabels(tenantCtx("other"), ada.ID)
	if err != nil {
		t.Fatalf("ListLabels(other): %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("aislamiento RLS de etiquetas roto: %d visibles", len(other))
	}
}
