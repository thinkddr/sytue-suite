// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Hilo de conversación ATRAVESANDO carpetas. El ceiling que cierra: la vista de
// conversación plegaba la lista de UNA carpeta, así que un hilo abierto desde
// Recibidos no enseñaba la copia en Enviados de tu propia respuesta. El threading
// del store ya era por cuenta (resolveThread casa In-Reply-To contra el
// rfc822_msg_id de la cuenta, sin mirar el buzón); lo que faltaba era poder
// PEDIR el hilo entero.
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// seedReply inserta una respuesta a `to` en el buzón `mbox`: mismo hilo por
// In-Reply-To. Devuelve el mensaje ya con su thread_id resuelto.
func seedReply(t *testing.T, ctx context.Context, st *store.Store, acc domain.Account, mbox uuid.UUID, subject, from string, to domain.Message) domain.Message {
	t.Helper()
	m := &domain.Message{
		AccountID:  acc.ID,
		MailboxID:  mbox,
		Subject:    subject,
		FromAddr:   from,
		MessageID:  uuid.NewString() + "@sytue.eu",
		InReplyTo:  to.MessageID,
		BlobKey:    "messages/" + acc.ID.String() + "/" + uuid.NewString() + ".eml",
		SizeBytes:  100,
		Snippet:    subject,
		ReceivedAt: time.Now().UTC().Add(time.Minute), // posterior: encabeza el hilo
	}
	if err := st.InsertMessage(ctx, m); err != nil {
		t.Fatalf("InsertMessage(%q): %v", subject, err)
	}
	return *m
}

func TestListThreadMessagesAtraviesaCarpetas(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")
	inbox, err := st.MailboxByRole(ctx, acc.ID, domain.RoleInbox)
	if err != nil {
		t.Fatalf("MailboxByRole(inbox): %v", err)
	}
	sent, err := st.MailboxByRole(ctx, acc.ID, domain.RoleSent)
	if err != nil {
		t.Fatalf("MailboxByRole(sent): %v", err)
	}
	archive, err := st.MailboxByRole(ctx, acc.ID, domain.RoleArchive)
	if err != nil {
		t.Fatalf("MailboxByRole(archive): %v", err)
	}

	// Un hilo repartido por tres carpetas: lo recibido, tu respuesta en Enviados
	// y una réplica ya archivada. Más un mensaje suelto que NO es del hilo.
	recibido := seedMessage(t, ctx, st, acc, inbox.ID, "Presupuesto Q3", "cliente@ext.eu", nil)
	respuesta := seedReply(t, ctx, st, acc, sent.ID, "Re: Presupuesto Q3", "ada@sytue.eu", recibido)
	archivado := seedReply(t, ctx, st, acc, archive.ID, "Re: Presupuesto Q3", "cliente@ext.eu", respuesta)
	otro := seedMessage(t, ctx, st, acc, inbox.ID, "Nóminas", "rrhh@sytue.eu", nil)

	// Precondición del ceiling: el threading YA era por cuenta.
	if respuesta.ThreadID != recibido.ThreadID || archivado.ThreadID != recibido.ThreadID {
		t.Fatalf("el hilo no agrupó entre carpetas: %s / %s / %s",
			recibido.ThreadID, respuesta.ThreadID, archivado.ThreadID)
	}

	// Y esto es lo que la carpeta sola no puede dar: el hilo entero.
	got, err := st.ListThreadMessages(ctx, acc.ID, recibido.ThreadID, 0)
	if err != nil {
		t.Fatalf("ListThreadMessages: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("mensajes del hilo = %d, quiero 3 (%+v)", len(got), got)
	}
	boxes := map[uuid.UUID]bool{}
	for _, m := range got {
		boxes[m.MailboxID] = true
		if m.ID == otro.ID {
			t.Fatal("se coló un mensaje de otro hilo")
		}
	}
	if !boxes[inbox.ID] || !boxes[sent.ID] || !boxes[archive.ID] {
		t.Fatalf("el hilo no atraviesa las tres carpetas: %v", boxes)
	}
	// Orden reciente→antiguo, igual que ListMessages: el cliente pliega las dos
	// listas sin reordenar.
	for i := 1; i < len(got); i++ {
		if got[i-1].ReceivedAt.Before(got[i].ReceivedAt) {
			t.Fatalf("orden no es reciente→antiguo: %v", got)
		}
	}

	// Un borrado (soft-delete) sale del hilo.
	if err := st.DeleteMessage(ctx, acc.ID, archivado.ID); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	after, err := st.ListThreadMessages(ctx, acc.ID, recibido.ThreadID, 0)
	if err != nil {
		t.Fatalf("ListThreadMessages tras borrar: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("tras borrar, mensajes = %d, quiero 2", len(after))
	}
}

// El hilo es del DUEÑO: ni otra cuenta del mismo tenant ni otro tenant lo ven.
// Sin el filtro por account_id, un thread_id filtrado abriría el correo del
// vecino de organización (RLS no separa cuentas dentro del mismo tenant).
func TestListThreadMessagesEsDelDuenoYDelTenant(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(t, ctx, st, "ada@sytue.eu")
	bob := seedAccount(t, ctx, st, "bob@sytue.eu")
	inbox, err := st.MailboxByRole(ctx, ada.ID, domain.RoleInbox)
	if err != nil {
		t.Fatalf("MailboxByRole: %v", err)
	}
	m := seedMessage(t, ctx, st, ada, inbox.ID, "Confidencial", "cliente@ext.eu", nil)

	if got, err := st.ListThreadMessages(ctx, bob.ID, m.ThreadID, 0); err != nil || len(got) != 0 {
		t.Fatalf("otra cuenta del tenant ve el hilo: %d mensajes, err=%v", len(got), err)
	}
	if got, err := st.ListThreadMessages(tenantCtx("other"), ada.ID, m.ThreadID, 0); err != nil || len(got) != 0 {
		t.Fatalf("aislamiento RLS roto: %d mensajes, err=%v", len(got), err)
	}
	if got, err := st.ListThreadMessages(ctx, ada.ID, m.ThreadID, 0); err != nil || len(got) != 1 {
		t.Fatalf("el dueño no ve su hilo: %d mensajes, err=%v", len(got), err)
	}
}
