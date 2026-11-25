// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// TestQuienesVenUnCuadernoYUnaNota (ADR 0078): a quién llega la señal de cambios cuando
// alguien escribe en un cuaderno compartido —su dueña y las personas con quien lo
// compartió, lean o editen— o en una nota compartida suelta —además, quien la escribió y
// con quien la compartió—. Nadie más de la organización.
func TestQuienesVenUnCuadernoYUnaNota(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Equipo", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompartirCuaderno(ctx, nb.ID, "ada", "bob", true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompartirCuaderno(ctx, nb.ID, "ada", "carla", false); err != nil {
		t.Fatal(err)
	}
	nota, err := st.CreateNote(ctx, nb.ID, "bob", domain.NoteInput{Title: "Acta"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompartirNota(ctx, nota.ID, "bob", "dani", false); err != nil {
		t.Fatal(err)
	}
	quiero := func(qué string, got []string, err error, want ...string) {
		t.Helper()
		slices.Sort(got)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("%s: %v, %v; quiero %v", qué, got, err, want)
		}
	}
	got, err := st.QuienesVenCuaderno(ctx, nb.ID)
	quiero("el cuaderno", got, err, "ada", "bob", "carla")
	got, err = st.QuienesVenNota(ctx, nota.ID)
	quiero("la nota", got, err, "ada", "bob", "carla", "dani")
}
