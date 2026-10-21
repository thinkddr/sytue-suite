// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de Sytue Mail contra un PostgreSQL real (efímero, vía
// platform/pkg/pgtest sobre testcontainers-go). Tras el build-tag `integration`
// (no corren en el build/CI por defecto). Verifican: migraciones + RLS,
// aprovisionamiento de cuenta y buzones de sistema, inserción de mensajes con
// asignación de UID, búsqueda FTS, copy/move y rename/delete de buzones, cola de
// salida (enqueue/claim/defer) y aislamiento por tenant.
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

func seedAccount(t *testing.T, ctx context.Context, st *store.Store, addr string) domain.Account {
	t.Helper()
	acc, err := st.EnsureAccount(ctx, "sub-"+addr, addr, "Test", 1<<30)
	if err != nil {
		t.Fatalf("EnsureAccount(%q): %v", addr, err)
	}
	return acc
}

func seedMessage(t *testing.T, ctx context.Context, st *store.Store, acc domain.Account, mbox uuid.UUID, subject, from string, flags []string) domain.Message {
	t.Helper()
	m := &domain.Message{
		AccountID:  acc.ID,
		MailboxID:  mbox,
		Subject:    subject,
		FromAddr:   from,
		MessageID:  uuid.NewString() + "@sytue.eu",
		BlobKey:    "messages/" + acc.ID.String() + "/" + uuid.NewString() + ".eml",
		SizeBytes:  100,
		Flags:      flags,
		Snippet:    subject + " cuerpo",
		ReceivedAt: time.Now().UTC(),
	}
	if err := st.InsertMessage(ctx, m); err != nil {
		t.Fatalf("InsertMessage(%q): %v", subject, err)
	}
	return *m
}

func TestMailMigrationsAndProvisioning(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	acc := seedAccount(t, ctx, st, "ada@sytue.eu")
	boxes, err := st.ListMailboxes(ctx, acc.ID)
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	if len(boxes) != len(domain.DefaultMailboxes) {
		t.Fatalf("buzones de sistema = %d, quiero %d", len(boxes), len(domain.DefaultMailboxes))
	}
	// Idempotencia: reprovisionar no duplica.
	if _, err := st.EnsureAccount(ctx, "sub-ada@sytue.eu", "ada@sytue.eu", "Test", 1<<30); err != nil {
		t.Fatalf("EnsureAccount idempotente: %v", err)
	}
	boxes2, _ := st.ListMailboxes(ctx, acc.ID)
	if len(boxes2) != len(boxes) {
		t.Fatal("reprovisionar duplicó buzones")
	}

	// La dirección la asigna Sytue ID y puede cambiar (dominio propio de la
	// organización, renombrado). Si la cuenta se quedara con la primera, el correo
	// a la nueva no encontraría a nadie y sus alias se rechazarían por dominio.
	movida, err := st.EnsureAccount(ctx, "sub-ada@sytue.eu", "ada@acme.sytue.eu", "Ada L.", 1<<30)
	if err != nil {
		t.Fatalf("EnsureAccount con dirección nueva: %v", err)
	}
	if movida.ID != acc.ID {
		t.Fatal("cambiar la dirección creó otra cuenta en vez de mover la suya")
	}
	if movida.Address != "ada@acme.sytue.eu" {
		t.Fatalf("Address = %q, quiero la nueva", movida.Address)
	}
	// Un token sin claims de perfil no borra lo que ya había.
	intacta, err := st.EnsureAccount(ctx, "sub-ada@sytue.eu", "", "", 1<<30)
	if err != nil {
		t.Fatalf("EnsureAccount sin claims: %v", err)
	}
	if intacta.Address != "ada@acme.sytue.eu" || intacta.DisplayName != "Ada L." {
		t.Fatalf("un token sin perfil pisó la cuenta: %q / %q", intacta.Address, intacta.DisplayName)
	}
}

