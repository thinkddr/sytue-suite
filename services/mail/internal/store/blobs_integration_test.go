// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

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

// LO QUE SE PRUEBA: que el contador de referencias de un blob compartido sube
// con cada fila que lo usa —de la organización que sea— y solo devuelve la clave
// para borrar cuando la última desaparece. Un error aquí es un objeto de S3
// borrado con un buzón de otra empresa todavía apuntando a él. Desde el
// 2026-10-16 los blobs nuevos son de una sola organización (blobs.Clave), pero
// los de antes, con esta clave, siguen compartidos entre varias.
func TestBlobCompartidoEntreOrganizaciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	cuentaAcme := seedAccount(t, acme, st, "ana@acme.eu")
	cuentaGlobex := seedAccount(t, globex, st, "bob@globex.eu")
	inboxAcme, _ := st.MailboxByRole(acme, cuentaAcme.ID, domain.RoleInbox)
	inboxGlobex, _ := st.MailboxByRole(globex, cuentaGlobex.ID, domain.RoleInbox)

	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	key := "shared/mensajes/" + hash + ".eml" // la clave de antes, común a todas

	// Un mensaje que apunta a una clave compartida SIN reserva no entra: sería un
	// objeto sin contabilidad, que nadie borraría nunca.
	sinReserva := mensaje(cuentaAcme, inboxAcme.ID, key)
	if err := st.InsertMessage(acme, &sinReserva); err == nil {
		t.Fatal("un blob compartido sin reserva no debe aceptarse")
	}

	existe, listo, err := st.ReservarBlobCompartido(context.Background(), hash, key, 100)
	if err != nil || existe || listo {
		t.Fatalf("primera reserva: existe=%v listo=%v err=%v", existe, listo, err)
	}
	if err := st.MarcarBlobListo(context.Background(), hash); err != nil {
		t.Fatal(err)
	}
	existe, listo, err = st.ReservarBlobCompartido(context.Background(), hash, key, 100)
	if err != nil || !existe || !listo {
		t.Fatalf("segunda reserva: existe=%v listo=%v err=%v", existe, listo, err)
	}

	// Dos organizaciones, el mismo blob: dos referencias.
	mA := mensaje(cuentaAcme, inboxAcme.ID, key)
	mG := mensaje(cuentaGlobex, inboxGlobex.ID, key)
	if err := st.InsertMessage(acme, &mA); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertMessage(globex, &mG); err != nil {
		t.Fatal(err)
	}

	// Acme borra y purga: la clave NO sale, Globex la sigue usando.
	if err := st.DeleteMessage(acme, cuentaAcme.ID, mA.ID); err != nil {
		t.Fatal(err)
	}
	n, huerfanas, err := st.PurgeDeleted(acme, time.Now().Add(time.Hour))
	if err != nil || n != 1 || len(huerfanas) != 0 {
		t.Fatalf("purga de acme: n=%d huerfanas=%v err=%v", n, huerfanas, err)
	}
	// Globex también: ahora sí, es la última referencia.
	if err := st.DeleteMessage(globex, cuentaGlobex.ID, mG.ID); err != nil {
		t.Fatal(err)
	}
	n, huerfanas, err = st.PurgeDeleted(globex, time.Now().Add(time.Hour))
	if err != nil || n != 1 || len(huerfanas) != 1 || huerfanas[0] != key {
		t.Fatalf("purga de globex: n=%d huerfanas=%v err=%v", n, huerfanas, err)
	}
	// Y la contabilidad se ha ido con ella: una reserva nueva vuelve a ser nueva.
	existe, _, _ = st.ReservarBlobCompartido(context.Background(), hash, key, 100)
	if existe {
		t.Fatal("tras borrar la última referencia, la fila de blob_refs debe desaparecer")
	}
}

// Una clave PROPIA del tenant (anterior a la deduplicación) compartida por una
// copia entre buzones: solo sale cuando la última fila que la usa se purga.
func TestBlobPropioConCopia(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	cuenta := seedAccount(t, ctx, st, "ana@acme.eu")
	inbox, _ := st.MailboxByRole(ctx, cuenta.ID, domain.RoleInbox)
	key := "messages/" + cuenta.ID.String() + "/" + uuid.NewString() + ".eml"
	m1, m2 := mensaje(cuenta, inbox.ID, key), mensaje(cuenta, inbox.ID, key)
	if err := st.InsertMessage(ctx, &m1); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertMessage(ctx, &m2); err != nil {
		t.Fatal(err)
	}
	_ = st.DeleteMessage(ctx, cuenta.ID, m1.ID)
	if _, huerfanas, err := st.PurgeDeleted(ctx, time.Now().Add(time.Hour)); err != nil || len(huerfanas) != 0 {
		t.Fatalf("con una copia viva la clave no sale: %v %v", huerfanas, err)
	}
	_ = st.DeleteMessage(ctx, cuenta.ID, m2.ID)
	if _, huerfanas, err := st.PurgeDeleted(ctx, time.Now().Add(time.Hour)); err != nil || len(huerfanas) != 1 {
		t.Fatalf("sin filas que la usen, la clave sale: %v %v", huerfanas, err)
	}
}

func mensaje(acc domain.Account, mbox uuid.UUID, key string) domain.Message {
	return domain.Message{
		AccountID: acc.ID, MailboxID: mbox, Subject: "hola", FromAddr: "x@ext.eu",
		MessageID: uuid.NewString() + "@sytue.eu", BlobKey: key, SizeBytes: 100,
		Snippet: "hola", ReceivedAt: time.Now().UTC(),
	}
}
