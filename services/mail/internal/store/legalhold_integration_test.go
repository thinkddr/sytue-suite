// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Integración de legal hold + eDiscovery contra PostgreSQL real (pgtest). Verifica
// la INVARIANTE no destructiva: un mensaje de un sujeto bajo hold sobrevive a
// papelera + purga; sin hold se purga normal; liberar el hold restaura el ciclo.
// Además: eDiscovery encuentra el correo preservado y filtra por fecha/consulta;
// aislamiento por org (un admin de A no ve correo/holds/traza de B); y auditoría.
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

// farFuture es un corte de purga que abarca todo lo ya soft-deleted (deleted_at
// ~ now < now+1h): purga TODO lo borrado que no esté bajo hold.
func farFuture() time.Time { return time.Now().Add(time.Hour) }

// TestLegalHoldPreservesThroughPurge: con hold el mensaje sobrevive a la purga;
// sin hold se purga. Al liberar el hold, la purga vuelve a llevárselo.
func TestLegalHoldPreservesThroughPurge(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	held := seedAccount(t, ctx, st, "held@acme.eu")
	free := seedAccount(t, ctx, st, "free@acme.eu")
	inboxHeld, _ := st.MailboxByRole(ctx, held.ID, domain.RoleInbox)
	inboxFree, _ := st.MailboxByRole(ctx, free.ID, domain.RoleInbox)
	mHeld := seedMessage(t, ctx, st, held, inboxHeld.ID, "prueba retenida", "x@ext.eu", nil)
	mFree := seedMessage(t, ctx, st, free, inboxFree.ID, "prueba libre", "x@ext.eu", nil)

	// Hold sobre el sujeto retenido.
	hold, err := st.CreateHold(ctx, domain.LegalHold{Subject: held.Subject, Case: "C1", CreatedBy: "admin-1"})
	if err != nil {
		t.Fatalf("CreateHold: %v", err)
	}

	// Borrado del usuario (papelera / borrado permanente = soft-delete) de AMBOS.
	if err := st.DeleteMessage(ctx, held.ID, mHeld.ID); err != nil {
		t.Fatalf("DeleteMessage held: %v", err)
	}
	if err := st.DeleteMessage(ctx, free.ID, mFree.ID); err != nil {
		t.Fatalf("DeleteMessage free: %v", err)
	}

	// Purga: se lleva SOLO el del sujeto libre; el retenido sobrevive.
	n, _, err := st.PurgeDeleted(ctx, farFuture())
	if err != nil {
		t.Fatalf("PurgeDeleted: %v", err)
	}
	if n != 1 {
		t.Fatalf("purgados = %d, quiero 1 (solo el no retenido)", n)
	}
	if got := searchCount(t, st, ctx, held.ID); got != 1 {
		t.Fatalf("mensaje retenido: %d supervivientes, quiero 1 (preservado)", got)
	}
	if got := searchCount(t, st, ctx, free.ID); got != 0 {
		t.Fatalf("mensaje libre: %d supervivientes, quiero 0 (purgado)", got)
	}

	// Liberar el hold restaura el ciclo normal: ahora la purga sí se lo lleva.
	if err := st.ReleaseHold(ctx, hold.ID, "admin-1"); err != nil {
		t.Fatalf("ReleaseHold: %v", err)
	}
	n, _, err = st.PurgeDeleted(ctx, farFuture())
	if err != nil {
		t.Fatalf("PurgeDeleted tras liberar: %v", err)
	}
	if n != 1 {
		t.Fatalf("tras liberar purgados = %d, quiero 1", n)
	}
	if got := searchCount(t, st, ctx, held.ID); got != 0 {
		t.Fatalf("tras liberar: %d supervivientes, quiero 0", got)
	}
}

// TestPurgeNoHoldUnchanged: sin ningún hold, la purga borra el correo soft-deleted
// como siempre (el comportamiento normal NO cambia).
func TestPurgeNoHoldUnchanged(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "u@acme.eu")
	inbox, _ := st.MailboxByRole(ctx, acc.ID, domain.RoleInbox)
	m := seedMessage(t, ctx, st, acc, inbox.ID, "efímero", "x@ext.eu", nil)
	if err := st.DeleteMessage(ctx, acc.ID, m.ID); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	n, _, err := st.PurgeDeleted(ctx, farFuture())
	if err != nil {
		t.Fatalf("PurgeDeleted: %v", err)
	}
	if n != 1 || searchCount(t, st, ctx, acc.ID) != 0 {
		t.Fatalf("sin hold la purga debe llevarse el correo (n=%d)", n)
	}
}