func TestInsertAndSearch(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")
	inbox, err := st.MailboxByRole(ctx, acc.ID, domain.RoleInbox)
	if err != nil {
		t.Fatalf("MailboxByRole: %v", err)
	}

	m1 := seedMessage(t, ctx, st, acc, inbox.ID, "Factura mensual", "billing@proveedor.eu", nil)
	m2 := seedMessage(t, ctx, st, acc, inbox.ID, "Reunión de equipo", "jefe@sytue.eu", []string{domain.FlagSeen})

	// UID incremental por buzón.
	if m1.UID == 0 || m2.UID <= m1.UID {
		t.Fatalf("UIDs no incrementales: %d, %d", m1.UID, m2.UID)
	}

	// FTS por texto.
	res, err := st.SearchMessages(ctx, acc.ID, store.SearchFilter{Text: "factura"})
	if err != nil {
		t.Fatalf("SearchMessages(text): %v", err)
	}
	if len(res) != 1 || res[0].ID != m1.ID {
		t.Fatalf("búsqueda 'factura' => %d resultados", len(res))
	}

	// Filtro por no leídos.
	res, err = st.SearchMessages(ctx, acc.ID, store.SearchFilter{Unseen: true})
	if err != nil {
		t.Fatalf("SearchMessages(unseen): %v", err)
	}
	if len(res) != 1 || res[0].ID != m1.ID {
		t.Fatalf("búsqueda unseen => %d resultados", len(res))
	}

	// Subcadena de remitente (ILIKE '%x%'; lo sirve el índice trigram de 0002).
	res, err = st.SearchMessages(ctx, acc.ID, store.SearchFilter{From: "proveedor"})
	if err != nil {
		t.Fatalf("SearchMessages(from): %v", err)
	}
	if len(res) != 1 || res[0].ID != m1.ID {
		t.Fatalf("búsqueda from 'proveedor' => %d resultados", len(res))
	}

	// Paginación KEYSET del buzón: página 1 (limit=1) trae el más reciente; el
	// (received_at, id) del último visto pagina a los anteriores sin OFFSET.
	page1, err := st.ListMessages(ctx, inbox.ID, 1, time.Time{}, uuid.Nil)
	if err != nil {
		t.Fatalf("ListMessages(página 1): %v", err)
	}
	if len(page1) != 1 || page1[0].ID != m2.ID {
		t.Fatalf("página 1 => %d resultados, quiero el más reciente (m2)", len(page1))
	}
	page2, err := st.ListMessages(ctx, inbox.ID, 1, page1[0].ReceivedAt, page1[0].ID)
	if err != nil {
		t.Fatalf("ListMessages(página 2): %v", err)
	}
	if len(page2) != 1 || page2[0].ID != m1.ID {
		t.Fatalf("página 2 => %d resultados, quiero m1", len(page2))
	}
	page3, err := st.ListMessages(ctx, inbox.ID, 1, page2[0].ReceivedAt, page2[0].ID)
	if err != nil {
		t.Fatalf("ListMessages(página 3): %v", err)
	}
	if len(page3) != 0 {
		t.Fatalf("página 3 => %d resultados, quiero 0", len(page3))
	}

	// ListMessagesByUID devuelve orden ascendente por UID.
	byUID, err := st.ListMessagesByUID(ctx, inbox.ID)
	if err != nil {
		t.Fatalf("ListMessagesByUID: %v", err)
	}
	if len(byUID) != 2 || byUID[0].UID > byUID[1].UID {
		t.Fatalf("orden por UID incorrecto: %+v", byUID)
	}

	// GetMessages preserva orden y omite ausentes.
	got, err := st.GetMessages(ctx, []uuid.UUID{m2.ID, uuid.New(), m1.ID})
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(got) != 2 || got[0].ID != m2.ID || got[1].ID != m1.ID {
		t.Fatalf("GetMessages orden/omisión: %+v", got)
	}
}

