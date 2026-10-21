// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la traza de auditoría contra un PostgreSQL real.
//
// Verifican las dos cosas que la hacen útil: que registra lo que dice registrar
// —y solo eso— y que una organización no ve la traza de otra. Un `audit_log` que
// se salta entradas no sirve para reconstruir un incidente, y uno que se filtra
// entre organizaciones es peor que no tenerlo.
package store_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// acciones devuelve solo el campo `action` de la traza, que es lo que se afirma
// en casi todos los casos: comparar entradas enteras ataría el test al formato
// del detalle, que puede crecer.
func acciones(ctx context.Context, t *testing.T, st *store.Store) []string {
	t.Helper()
	entradas, err := st.ListAudit(ctx, store.FiltroAudit{Limit: 100})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	out := make([]string, 0, len(entradas))
	for _, e := range entradas {
		out = append(out, e.Action)
	}
	return out
}

// Publicar un cuaderno deja constancia: es la única acción del servicio que saca
// contenido fuera de la organización.
func TestPublicarYDespublicarDejanTraza(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Recetas", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	// Crear no audita: no cambia quién puede verlo.
	if got := acciones(ctx, t, st); len(got) != 0 {
		t.Fatalf("crear un cuaderno no debe auditarse, y dejó %v", got)
	}

	if _, err := st.UpdateNotebook(ctx, nb.ID, "ada", entradaCuaderno("Recetas", domain.VisPublic)); err != nil {
		t.Fatalf("publicar: %v", err)
	}
	if got := acciones(ctx, t, st); len(got) != 1 || got[0] != domain.AuditNotebookPublish {
		t.Fatalf("publicar debía dejar %q y dejó %v", domain.AuditNotebookPublish, got)
	}

	// Renombrarlo SIN tocar la visibilidad no vuelve a auditar: la traza es de
	// exposición, no de actividad.
	if _, err := st.UpdateNotebook(ctx, nb.ID, "ada", entradaCuaderno("Recetas de Ada", domain.VisPublic)); err != nil {
		t.Fatalf("renombrar: %v", err)
	}
	if got := acciones(ctx, t, st); len(got) != 1 {
		t.Fatalf("renombrar un cuaderno público no debe auditarse, y la traza quedó en %v", got)
	}

	if _, err := st.UpdateNotebook(ctx, nb.ID, "ada", entradaCuaderno("Recetas de Ada", domain.VisPersonal)); err != nil {
		t.Fatalf("despublicar: %v", err)
	}
	got := acciones(ctx, t, st)
	if len(got) != 2 || got[0] != domain.AuditNotebookUnpublish {
		t.Fatalf("despublicar debía dejar %q primero y la traza es %v", domain.AuditNotebookUnpublish, got)
	}
}

// Purgar una nota es irreversible, así que queda registrado.
func TestPurgarNotaDejaTraza(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Borradores", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	n, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "una"})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if err := st.DeleteNote(ctx, n.ID, "ada"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	// A la papelera no audita: tiene vuelta atrás.
	if got := acciones(ctx, t, st); len(got) != 0 {
		t.Fatalf("mandar a la papelera no debe auditarse, y dejó %v", got)
	}

	if err := st.PurgeNote(ctx, n.ID, "ada"); err != nil {
		t.Fatalf("PurgeNote: %v", err)
	}
	if got := acciones(ctx, t, st); len(got) != 1 || got[0] != domain.AuditNotePurge {
		t.Fatalf("purgar debía dejar %q y dejó %v", domain.AuditNotePurge, got)
	}
}

// LO QUE NO PUEDE FALLAR: la traza de una organización no se ve desde otra. Si
// esto se rompe, un administrador leería qué publica y qué destruye la
// competencia.
func TestLaTrazaNoSeVeDesdeOtraOrganizacion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	acme, otra := tenantCtx("acme"), tenantCtx("otra")

	nb, err := st.CreateNotebook(acme, "ada", entradaCuaderno("Interno", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	if _, err := st.UpdateNotebook(acme, nb.ID, "ada", entradaCuaderno("Interno", domain.VisPublic)); err != nil {
		t.Fatalf("publicar: %v", err)
	}

	if got := acciones(acme, t, st); len(got) != 1 {
		t.Fatalf("acme debía ver su propia entrada, y ve %v", got)
	}
	if got := acciones(otra, t, st); len(got) != 0 {
		t.Fatalf("otra organización NO debe ver la traza de acme, y ve %v", got)
	}
}