// TestEDiscoveryFindsPreservedAndFilters: la búsqueda encuentra el correo
// preservado (soft-deleted) y filtra por fecha y por consulta de texto.
func TestEDiscoveryFindsPreservedAndFilters(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "sue@acme.eu")
	inbox, _ := st.MailboxByRole(ctx, acc.ID, domain.RoleInbox)

	old := seedMsgAt(t, ctx, st, acc, inbox.ID, "Factura antigua", "billing@ext.eu", time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC))
	recent := seedMsgAt(t, ctx, st, acc, inbox.ID, "Factura reciente", "billing@ext.eu", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	_ = seedMsgAt(t, ctx, st, acc, inbox.ID, "Reunión equipo", "jefe@acme.eu", time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC))

	// Preservar (borrar) el reciente: bajo hold seguiría encontrándose.
	if _, err := st.CreateHold(ctx, domain.LegalHold{Subject: acc.Subject, Case: "C2", CreatedBy: "admin-1"}); err != nil {
		t.Fatalf("CreateHold: %v", err)
	}
	if err := st.DeleteMessage(ctx, acc.ID, recent.ID); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	// Consulta de texto: ambas facturas, incluida la preservada.
	hits, err := st.EDiscoverySearch(ctx, acc.ID, store.EDiscoveryFilter{Query: "factura"})
	if err != nil {
		t.Fatalf("EDiscoverySearch(texto): %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("consulta 'factura' => %d, quiero 2 (incl. preservada)", len(hits))
	}
	var sawPreserved bool
	for _, hit := range hits {
		if hit.Message.ID == recent.ID {
			if !hit.Preserved {
				t.Fatal("el correo borrado bajo hold debe venir Preserved=true")
			}
			sawPreserved = true
		}
	}
	if !sawPreserved {
		t.Fatal("la búsqueda debe incluir el correo preservado")
	}

	// Filtro por rango de fechas: solo la factura antigua (2020).
	hits, err = st.EDiscoverySearch(ctx, acc.ID, store.EDiscoveryFilter{
		From: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("EDiscoverySearch(fecha): %v", err)
	}
	if len(hits) != 1 || hits[0].Message.ID != old.ID {
		t.Fatalf("filtro de fecha 2019-2021 => %d, quiero 1 (la antigua)", len(hits))
	}
}

// TestEDiscoveryOrgIsolation: un admin de la org A no ve el correo ni resuelve la
// cuenta de la org B (RLS por tenant, deny-by-default).
func TestEDiscoveryOrgIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctxA := tenantCtx("acme")
	ctxB := tenantCtx("globex")

	accB := seedAccount(t, ctxB, st, "spy@globex.eu")
	inboxB, _ := st.MailboxByRole(ctxB, accB.ID, domain.RoleInbox)
	seedMessage(t, ctxB, st, accB, inboxB.ID, "confidencial B", "x@ext.eu", nil)

	// Admin de A busca el correo de una cuenta de B (por su id): 0 (RLS lo aísla).
	hits, err := st.EDiscoverySearch(ctxA, accB.ID, store.EDiscoveryFilter{})
	if err != nil {
		t.Fatalf("EDiscoverySearch cross-org: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("admin de A ve %d mensajes de B, quiero 0", len(hits))
	}
	// Resolver la cuenta de B por email desde A: ErrNotFound.
	if _, err := st.AccountByAddress(ctxA, "spy@globex.eu"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("AccountByAddress cross-org = %v, quiero ErrNotFound", err)
	}
}

// TestLegalHoldAndAuditIsolation: crear hold + auditar escriben en audit_log, y la
// traza está aislada por org (A no ve la de B).
func TestLegalHoldAndAuditIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctxA := tenantCtx("acme")
	ctxB := tenantCtx("globex")
	accA := seedAccount(t, ctxA, st, "a@acme.eu")

	// CreateHold audita en su propia transacción.
	if _, err := st.CreateHold(ctxA, domain.LegalHold{Subject: accA.Subject, Case: "C3", CreatedBy: "admin-A"}); err != nil {
		t.Fatalf("CreateHold: %v", err)
	}
	// Búsqueda/export auditan vía WriteAudit.
	if err := st.WriteAudit(ctxA, "admin-A", "ediscovery.search", "a@acme.eu", map[string]any{"case": "C3"}); err != nil {
		t.Fatalf("WriteAudit: %v", err)
	}

	entries, err := st.ListAudit(ctxA, 50)
	if err != nil {
		t.Fatalf("ListAudit A: %v", err)
	}
	var sawCreate, sawSearch bool
	for _, e := range entries {
		switch e.Action {
		case "legalhold.create":
			sawCreate = true
		case "ediscovery.search":
			sawSearch = true
		}
		if e.Actor != "admin-A" {
			t.Fatalf("actor inesperado en traza: %q", e.Actor)
		}
	}
	if !sawCreate || !sawSearch {
		t.Fatalf("faltan entradas de auditoría: create=%v search=%v", sawCreate, sawSearch)
	}

	// Aislamiento: la org B no ve la traza de A.
	entriesB, err := st.ListAudit(ctxB, 50)
	if err != nil {
		t.Fatalf("ListAudit B: %v", err)
	}
	if len(entriesB) != 0 {
		t.Fatalf("org B ve %d entradas de auditoría de A, quiero 0", len(entriesB))
	}
	// Y B tampoco ve el hold de A.
	holdsB, err := st.ListHolds(ctxB, true)
	if err != nil {
		t.Fatalf("ListHolds B: %v", err)
	}
	if len(holdsB) != 0 {
		t.Fatalf("org B ve %d holds de A, quiero 0", len(holdsB))
	}
}

