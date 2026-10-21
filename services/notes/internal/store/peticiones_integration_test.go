// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// El permiso de «Pedir acceso de edición» de un cuaderno público de otra persona: Bob
// lo lee y no escribe; lo pide; solo Ada, la dueña, ve y concede, y solo a quien lo
// pidió; concedido, Bob escribe sus notas, pero no cambia el cuaderno en sí.
func TestPedirYConcederEdicionDeUnCuaderno(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Manual", domain.VisPublic))
	if err != nil {
		t.Fatal(err)
	}
	solo := func() bool {
		ls, _ := st.ListNotebooks(ctx, "bob")
		return len(ls) == 1 && ls[0].ReadOnly
	}
	if !solo() {
		t.Fatal("el cuaderno público de Ada no le sale a Bob de solo lectura")
	}
	if _, err := st.CreateNote(ctx, nb.ID, "bob", domain.NoteInput{Title: "x"}); err == nil {
		t.Fatal("Bob escribe en el cuaderno público de Ada sin permiso")
	}
	nueva, yaPuede, dueno, err := st.PedirEdicion(ctx, nb.ID, peticiones.Peticion{Sujeto: "bob", Nombre: "Bob"})
	if err != nil || !nueva || yaPuede || dueno != "ada" {
		t.Fatalf("pedir: nueva=%v yaPuede=%v dueño=%q err=%v", nueva, yaPuede, dueno, err)
	}
	if _, err := st.PeticionesEdicion(ctx, nb.ID, "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Bob ve las peticiones: %v", err)
	}
	if err := st.ResolverPeticionEdicion(ctx, nb.ID, "bob", "bob", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Bob se concede a sí mismo: %v", err)
	}
	if err := st.ResolverPeticionEdicion(ctx, nb.ID, "ada", "nadie", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("concedió a quien no lo pidió: %v", err)
	}
	if err := st.ResolverPeticionEdicion(ctx, nb.ID, "ada", "bob", true); err != nil {
		t.Fatal(err)
	}
	if solo() {
		t.Error("tras conceder, el cuaderno le sigue saliendo a Bob de solo lectura")
	}
	if _, err := st.CreateNote(ctx, nb.ID, "bob", domain.NoteInput{Title: "de Bob"}); err != nil {
		t.Errorf("tras conceder, Bob no escribe: %v", err)
	}
	if _, err := st.UpdateNotebook(ctx, nb.ID, "bob", entradaCuaderno("Mío", domain.VisOrganization)); err == nil {
		t.Error("con edición concedida, Bob cambia el cuaderno en sí (y podría abrirlo a toda la organización)")
	}
}