func TestCopyMoveAndMailboxOps(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")
	inbox, _ := st.MailboxByRole(ctx, acc.ID, domain.RoleInbox)

	custom, err := st.CreateMailbox(ctx, acc.ID, nil, "Proyectos")
	if err != nil {
		t.Fatalf("CreateMailbox: %v", err)
	}
	m := seedMessage(t, ctx, st, acc, inbox.ID, "hola", "x@y.eu", nil)

	// COPY duplica a otra carpeta con UID nuevo.
	newUID, err := st.CopyMessage(ctx, m.ID, custom.ID)
	if err != nil {
		t.Fatalf("CopyMessage: %v", err)
	}
	if newUID == 0 {
		t.Fatal("CopyMessage devolvió UID 0")
	}
	inCustom, _ := st.ListMessagesByUID(ctx, custom.ID)
	if len(inCustom) != 1 {
		t.Fatalf("COPY: carpeta destino tiene %d mensajes", len(inCustom))
	}

	// MOVE traslada el original.
	if err := st.MoveMessage(ctx, acc.ID, m.ID, custom.ID); err != nil {
		t.Fatalf("MoveMessage: %v", err)
	}
	inInbox, _ := st.ListMessagesByUID(ctx, inbox.ID)
	if len(inInbox) != 0 {
		t.Fatalf("MOVE: el Inbox aún tiene %d mensajes", len(inInbox))
	}

	// RENAME de buzón de usuario permitido; de sistema prohibido.
	if err := st.RenameMailbox(ctx, acc.ID, custom.ID, "Trabajo"); err != nil {
		t.Fatalf("RenameMailbox: %v", err)
	}
	if err := st.RenameMailbox(ctx, acc.ID, inbox.ID, "NoInbox"); !errors.Is(err, domain.ErrSpecialMailbox) {
		t.Fatalf("RenameMailbox(inbox) = %v, quiero ErrSpecialMailbox", err)
	}

	// DELETE de buzón no vacío falla; vacío ok.
	if err := st.DeleteMailbox(ctx, acc.ID, custom.ID); !errors.Is(err, domain.ErrMailboxNotEmpty) {
		t.Fatalf("DeleteMailbox(no vacío) = %v", err)
	}
	empty, _ := st.CreateMailbox(ctx, acc.ID, nil, "Vacío")
	if err := st.DeleteMailbox(ctx, acc.ID, empty.ID); err != nil {
		t.Fatalf("DeleteMailbox(vacío): %v", err)
	}
}

func TestOutboundQueue(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")

	o := &domain.OutboundMessage{
		AccountID:  acc.ID,
		From:       "ada@sytue.eu",
		Recipients: []string{"bob@example.com"},
		BlobKey:    "messages/x.eml",
	}
	if err := st.Enqueue(ctx, o); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// ClaimDue corre sin tenant (rol de servicio) y reclama lo pendiente.
	claimed, err := st.ClaimDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != o.ID {
		t.Fatalf("ClaimDue => %d ítems", len(claimed))
	}

	// Diferir programa reintento; no rebota antes del máximo.
	bounced, err := st.MarkDeferred(context.Background(), o.ID, "4xx temporal", time.Second, 5)
	if err != nil {
		t.Fatalf("MarkDeferred: %v", err)
	}
	if bounced {
		t.Fatal("no debería rebotar en el primer intento")
	}
}

