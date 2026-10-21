// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de Sytue Notepads contra un PostgreSQL real (efímero, vía
// platform/pkg/pgtest sobre testcontainers-go). Detrás del build-tag
// `integration`: no corren en el build por defecto (sin Docker).
//
// Verifican LO QUE NO PUEDE FALLAR: que las migraciones aplican con su RLS, y que
// las cláusulas de visibilidad realmente separan a las personas dentro de una
// misma organización —RLS separa organizaciones, no personas, así que si estas
// cláusulas fallan no hay una segunda red debajo.
package store_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

func siBool(v bool) *bool { return &v }

func TestMigracionesYRLS(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	// Sin excepciones: TODAS las tablas de Notes llevan datos de organización, así
	// que ninguna tiene motivo para quedarse fuera de la RLS forzada.
	pgtest.ExigirRLSForzada(t, h, nil)
}

// Un cuaderno PERSONAL de Ada no lo ve Alan aunque compartan organización.
func TestCuadernoPersonalNoSeVeDesdeOtraCuenta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Diario", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	if _, err := st.GetNotebook(ctx, nb.ID, "alan"); err == nil {
		t.Fatal("Alan pudo abrir el cuaderno personal de Ada")
	}
	vistos, err := st.ListNotebooks(ctx, "alan")
	if err != nil {
		t.Fatalf("ListNotebooks: %v", err)
	}
	for _, v := range vistos {
		if v.ID == nb.ID {
			t.Fatal("el cuaderno personal de Ada salió en el árbol de Alan")
		}
	}
}

// Una nota marcada privada dentro de un cuaderno de ORGANIZACIÓN sigue siendo de
// quien la escribió: es la excepción que hace útil el cuaderno compartido.
func TestNotaPrivadaEnCuadernoDeOrganizacion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Equipo", domain.VisOrganization))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	publica := domain.NoteInput{Kind: domain.KindText, Content: "para todos"}
	if err := publica.Normalize(false); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if _, err := st.CreateNote(ctx, nb.ID, "ada", publica); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	privada := domain.NoteInput{Kind: domain.KindText, Content: "solo yo", Visibility: domain.VisPrivate}
	if err := privada.Normalize(false); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if _, err := st.CreateNote(ctx, nb.ID, "ada", privada); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	deAlan, err := st.ListNotes(ctx, nb.ID, "alan")
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(deAlan) != 1 {
		t.Fatalf("Alan vio %d notas, esperaba 1 (la privada de Ada no debe salir)", len(deAlan))
	}
	deAda, err := st.ListNotes(ctx, nb.ID, "ada")
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(deAda) != 2 {
		t.Fatalf("Ada vio %d de sus 2 notas", len(deAda))
	}
}

