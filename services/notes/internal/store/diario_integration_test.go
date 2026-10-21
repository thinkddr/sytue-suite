// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func TestDiarioConcurrenciaYAislamiento(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	var wg sync.WaitGroup
	ids := make([]uuid.UUID, 8)
	errs := make([]error, len(ids))
	for i := range ids {
		wg.Go(func() {
			nb, err := st.DailyNotebook(ctx, "ada", "2026-10-01", "Hoy", true)
			ids[i], errs[i] = nb.ID, err
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("apertura %d: %v", i, err)
		}
		if ids[i] != ids[0] {
			t.Fatal("aperturas simultáneas duplican el diario")
		}
	}
	bob, err := st.DailyNotebook(ctx, "bob", "2026-10-01", "Hoy", false)
	if err != nil {
		t.Fatal(err)
	}
	otro, err := st.DailyNotebook(tenantCtx("otra"), "ada", "2026-10-01", "Hoy", false)
	if err != nil {
		t.Fatal(err)
	}
	if bob.ID == ids[0] || otro.ID == ids[0] {
		t.Fatal("el diario cruza personas u organizaciones")
	}
	nb, err := st.UpdateNotebook(ctx, ids[0], "ada", entradaCuaderno("Mi jueves", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	again, err := st.DailyNotebook(ctx, "ada", "2026-10-01", "Hoy", true)
	if err != nil || again.Title != nb.Title || again.ID != nb.ID {
		t.Fatalf("reabrir pierde personalización: %+v %v", again, err)
	}
}

func TestDiarioTrasladaPendientesSinDuplicarNiAmpliarPrivacidad(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	old, err := st.DailyNotebook(ctx, "ada", "2026-09-30", "Ayer", false)
	if err != nil {
		t.Fatal(err)
	}
	privado := entradaCuaderno("Ayer privado", domain.VisPrivate)
	privado.Indexable = siBool(false)
	if _, err = st.UpdateNotebook(ctx, old.ID, "ada", privado); err != nil {
		t.Fatal(err)
	}
	crear := func(titulo string, hecha bool) domain.Note {
		t.Helper()
		in := domain.NoteInput{Kind: domain.KindTodo, Title: titulo, Checked: siBool(hecha)}
		if err := in.Normalize(false); err != nil {
			t.Fatal(err)
		}
		n, err := st.CreateNote(ctx, old.ID, "ada", in)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	pendiente := crear("Pendiente", false)
	hecha := crear("Hecha", true)
	compartida := crear("Compartida", false)
	if _, _, err := st.CompartirNota(ctx, compartida.ID, "ada", "bob", false); err != nil {
		t.Fatal(err)
	}
	padre := crear("Padre", false)
	hija := crear("Hija", false)
	if err := st.MoveNote(ctx, hija.ID, "ada", hija.X, hija.Y, hija.Col, hija.Position, &padre.ID); err != nil {
		t.Fatal(err)
	}
	today, err := st.DailyNotebook(ctx, "ada", "2026-10-01", "Hoy", true)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := st.DailyNotebook(ctx, "ada", "2026-10-01", "Hoy", true); err != nil {
			t.Fatal(err)
		}
	}
	ns, err := st.ListNotes(ctx, today.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	// La suelta y el padre con su hija, que viaja con él: el subárbol va entero.
	movidas := map[uuid.UUID]domain.Note{}
	for _, n := range ns {
		movidas[n.ID] = n
	}
	if len(ns) != 3 || movidas[pendiente.ID].ID == uuid.Nil || movidas[padre.ID].ID == uuid.Nil ||
		movidas[hija.ID].GroupID == nil || *movidas[hija.ID].GroupID != padre.ID {
		t.Fatalf("traslado duplicado o rompió el subárbol: %+v", ns)
	}
	ns[0] = movidas[pendiente.ID]
	if ns[0].Visibility != domain.VisPrivate || ns[0].Indexable == nil || *ns[0].Indexable {
		t.Fatalf("traslado amplía privacidad/índice: %+v", ns[0])
	}
	var data map[string]any
	if err := json.Unmarshal(ns[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["carried_from"] != "2026-09-30" {
		t.Fatal("se pierde el día de origen")
	}
	quedan, err := st.ListNotes(ctx, old.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if len(quedan) != 2 {
		t.Fatalf("no quedaron hecha (%s) y compartida: %v", hecha.ID, quedan)
	}
	// Una vez compartido el destino, no se deben traer allí notas privadas nuevas.
	nueva := crear("Otra privada", false)
	if _, _, err := st.CompartirCuaderno(ctx, today.ID, "ada", "bob", false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DailyNotebook(ctx, "ada", "2026-10-01", "Hoy", true); err != nil {
		t.Fatal(err)
	}
	ns, err = st.ListNotes(ctx, old.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range ns {
		if n.ID == nueva.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("un destino compartido recibió una tarea privada")
	}
}

func TestDiasDelDiarioCuentasNotasYEsteDia(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	nota := func(nb uuid.UUID, quien string, in domain.NoteInput) domain.Note {
		t.Helper()
		if err := in.Normalize(false); err != nil {
			t.Fatal(err)
		}
		n, err := st.CreateNote(ctx, nb, quien, in)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	dia := func(fecha string) uuid.UUID {
		t.Helper()
		nb, err := st.DailyNotebook(ctx, "ada", fecha, fecha, false)
		if err != nil {
			t.Fatal(err)
		}
		return nb.ID
	}
	lunes, martes := dia("2026-09-28"), dia("2026-09-29")
	dia("2026-09-30") // vacío: no sale
	hace1, hace2, otroMes := dia("2025-09-29"), dia("2024-09-29"), dia("2025-10-29")
	nota(lunes, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "Pendiente", Checked: siBool(false)})
	nota(lunes, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "Hecha", Checked: siBool(true)})
	nota(lunes, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "Cancelada", Cancelled: siBool(true)})
	nota(lunes, "ada", domain.NoteInput{Kind: domain.KindText, Content: "<p>Idea</p>"})
	borrada := nota(martes, "ada", domain.NoteInput{Kind: domain.KindText, Content: "<p>Borrada</p>"})
	if err := st.DeleteNote(ctx, borrada.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	nota(martes, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "Martes", Checked: siBool(false)})
	nota(hace1, "ada", domain.NoteInput{Kind: domain.KindText, Content: "<p>Hace un año</p>"})
	nota(hace2, "ada", domain.NoteInput{Kind: domain.KindText, Content: "<p>Hace dos</p>"})
	nota(otroMes, "ada", domain.NoteInput{Kind: domain.KindText, Content: "<p>Otro mes</p>"})
	// El diario de otra persona, el mismo día: no se cuenta nunca.
	bob, err := st.DailyNotebook(ctx, "bob", "2026-09-28", "Bob", false)
	if err != nil {
		t.Fatal(err)
	}
	nota(bob.ID, "bob", domain.NoteInput{Kind: domain.KindText, Content: "<p>De Bob</p>"})

	dias, err := st.DailyRange(ctx, "ada", "2026-09-28", "2026-10-04", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(dias) != 2 || dias[0].Date != "2026-09-28" || dias[0].Total != 4 || dias[0].Pending != 1 ||
		dias[1].Date != "2026-09-29" || dias[1].Total != 1 || dias[1].Pending != 1 || dias[0].Notes != nil {
		t.Fatalf("cuentas de la semana: %+v", dias)
	}
	dias, err = st.DailyRange(ctx, "ada", "2026-09-28", "2026-09-29", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(dias) != 2 || len(dias[0].Notes) != 4 || len(dias[1].Notes) != 1 || dias[1].Notes[0].Title != "Martes" {
		t.Fatalf("notas de la semana: %+v", dias)
	}
	este, err := st.ThisDay(ctx, "ada", "2026-09-29")
	if err != nil {
		t.Fatal(err)
	}
	if len(este) != 2 || este[0].Date != "2025-09-29" || este[1].Date != "2024-09-29" || len(este[0].Notes) != 1 {
		t.Fatalf("este día: %+v", este)
	}
}