func TestFiltersCRUDAndApply(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")
	boletines, err := st.CreateMailbox(ctx, acc.ID, nil, "Boletines")
	if err != nil {
		t.Fatalf("CreateMailbox: %v", err)
	}

	// Regla válida: mover a Boletines + marcar leído lo que venga de newsletter@.
	f, err := st.CreateFilter(ctx, acc.ID, domain.Filter{
		Name: "Boletines", Enabled: true, Priority: 10,
		MatchFrom: "newsletter@", MoveTo: &boletines.ID, MarkRead: true,
	})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	if f.ID == uuid.Nil {
		t.Fatal("CreateFilter no devolvió id")
	}

	// move_to a un buzón inexistente / de otra cuenta => ErrInvalidFilter (checkMoveTo).
	ghost := uuid.New()
	if _, err := st.CreateFilter(ctx, acc.ID, domain.Filter{MatchFrom: "x@y", MoveTo: &ghost}); !errors.Is(err, domain.ErrInvalidFilter) {
		t.Fatalf("move_to fantasma = %v, quiero ErrInvalidFilter", err)
	}
	// Sin condición => ErrInvalidFilter (Validate).
	if _, err := st.CreateFilter(ctx, acc.ID, domain.Filter{MarkRead: true}); !errors.Is(err, domain.ErrInvalidFilter) {
		t.Fatalf("regla sin condición = %v, quiero ErrInvalidFilter", err)
	}

	// Una segunda regla con prioridad menor: debe listarse antes (orden de evaluación).
	if _, err := st.CreateFilter(ctx, acc.ID, domain.Filter{
		Name: "Urgente", Enabled: true, Priority: 1, MatchSubject: "urgente", MarkFlagged: true,
	}); err != nil {
		t.Fatalf("CreateFilter(2): %v", err)
	}
	all, err := st.ListFilters(ctx, acc.ID)
	if err != nil {
		t.Fatalf("ListFilters: %v", err)
	}
	if len(all) != 2 || all[0].Name != "Urgente" {
		t.Fatalf("orden por prioridad roto: %+v", all)
	}

	// Desactivar la de Boletines: desaparece de las activas.
	if err := st.UpdateFilter(ctx, acc.ID, f.ID, domain.Filter{
		Name: f.Name, Enabled: false, MatchFrom: "newsletter@", MoveTo: &boletines.ID, MarkRead: true,
	}); err != nil {
		t.Fatalf("UpdateFilter: %v", err)
	}
	active, _ := st.ListActiveFilters(ctx, acc.ID)
	if len(active) != 1 || active[0].Name != "Urgente" {
		t.Fatalf("activas tras desactivar = %+v", active)
	}

	// Aplicación pura sobre las activas: "urgente" en el asunto => \Flagged, sin mover.
	msg := domain.Message{FromAddr: "jefe@sytue.eu", Subject: "Asunto urgente"}
	if dest := domain.ApplyInbound(active, &msg); dest != nil {
		t.Fatalf("dest = %v, quiero nil (la regla activa solo marca)", dest)
	}
	if !msg.HasFlag(domain.FlagFlagged) {
		t.Error("ApplyInbound no aplicó \\Flagged")
	}

	// Borrado idempotente-consciente: primera vez ok, segunda ErrNotFound.
	if err := st.DeleteFilter(ctx, acc.ID, f.ID); err != nil {
		t.Fatalf("DeleteFilter: %v", err)
	}
	if err := st.DeleteFilter(ctx, acc.ID, f.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("segundo DeleteFilter = %v, quiero ErrNotFound", err)
	}
}

func TestFilterTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	acc := seedAccount(t, tenantCtx("acme"), st, "ada@sytue.eu")
	if _, err := st.CreateFilter(tenantCtx("acme"), acc.ID, domain.Filter{
		MatchFrom: "x@y", MarkRead: true, Enabled: true,
	}); err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	// Otro tenant no ve la regla (RLS de mail_filters).
	other, err := st.ListFilters(tenantCtx("other"), acc.ID)
	if err != nil {
		t.Fatalf("ListFilters(other): %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("aislamiento RLS de reglas roto: %d visibles", len(other))
	}
}

func TestTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)

	accA := seedAccount(t, tenantCtx("acme"), st, "ada@sytue.eu")
	// El tenant "other" no ve la cuenta de "acme".
	if _, err := st.AccountBySubject(tenantCtx("other"), "sub-ada@sytue.eu"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento RLS roto: %v", err)
	}
	// Y sí la ve el suyo.
	if _, err := st.AccountBySubject(tenantCtx("acme"), "sub-ada@sytue.eu"); err != nil {
		t.Fatalf("AccountBySubject(acme): %v", err)
	}
	_ = accA
}