// TestDeleteMailboxRespectsHold: borrar una carpeta cascadearía (hard-delete) a
// sus mensajes; bajo hold se niega si aún guarda correo (incl. preservado). Sin
// hold, el comportamiento normal (carpeta sin correo vivo => borrable) no cambia.
func TestDeleteMailboxRespectsHold(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "u@acme.eu")

	// Con hold: la carpeta con un mensaje soft-deleted NO se puede borrar.
	if _, err := st.CreateHold(ctx, domain.LegalHold{Subject: acc.Subject, Case: "C4", CreatedBy: "admin-1"}); err != nil {
		t.Fatalf("CreateHold: %v", err)
	}
	mb, err := st.CreateMailbox(ctx, acc.ID, nil, "Proyecto")
	if err != nil {
		t.Fatalf("CreateMailbox: %v", err)
	}
	m := seedMessage(t, ctx, st, acc, mb.ID, "material del caso", "x@ext.eu", nil)
	if err := st.DeleteMessage(ctx, acc.ID, m.ID); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if err := st.DeleteMailbox(ctx, acc.ID, mb.ID); !errors.Is(err, domain.ErrMailboxNotEmpty) {
		t.Fatalf("DeleteMailbox bajo hold = %v, quiero ErrMailboxNotEmpty", err)
	}
	if got := searchCount(t, st, ctx, acc.ID); got != 1 {
		t.Fatalf("el correo del caso debe seguir vivo: %d", got)
	}

	// Sin hold: la misma carpeta (solo correo soft-deleted) sí es borrable
	// (comportamiento normal inalterado). Se resuelve creando otra cuenta libre.
	free := seedAccount(t, ctx, st, "free@acme.eu")
	mbf, _ := st.CreateMailbox(ctx, free.ID, nil, "Temp")
	mf := seedMessage(t, ctx, st, free, mbf.ID, "borrable", "x@ext.eu", nil)
	if err := st.DeleteMessage(ctx, free.ID, mf.ID); err != nil {
		t.Fatalf("DeleteMessage free: %v", err)
	}
	if err := st.DeleteMailbox(ctx, free.ID, mbf.ID); err != nil {
		t.Fatalf("DeleteMailbox sin hold debe funcionar: %v", err)
	}
}

// --- helpers --------------------------------------------------------------

// searchCount cuenta cuántos mensajes (incl. preservados/soft-deleted) sobreviven
// para la cuenta, vía eDiscovery (que no filtra deleted_at).
func searchCount(t *testing.T, st *store.Store, ctx context.Context, accountID uuid.UUID) int {
	t.Helper()
	hits, err := st.EDiscoverySearch(ctx, accountID, store.EDiscoveryFilter{})
	if err != nil {
		t.Fatalf("EDiscoverySearch: %v", err)
	}
	return len(hits)
}

// seedMsgAt inserta un mensaje con un received_at concreto (para probar el filtro
// de fechas de eDiscovery); seedMessage siempre usa now().
func seedMsgAt(t *testing.T, ctx context.Context, st *store.Store, acc domain.Account, mbox uuid.UUID, subject, from string, when time.Time) domain.Message {
	t.Helper()
	m := &domain.Message{
		AccountID:  acc.ID,
		MailboxID:  mbox,
		Subject:    subject,
		FromAddr:   from,
		MessageID:  uuid.NewString() + "@sytue.eu",
		BlobKey:    "messages/" + acc.ID.String() + "/" + uuid.NewString() + ".eml",
		SizeBytes:  100,
		Snippet:    subject + " cuerpo",
		ReceivedAt: when,
	}
	if err := st.InsertMessage(ctx, m); err != nil {
		t.Fatalf("InsertMessage(%q): %v", subject, err)
	}
	return *m
}
