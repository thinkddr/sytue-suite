// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"encoding/json"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// Una captura sin conexión se reintenta con el mismo `client_id`: por muchas veces que
// llegue, y aunque lleguen a la vez, es UNA nota. Y la marca no cruza personas.
func TestCapturaConClientIDEsIdempotente(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Diario", domain.VisOrganization))
	if err != nil {
		t.Fatal(err)
	}
	in := domain.NoteInput{Kind: domain.KindText, Content: "<p>Llamar a Marta</p>", Data: json.RawMessage(`{"client_id":"cap-1"}`)}
	if err := in.Normalize(false); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 6)
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i := range ids {
		wg.Go(func() {
			n, err := st.CreateNote(ctx, nb.ID, "ada", in)
			ids[i], errs[i] = n.ID.String(), err
		})
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil {
			t.Fatalf("reintento %d: %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Fatal("el mismo client_id ha creado dos notas")
		}
	}
	// Otra persona con el mismo client_id en el mismo cuaderno de organización es otra nota.
	bob, err := st.CreateNote(ctx, nb.ID, "bob", in)
	if err != nil || bob.ID.String() == ids[0] {
		t.Fatalf("el client_id cruza personas: %v", err)
	}
	// Sin client_id, dos envíos son dos notas, como siempre.
	sin := domain.NoteInput{Kind: domain.KindText, Content: "<p>x</p>"}
	if err := sin.Normalize(false); err != nil {
		t.Fatal(err)
	}
	a, _ := st.CreateNote(ctx, nb.ID, "ada", sin)
	b, _ := st.CreateNote(ctx, nb.ID, "ada", sin)
	if a.ID == b.ID {
		t.Fatal("sin client_id no hay deduplicación")
	}
	notas, err := st.ListNotes(ctx, nb.ID, "ada")
	if err != nil || len(notas) != 4 {
		t.Fatalf("hay %d notas (%v); quiero 4", len(notas), err)
	}
}
