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

// La lista de hashtags y la búsqueda por «#palabra» CUENTAN LO MISMO, y ninguna de
// las dos delata lo que quien pregunta no puede leer: la nota privada de otra
// persona en un cuaderno compartido, ni la que se sacó del índice.
func TestHashtagsCuentanLoQueLaBusquedaEnsena(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Equipo", domain.VisOrganization))
	if err != nil {
		t.Fatal(err)
	}
	crea := func(sub string, in domain.NoteInput) {
		t.Helper()
		in.Kind = domain.KindText
		if err := in.Normalize(false); err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateNote(ctx, nb.ID, sub, in); err != nil {
			t.Fatal(err)
		}
	}
	crea("ada", domain.NoteInput{Title: "Acta #clientes", Content: "<p>La <b>#reunion</b> de ayer &amp; el #plan-2026</p>"})
	// Repetido en la misma nota cuenta una vez; con tilde y mayúscula es el mismo.
	crea("ada", domain.NoteInput{Content: "<p>#Reunión, otra #Reunión</p>"})
	// La palabra sin «#», un prefijo y un ancla NO son el hashtag.
	crea("ada", domain.NoteInput{Content: `<p>reunion a secas, #reuniones y <a href="https://sytue.eu/#reunion">enlace</a></p>`})
	// Lo que Ada no puede leer o se sacó del índice no cuenta.
	crea("bob", domain.NoteInput{Content: "<p>#reunion #secreto</p>", Visibility: domain.VisPrivate})
	crea("ada", domain.NoteInput{Content: "<p>#reunion #fuera</p>", Indexable: siBool(false)})

	tags, err := st.Hashtags(ctx, "ada")
	if err != nil {
		t.Fatal(err)
	}
	// Del más usado al menos, y a igualdad por orden alfabético. La grafía es la de
	// la nota más reciente que lo lleva.
	want := []store.Hashtag{{Tag: "Reunión", Count: 2}, {Tag: "clientes", Count: 1}, {Tag: "plan-2026", Count: 1}, {Tag: "reuniones", Count: 1}}
	if !slices.Equal(tags, want) {
		t.Fatalf("Hashtags(ada) = %+v, se esperaba %+v", tags, want)
	}

	for _, c := range []struct {
		sub, q string
		n      int
	}{
		{"ada", "#reunion", 2},   // ni «reunion a secas» ni «#reuniones»
		{"ada", "#REUNIÓN", 2},   // plegado
		{"ada", "#plan-2026", 1}, // con guion
		{"ada", "#clientes", 1},  // en el título
		{"ada", "#secreto", 0},   // privada de Bob
		{"ada", "#fuera", 0},     // fuera del índice
		{"bob", "#secreto", 1},
		{"ada", "reunion", 3}, // la búsqueda de texto sigue igual
	} {
		hits, err := st.SearchNotes(ctx, c.sub, c.q, 10, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != c.n {
			t.Errorf("SearchNotes(%s, %q) = %d notas, se esperaban %d", c.sub, c.q, len(hits), c.n)
		}
	}
}
