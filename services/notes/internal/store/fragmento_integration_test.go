// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// El fragmento de un resultado trae LO BUSCADO aunque esté al final de una nota larga, y
// sin las entidades del HTML: antes eran las 200 primeras letras, con `&nbsp;` dentro.
func TestElFragmentoTraeLoBuscado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Notas", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	larga := domain.NoteInput{
		Kind: domain.KindText, Title: "Acta",
		Content: "<p>" + strings.Repeat("relleno ", 80) + "</p><p>Café&nbsp;con el <b>proveedor</b> de berilio</p>",
	}
	if err := larga.Normalize(false); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if _, err := st.CreateNote(ctx, nb.ID, "ada", larga); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	hits, err := st.SearchNotes(ctx, "ada", "berilio", 10, false)
	if err != nil || len(hits) != 1 {
		t.Fatalf("SearchNotes = %v, %v", hits, err)
	}
	f := hits[0].Snippet
	if !strings.Contains(f, "berilio") || !strings.HasPrefix(f, "…") {
		t.Fatalf("el fragmento no rodea lo buscado: %q", f)
	}
	if strings.Contains(f, "&nbsp;") || !strings.Contains(f, "Café con el") {
		t.Fatalf("el fragmento lleva entidades o blancos de HTML: %q", f)
	}
}
