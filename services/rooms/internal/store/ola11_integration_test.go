// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 11 (categorías de canal) contra PostgreSQL real
// (pgtest): CRUD de categorías del sidebar per-usuario (mig. 0009), asignación
// canal→categoría reflejada por ListChannels vía el LEFT JOIN a la membresía (sin
// N+1), borrado que hace recaer los canales a la categoría por defecto (category_id
// NULL) en la misma transacción, y aislamiento per-usuario (la categoría de otro no
// se puede asignar/actualizar/borrar). Detrás del build-tag `integration`. Sigue el
// patrón de ola10_integration_test.go.
package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// TestCategoriesCRUDAndAssignment: crear/listar/ordenar categorías, asignar un canal a
// una (reflejado por ListChannels sin N+1), colapsar/renombrar y borrar (el canal recae
// a la categoría por defecto, category_id NULL, en la misma transacción del borrado).
func TestCategoriesCRUDAndAssignment(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	// Crear dos categorías: sort_order incremental (0,1).
	c1, err := st.CreateCategory(ctx, "alice", "Proyectos")
	if err != nil {
		t.Fatalf("CreateCategory 1: %v", err)
	}
	c2, err := st.CreateCategory(ctx, "alice", "Clientes")
	if err != nil {
		t.Fatalf("CreateCategory 2: %v", err)
	}
	if c1.SortOrder != 0 || c2.SortOrder != 1 {
		t.Fatalf("sort_order incremental esperado 0,1 got %d,%d", c1.SortOrder, c2.SortOrder)
	}

	// Listar: las dos, en orden.
	cats, err := st.ListCategories(ctx, "alice")
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(cats) != 2 || cats[0].ID != c1.ID || cats[1].ID != c2.ID {
		t.Fatalf("ListCategories inesperado: %+v", cats)
	}

	// Asignar el canal a c1 con orden 5.
	sort := 5
	got, err := st.SetChannelCategory(ctx, ch.ID, "alice", "Alice", &c1.ID, &sort)
	if err != nil {
		t.Fatalf("SetChannelCategory: %v", err)
	}
	if got.CategoryID == nil || *got.CategoryID != c1.ID || got.SortOrder != 5 {
		t.Fatalf("canal no asignado: cat=%v sort=%d", got.CategoryID, got.SortOrder)
	}

	// ListChannels lo refleja (del LEFT JOIN a la propia membresía, sin N+1).
	list, err := st.ListChannels(ctx, tm.ID, "alice", false)
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if c := findChannel(t, list, ch.ID); c.CategoryID == nil || *c.CategoryID != c1.ID || c.SortOrder != 5 {
		t.Fatalf("ListChannels no refleja la categoría: cat=%v sort=%d", c.CategoryID, c.SortOrder)
	}

	// Colapsar y renombrar c1 (los nil no tocan los otros campos).
	yes := true
	newName := "Proyectos 2026"
	upd, err := st.UpdateCategory(ctx, c1.ID, "alice", store.UpdateCategoryRecord{Collapsed: &yes, Name: &newName})
	if err != nil {
		t.Fatalf("UpdateCategory: %v", err)
	}
	if !upd.Collapsed || upd.Name != newName || upd.SortOrder != 0 {
		t.Fatalf("update no aplicado / dañó otros campos: %+v", upd)
	}

	// Borrar c1 → el canal recae a NULL (categoría por defecto) en la misma tx.
	if err := st.DeleteCategory(ctx, c1.ID, "alice"); err != nil {
		t.Fatalf("DeleteCategory: %v", err)
	}
	list, err = st.ListChannels(ctx, tm.ID, "alice", false)
	if err != nil {
		t.Fatalf("ListChannels tras borrar: %v", err)
	}
	if c := findChannel(t, list, ch.ID); c.CategoryID != nil {
		t.Fatalf("tras borrar la categoría, el canal debe recaer a NULL, got %v", c.CategoryID)
	}
	// ListCategories ya no la trae; queda solo c2.
	cats, err = st.ListCategories(ctx, "alice")
	if err != nil {
		t.Fatalf("ListCategories tras borrar: %v", err)
	}
	if len(cats) != 1 || cats[0].ID != c2.ID {
		t.Fatalf("tras borrar quedan %+v, quiero solo c2", cats)
	}
	// Borrar una categoría inexistente → ErrNotFound.
	if err := st.DeleteCategory(ctx, c1.ID, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("borrar categoría inexistente debe ser ErrNotFound, got %v", err)
	}
}

// TestCategoryIsolationPerUser: la categoría es estado per-usuario; la de OTRO usuario
// no se puede asignar (ErrNotFound), ni actualizar/borrar (ErrNotFound).
func TestCategoryIsolationPerUser(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}

	aliceCat, err := st.CreateCategory(ctx, "alice", "Privada de Alice")
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	// bob (miembro del team, con acceso al canal público) no puede asignar su vista del
	// canal a la categoría de alice: es de otro subject → ErrNotFound.
	if _, err := st.SetChannelCategory(ctx, ch.ID, "bob", "Bob", &aliceCat.ID, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("asignar a categoría ajena debe ser ErrNotFound, got %v", err)
	}
	// Y no puede actualizarla ni borrarla.
	name := "secuestrada"
	if _, err := st.UpdateCategory(ctx, aliceCat.ID, "bob", store.UpdateCategoryRecord{Name: &name}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UpdateCategory ajena debe ser ErrNotFound, got %v", err)
	}
	if err := st.DeleteCategory(ctx, aliceCat.ID, "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("DeleteCategory ajena debe ser ErrNotFound, got %v", err)
	}
	// La categoría de alice sigue intacta.
	if cats, _ := st.ListCategories(ctx, "alice"); len(cats) != 1 || cats[0].Name != "Privada de Alice" {
		t.Fatalf("la categoría de alice fue alterada: %+v", cats)
	}
}
