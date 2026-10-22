// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// En un cuaderno de organización cualquier miembro escribe y lo rotula; moverlo,
// publicarlo, esconderlo y cifrarlo es de su dueña o de quien administra la
// organización, igual que borrarlo.
func TestAjustesDeUnCuadernoDeOrganizacionSonDeSuDuena(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	equipo, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Equipo", domain.VisOrganization))
	if err != nil {
		t.Fatal(err)
	}
	deBob, err := st.CreateNotebook(ctx, "bob", entradaCuaderno("De Bob", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}

	// Bob, un miembro cualquiera: el rótulo, sí.
	if _, err := st.UpdateNotebook(ctx, equipo.ID, "bob", entradaCuaderno("Equipo A", domain.VisOrganization)); err != nil {
		t.Fatalf("un miembro no puede rotular el cuaderno de organización: %v", err)
	}
	mover := entradaCuaderno("Equipo A", domain.VisOrganization)
	mover.ParentID = &deBob.ID
	cifrar := entradaCuaderno("Equipo A", domain.VisOrganization)
	cifrar.Encrypted = true
	cifrar.Crypto = &domain.CryptoParams{V: 1, KDF: "PBKDF2-SHA256", Salt: "c2FsdA==", Iterations: domain.MinKDFIterations, Check: "encv1:x"}
	for _, c := range []struct {
		nombre string
		in     domain.NotebookInput
	}{
		{"moverlo", mover},
		{"publicarlo", entradaCuaderno("Equipo A", domain.VisPublic)},
		{"esconderlo", entradaCuaderno("Equipo A", domain.VisPersonal)},
		{"cifrarlo", cifrar},
	} {
		if _, err := st.UpdateNotebook(ctx, equipo.ID, "bob", c.in); !errors.Is(err, domain.ErrSoloDueno) {
			t.Errorf("%s siendo un miembro cualquiera: salió %v, esperaba ErrSoloDueno", c.nombre, err)
		}
	}
	nb, err := st.GetNotebook(ctx, equipo.ID, "carla")
	if err != nil || nb.ParentID != nil || nb.Visibility != domain.VisOrganization || nb.Encrypted {
		t.Fatalf("el cuaderno cambió de sitio, de visibilidad o de cifrado: %+v (err %v)", nb, err)
	}
	// Y borrar lo suyo no se lleva lo de Ada.
	if err := st.DeleteNotebook(ctx, deBob.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetNotebook(ctx, equipo.ID, "ada"); err != nil {
		t.Fatalf("el cuaderno de Ada se fue al borrar Bob el suyo: %v", err)
	}

	// Quien administra la organización, sí… mientras sea un cuaderno de organización.
	if _, err := st.ActualizarCuaderno(ctx, equipo.ID, "carla", true, entradaCuaderno("Equipo A", domain.VisPersonal)); err != nil {
		t.Fatalf("quien administra no puede esconderlo: %v", err)
	}
	if _, err := st.ActualizarCuaderno(ctx, equipo.ID, "carla", true, entradaCuaderno("Equipo A", domain.VisOrganization)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("quien administra cambia un cuaderno personal ajeno: %v", err)
	}
	// Y su dueña, siempre.
	if _, err := st.UpdateNotebook(ctx, equipo.ID, "ada", entradaCuaderno("Equipo A", domain.VisPublic)); err != nil {
		t.Fatalf("la dueña no puede publicarlo: %v", err)
	}
}

// Un cuaderno no se cuelga de sí mismo, ni de uno que cuelga de él, ni de uno que
// quien mueve no puede escribir. Y un ciclo que ya estuviera guardado no deja sin
// acabar las consultas que bajan por el árbol.
func TestUnCuadernoNoSeCuelgaDeSuPropiaRama(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	crear := func(quien, titulo string) domain.Notebook {
		t.Helper()
		nb, err := st.CreateNotebook(ctx, quien, entradaCuaderno(titulo, domain.VisPersonal))
		if err != nil {
			t.Fatal(err)
		}
		return nb
	}
	bajo := func(titulo string, padre domain.Notebook) domain.NotebookInput {
		in := entradaCuaderno(titulo, domain.VisPersonal)
		in.ParentID = &padre.ID
		return in
	}
	a, b, ajeno := crear("ada", "A"), crear("ada", "B"), crear("bob", "De Bob")

	if _, err := st.UpdateNotebook(ctx, b.ID, "ada", bajo("B", a)); err != nil {
		t.Fatalf("colgar B de A: %v", err)
	}
	for _, c := range []struct {
		nombre string
		padre  domain.Notebook
	}{{"de sí mismo", a}, {"de su hijo", b}, {"de uno ajeno que no puede escribir", ajeno}} {
		if _, err := st.UpdateNotebook(ctx, a.ID, "ada", bajo("A", c.padre)); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("colgar A %s: salió %v, esperaba ErrInvalidInput", c.nombre, err)
		}
	}

	// Un ciclo guardado antes de esta guardia: se escribe a mano.
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := db.MarkWriteTransaction(ctx, tx); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE notebooks SET parent_id = $2 WHERE id = $1`, a.ID, b.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	conPlazo, cancelar := context.WithTimeout(ctx, 10*time.Second)
	defer cancelar()
	if err := st.ArchiveNotebook(conPlazo, a.ID, "ada", true); err != nil {
		t.Fatalf("archivar una rama con un ciclo: %v", err)
	}
	if err := st.DeleteNotebook(conPlazo, a.ID, "ada"); err != nil {
		t.Fatalf("borrar una rama con un ciclo: %v", err)
	}
}
