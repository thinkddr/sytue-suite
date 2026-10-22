// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 14 (emoji personalizados) contra PostgreSQL real
// (pgtest): alta/listado/borrado del catálogo por tenant, unicidad del nombre (PK),
// borrado por autor o admin del workspace, y aislamiento por tenant (RLS). Detrás del
// build-tag `integration`. Sigue el patrón de ola12/ola13_integration_test.go.
package store_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestCustomEmojiCRUDAndUnique(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	// Alta.
	fileID := uuid.New()
	e, err := st.CreateCustomEmoji(ctx, "party_parrot", fileID, "image/gif", "alice")
	if err != nil {
		t.Fatalf("CreateCustomEmoji: %v", err)
	}
	if e.Name != "party_parrot" || e.FileID != fileID || e.ContentType != "image/gif" || e.CreatedBy != "alice" {
		t.Fatalf("emoji inesperado: %+v", e)
	}

	// Listado.
	list, err := st.ListCustomEmoji(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "party_parrot" {
		t.Fatalf("ListCustomEmoji = %+v err=%v, quiero 1 (party_parrot)", list, err)
	}

	// Nombre único por tenant (PK) → ErrConflict.
	if _, err := st.CreateCustomEmoji(ctx, "party_parrot", uuid.New(), "image/png", "bob"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("nombre duplicado debe dar ErrConflict, got %v", err)
	}

	// Borrado por el autor.
	if err := st.DeleteCustomEmoji(ctx, "party_parrot", "alice"); err != nil {
		t.Fatalf("DeleteCustomEmoji autor: %v", err)
	}
	if list, _ := st.ListCustomEmoji(ctx); len(list) != 0 {
		t.Fatalf("tras borrar el catálogo debe quedar vacío: %+v", list)
	}
	// Borrar uno inexistente → ErrNotFound.
	if err := st.DeleteCustomEmoji(ctx, "party_parrot", "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("borrar inexistente debe dar ErrNotFound, got %v", err)
	}
}

func TestCustomEmojiDeleteAuthorOrAdmin(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	// "alice" es admin de su team y, por su token, de la organización; "bob" es solo miembro.
	tm, _ := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}

	// bob crea un emoji; carol (ajena, no admin) NO puede borrarlo.
	if _, err := st.CreateCustomEmoji(ctx, "shipit", uuid.New(), "image/png", "bob"); err != nil {
		t.Fatalf("CreateCustomEmoji bob: %v", err)
	}
	if err := st.DeleteCustomEmoji(ctx, "shipit", "carol"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("no-autor no-admin debe recibir ErrForbidden, got %v", err)
	}
	// Ser admin de un team no da para moderar lo que es de toda la organización.
	if err := st.DeleteCustomEmoji(sinRol(ctx, "alice"), "shipit", "alice"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin de un team sin rol en la organización debe recibir ErrForbidden, got %v", err)
	}
	// alice, con el rol de la organización, sí puede borrar el emoji de bob (moderación).
	if err := st.DeleteCustomEmoji(comoAdmin(ctx, "alice"), "shipit", "alice"); err != nil {
		t.Fatalf("admin del workspace debe poder borrar: %v", err)
	}
}

func TestCustomEmojiTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctxA := tenantCtx("tenant-a")
	ctxB := tenantCtx("tenant-b")

	if _, err := st.CreateCustomEmoji(ctxA, "secreto", uuid.New(), "image/png", "alice"); err != nil {
		t.Fatalf("A CreateCustomEmoji: %v", err)
	}
	// El mismo nombre en B NO colisiona (aislamiento) y B no ve el catálogo de A.
	if _, err := st.CreateCustomEmoji(ctxB, "secreto", uuid.New(), "image/png", "bob"); err != nil {
		t.Fatalf("B CreateCustomEmoji (mismo nombre, otro tenant): %v", err)
	}
	if list, _ := st.ListCustomEmoji(ctxB); len(list) != 1 {
		t.Fatalf("B debe ver solo su propio catálogo: %+v", list)
	}
}
