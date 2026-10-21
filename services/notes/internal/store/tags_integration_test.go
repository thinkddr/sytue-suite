// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// `SaveTag` no tenía tests, y lo que hace es delicado de una forma que no se ve
// leyéndolo: `tag_states` cae en CASCADA sobre las notas ya etiquetadas, así que
// recrear un estado en vez de actualizarlo le quita el estado a todas las notas
// que lo usaban. El código lo dice en un comentario; aquí queda fijado.
package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func TestSaveTagCreaConSusEstados(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	tag, err := st.SaveTag(ctx, "ada", nil, domain.TagInput{
		Name: "Prioridad",
		States: []domain.TagStateInput{
			{Name: "Alta", Color: "#ff0000"},
			{Name: "Baja", Color: "#00ff00"},
		},
	})
	if err != nil {
		t.Fatalf("SaveTag: %v", err)
	}
	if tag.Name != "Prioridad" {
		t.Fatalf("nombre = %q", tag.Name)
	}
	if len(tag.States) != 2 {
		t.Fatalf("estados = %d, quiero 2", len(tag.States))
	}
}

// LO QUE DE VERDAD IMPORTA: actualizar una etiqueta CONSERVA el id de los
// estados que ya existían. Si se borraran y reinsertaran, el id nuevo no
// coincidiría con el que tienen guardado las notas y todas perderían su estado
// —en silencio, porque la etiqueta seguiría existiendo con el mismo nombre—.
func TestSaveTagConservaElIdDeLosEstados(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	tag, err := st.SaveTag(ctx, "ada", nil, domain.TagInput{
		Name:   "Prioridad",
		States: []domain.TagStateInput{{Name: "Alta", Color: "#ff0000"}},
	})
	if err != nil {
		t.Fatalf("SaveTag: %v", err)
	}
	antes := tag.States[0].ID

	// Se reenvía el MISMO estado con su id y otro nombre: es lo que manda el
	// cliente al renombrar un valor desde la interfaz.
	tag, err = st.SaveTag(ctx, "ada", &tag.ID, domain.TagInput{
		Name:   "Prioridad",
		States: []domain.TagStateInput{{ID: &antes, Name: "Urgente", Color: "#ff0000"}},
	})
	if err != nil {
		t.Fatalf("SaveTag (actualizar): %v", err)
	}
	if len(tag.States) != 1 {
		t.Fatalf("estados = %d, quiero 1", len(tag.States))
	}
	if tag.States[0].ID != antes {
		t.Fatal("el id del estado cambió al renombrarlo: las notas etiquetadas lo perderían")
	}
	if tag.States[0].Name != "Urgente" {
		t.Fatalf("nombre del estado = %q, quiero «Urgente»", tag.States[0].Name)
	}
}

// El otro lado: un estado que el cliente ya NO manda desaparece.
func TestSaveTagBorraLosEstadosQueYaNoVienen(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	tag, err := st.SaveTag(ctx, "ada", nil, domain.TagInput{
		Name: "Prioridad",
		States: []domain.TagStateInput{
			{Name: "Alta", Color: "#ff0000"},
			{Name: "Baja", Color: "#00ff00"},
		},
	})
	if err != nil {
		t.Fatalf("SaveTag: %v", err)
	}
	quedarse := tag.States[0].ID

	tag, err = st.SaveTag(ctx, "ada", &tag.ID, domain.TagInput{
		Name:   "Prioridad",
		States: []domain.TagStateInput{{ID: &quedarse, Name: "Alta", Color: "#ff0000"}},
	})
	if err != nil {
		t.Fatalf("SaveTag (quitar uno): %v", err)
	}
	if len(tag.States) != 1 || tag.States[0].ID != quedarse {
		t.Fatalf("debía quedar solo el conservado, y hay %d", len(tag.States))
	}
}

// Dos etiquetas con el mismo nombre para la misma persona no pueden coexistir:
// el índice único lo impide y el store lo traduce a entrada inválida, no a un
// error de base de datos en crudo.
func TestSaveTagRechazaNombreRepetido(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.SaveTag(ctx, "ada", nil, domain.TagInput{Name: "Prioridad"}); err != nil {
		t.Fatalf("SaveTag: %v", err)
	}
	_, err := st.SaveTag(ctx, "ada", nil, domain.TagInput{Name: "Prioridad"})
	if err == nil {
		t.Fatal("se creó una segunda etiqueta con el mismo nombre")
	}
}
