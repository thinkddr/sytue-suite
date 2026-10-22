// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de los BORRADORES EN SERVIDOR (paridad §4) contra PostgreSQL
// real (pgtest):
//   - upsert por ámbito: reescribir el borrador de un canal PISA la fila (el índice
//     único es por expresión sobre COALESCE(parent_id) porque dos NULL no colisionan);
//   - el borrador de un HILO es otro ámbito y convive con el del canal;
//   - vaciarlo lo borra y no reaparece;
//   - la verja: no se puede escribir un borrador en un canal privado ajeno;
//   - aislamiento por tenant (RLS ENABLE+FORCE en mig. 0026).
//
// Detrás del build-tag `integration`. Sigue el patrón de ola16/ola18.
package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestDraftsUpsertByScope(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	root, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "raíz",
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	if _, err := st.PutDraft(ctx, "alice", ch.ID, nil, "primera versión"); err != nil {
		t.Fatalf("PutDraft canal: %v", err)
	}
	// Reescribir el MISMO ámbito pisa la fila: si el índice único no tuviera el
	// COALESCE, aquí habría dos borradores de canal y la vista mostraría el viejo.
	if _, err := st.PutDraft(ctx, "alice", ch.ID, nil, "segunda versión"); err != nil {
		t.Fatalf("PutDraft canal (upsert): %v", err)
	}
	// El borrador del HILO es otro ámbito: convive con el del canal.
	if _, err := st.PutDraft(ctx, "alice", ch.ID, &root.ID, "borrador del hilo"); err != nil {
		t.Fatalf("PutDraft hilo: %v", err)
	}

	drafts, err := st.ListDrafts(ctx, "alice")
	if err != nil {
		t.Fatalf("ListDrafts: %v", err)
	}
	if len(drafts) != 2 {
		t.Fatalf("esperaba 2 borradores (canal + hilo), got %d: %+v", len(drafts), drafts)
	}
	var canal, hilo *domain.Draft
	for i := range drafts {
		if drafts[i].ParentID == nil {
			canal = &drafts[i]
		} else {
			hilo = &drafts[i]
		}
	}
	if canal == nil || canal.Body != "segunda versión" {
		t.Fatalf("el borrador del canal debe ser el ÚLTIMO escrito: %+v", canal)
	}
	if hilo == nil || *hilo.ParentID != root.ID || hilo.Body != "borrador del hilo" {
		t.Fatalf("borrador de hilo inesperado: %+v", hilo)
	}

	// Vaciar borra solo ese ámbito.
	if err := st.DeleteDraft(ctx, "alice", ch.ID, nil); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}
	drafts, err = st.ListDrafts(ctx, "alice")
	if err != nil {
		t.Fatalf("ListDrafts tras borrar: %v", err)
	}
	if len(drafts) != 1 || drafts[0].ParentID == nil {
		t.Fatalf("solo debe quedar el borrador del hilo: %+v", drafts)
	}
	// Idempotente: volver a borrar lo que ya no está no es un error.
	if err := st.DeleteDraft(ctx, "alice", ch.ID, nil); err != nil {
		t.Fatalf("DeleteDraft repetido: %v", err)
	}
	// Y el borrador de otro usuario no existe aunque el canal sea el mismo.
	if otros, err := st.ListDrafts(ctx, "bob"); err != nil || len(otros) != 0 {
		t.Fatalf("los borradores son per-usuario: %+v (err=%v)", otros, err)
	}
}

// La verja: un borrador no es un almacén libre indexado por un canal ajeno. Sin
// membresía en un canal privado, escribirlo se deniega igual que leerlo.
func TestDraftRequiresChannelAccess(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	_, priv := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPrivate)
	if _, err := st.PutDraft(ctx, "mallory", priv.ID, nil, "texto"); err == nil {
		t.Fatal("escribir un borrador en un canal privado ajeno debe denegarse")
	} else if !errors.Is(err, domain.ErrForbidden) && !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error inesperado: %v", err)
	}
}

// RLS: el borrador de una organización no existe para otra.
func TestDraftsTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctxA, ctxB := tenantCtx("acme"), tenantCtx("otra")

	_, ch := seedTeamChannel(t, ctxA, st, "alice", domain.ChannelPublic)
	if _, err := st.PutDraft(ctxA, "alice", ch.ID, nil, "secreto de acme"); err != nil {
		t.Fatalf("PutDraft: %v", err)
	}
	if drafts, err := st.ListDrafts(ctxB, "alice"); err != nil || len(drafts) != 0 {
		t.Fatalf("otro tenant no debe ver borradores: %+v (err=%v)", drafts, err)
	}
	// Ni siquiera puede escribir en ese canal: para él no existe.
	if _, err := st.PutDraft(ctxB, "alice", ch.ID, nil, "intruso"); err == nil {
		t.Fatal("escribir en un canal de otro tenant debe fallar")
	}
}