// Lo que se marca fuera del índice NO aparece en la búsqueda, ni siquiera para su
// dueño: es la promesa que hace la casilla, y aquí es donde se cumple o no.
func TestNotaNoIndexableNoSaleEnBusqueda(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Notas", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	dentro := domain.NoteInput{Kind: domain.KindText, Title: "berilio", Content: "indexada"}
	_ = dentro.Normalize(false)
	if _, err := st.CreateNote(ctx, nb.ID, "ada", dentro); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	fuera := domain.NoteInput{Kind: domain.KindText, Title: "berilio", Content: "oculta", Indexable: siBool(false)}
	_ = fuera.Normalize(false)
	if _, err := st.CreateNote(ctx, nb.ID, "ada", fuera); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	hits, err := st.SearchNotes(ctx, "ada", "berilio", 10, false)
	if err != nil {
		t.Fatalf("SearchNotes: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("la búsqueda devolvió %d resultados, esperaba 1 (la no indexable debe quedar fuera)", len(hits))
	}
}

// La consulta inversa —de un evento a sus notas— es lo que hace bidireccional el
// enlace con Calendar sin duplicar el dato.
func TestNotasDeUnEventoDeCalendar(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Reuniones", domain.VisOrganization))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	const evento = "11111111-1111-1111-1111-111111111111"
	in := domain.NoteInput{
		Kind:    domain.KindEvent,
		Title:   "Acta del comité",
		Content: "acuerdos",
		Links: []domain.NoteLink{{
			TargetKind: domain.TargetCalendarEvent, TargetID: evento, Label: "Comité semanal",
		}},
	}
	if err := in.Normalize(false); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if _, err := st.CreateNote(ctx, nb.ID, "ada", in); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	notas, err := st.NotesForTarget(ctx, "alan", domain.TargetCalendarEvent, evento, 10)
	if err != nil {
		t.Fatalf("NotesForTarget: %v", err)
	}
	if len(notas) != 1 || notas[0].Title != "Acta del comité" {
		t.Fatalf("la consulta inversa devolvió %d notas", len(notas))
	}
	if len(notas[0].Links) != 1 || notas[0].Links[0].Label != "Comité semanal" {
		t.Fatalf("el enlace no llegó adjunto: %+v", notas[0].Links)
	}
}

// El enlace público se lee SIN sesión, y con RLS forzada eso significa sin
// organización fijada: la política de aislamiento no devuelve ni una fila. Esta
// prueba es la que sujeta la política acotada de la migración 0003 —sin ella, el
// enlace responde «no existe» para TODOS los tokens, también los buenos.
func TestElEnlacePublicoSeLeeSinSesion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Manual", domain.VisPublic))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	visible := domain.NoteInput{Kind: domain.KindText, Title: "Cómo empezar", Content: "<p>hola</p>"}
	_ = visible.Normalize(false)
	if _, err := st.CreateNote(ctx, nb.ID, "ada", visible); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	// Una nota marcada privada NO sale por el enlace aunque el cuaderno sea público:
	// la excepción de la nota manda sobre el contenedor.
	privada := domain.NoteInput{Kind: domain.KindText, Title: "borrador", Visibility: domain.VisPrivate}
	_ = privada.Normalize(false)
	if _, err := st.CreateNote(ctx, nb.ID, "ada", privada); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	// SIN tenant en el contexto: es exactamente la situación de quien abre el enlace.
	pub, notas, err := st.PublicNotebook(context.Background(), nb.PublicToken)
	if err != nil {
		t.Fatalf("PublicNotebook: %v", err)
	}
	if pub.Title != "Manual" {
		t.Fatalf("el cuaderno publicado llegó como %q", pub.Title)
	}
	if pub.OwnerSub != "" {
		t.Fatal("el enlace público expuso el sujeto de su dueño")
	}
	if len(notas) != 1 || notas[0].Title != "Cómo empezar" {
		t.Fatalf("el enlace público devolvió %d notas (la privada no debe salir)", len(notas))
	}
}

// Despublicar tiene que INVALIDAR el enlace. Si el token sobrevive, el enlace que
// alguien ya tiene sigue abriendo el cuaderno: la fuga clásica de «lo he quitado».
func TestDespublicarInvalidaElEnlace(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Publicado", domain.VisPublic))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	if nb.PublicToken == "" {
		t.Fatal("un cuaderno público nació sin token")
	}
	if _, err := st.GetNotebookByPublicToken(ctx, nb.PublicToken); err != nil {
		t.Fatalf("el enlace público no resolvía: %v", err)
	}

	if _, err := st.UpdateNotebook(ctx, nb.ID, "ada", entradaCuaderno("Publicado", domain.VisPersonal)); err != nil {
		t.Fatalf("UpdateNotebook: %v", err)
	}
	if _, err := st.GetNotebookByPublicToken(ctx, nb.PublicToken); err == nil {
		t.Fatal("el enlace público seguía vivo tras despublicar")
	}
}

func entradaCuaderno(titulo string, vis domain.Visibility) domain.NotebookInput {
	in := domain.NotebookInput{Title: titulo, Visibility: vis}
	if err := in.Normalize(); err != nil {
		panic(err)
	}
	return in
}
