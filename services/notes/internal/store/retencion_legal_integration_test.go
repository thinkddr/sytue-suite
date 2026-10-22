// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la retención legal.
//
// Todo lo de aquí falla EN SILENCIO si se rompe: un hold que no preserva no da
// ningún error, simplemente deja que el recolector destruya lo que un juzgado
// mandó conservar, y nadie se entera hasta que alguien lo pide. Por eso se
// prueba contra la base de verdad y no con dobles.
package store_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// cuadernoEnPapeleraDesde antigua el `deleted_at` de un cuaderno, como
// `enPapeleraDesde` hace con las notas.
func cuadernoEnPapeleraDesde(t *testing.T, h *pgtest.Harness, id string, hace time.Duration) {
	t.Helper()
	// En transacción de mantenimiento por lo mismo que `enPapeleraDesde`: sin
	// `app.maintenance` la RLS de `notebooks` deja el UPDATE en cero filas y sin
	// error, y el test acaba culpando a la purga de algo que no hizo.
	err := h.Pool.InMaintenanceTx(t.Context(), func(tx pgx.Tx) error {
		if err := db.MarkWriteTransaction(t.Context(), tx); err != nil {
			return err
		}
		tag, err := tx.Exec(t.Context(),
			`UPDATE notebooks SET deleted_at = now() - $2::interval WHERE id = $1`,
			id, hace.String())
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("antiguar el cuaderno afectó a %d filas, no a 1", tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("antiguar deleted_at del cuaderno: %v", err)
	}
}

// sigueViva cuenta filas por id EN UNA TRANSACCIÓN DE MANTENIMIENTO.
//
// Contar con `h.Pool` a pelo devuelve 0 SIEMPRE, exista la fila o no: `notes` y
// `notebooks` tienen RLS con FORCE y una consulta sin `app.tenant` no casa con
// ninguna política. Un test que comprueba así da por destruido lo que sigue ahí
// —y aprueba lo contrario en cuanto alguien invierta la aserción—.
func sigueViva(t *testing.T, h *pgtest.Harness, tabla string, id any) int {
	t.Helper()
	var n int
	err := h.Pool.InMaintenanceTx(t.Context(), func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(),
			`SELECT count(*) FROM `+tabla+` WHERE id = $1`, id).Scan(&n)
	})
	if err != nil {
		t.Fatalf("contar en %s: %v", tabla, err)
	}
	return n
}

// Un hold activo impide que el recolector destruya lo que cubre.
//
// Y CUBRE TAMBIÉN LO QUE YA ESTABA EN LA PAPELERA: el hold se crea DESPUÉS de
// borrar la nota, que es el orden real —primero pasa algo, después llega la
// orden de preservar—. Si la preservación sellara cada nota al crearse el hold
// en vez de evaluarse en el punto de borrado, este test pasaría igual y el caso
// de verdad se perdería.
func TestUnHoldActivoImpideLaPurga(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	// De organización: hacen falta notas de DOS personas en el mismo cuaderno, para
	// ver que el hold distingue por dueño y no se lleva por delante todo lo demás.
	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Cuaderno", domain.VisOrganization))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	preservada, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "preservada"})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	libre, err := st.CreateNote(ctx, nb.ID, "bob", domain.NoteInput{Title: "libre"})
	if err != nil {
		t.Fatalf("CreateNote(bob): %v", err)
	}
	for _, n := range []domain.Note{preservada, libre} {
		if err := st.DeleteNote(ctx, n.ID, n.OwnerSub); err != nil {
			t.Fatalf("DeleteNote: %v", err)
		}
		enPapeleraDesde(t, h, n.ID.String(), 40*24*time.Hour)
	}

	if _, err := st.CreateHold(ctx, "ada", "litigio", "CASO-1", "admin"); err != nil {
		t.Fatalf("CreateHold: %v", err)
	}

	n, err := st.PurgeExpiredTrash(t.Context(), 30*24*time.Hour)
	if err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}
	if n != 1 {
		t.Fatalf("debía purgar solo la de bob (1 fila) y purgó %d", n)
	}
	if sigueViva(t, h, "notes", preservada.ID) != 1 {
		t.Fatal("la nota bajo hold se destruyó")
	}
}

// LO QUE MÁS FÁCIL SE ROMPE, y sin ruido: la cascada de la clave ajena.
//
// `notes.notebook_id` y `notebooks.parent_id` llevan ON DELETE CASCADE, y la
// cascada la ejecuta Postgres SIN pasar por el WHERE de la purga. Si el borrado
// de cuadernos no excluye a los que contienen algo preservado, purgar un
// cuaderno caducado se lleva por delante las notas bajo hold de dentro, y el
// hold no habrá servido de nada.
//
// El cuaderno de este test está caducado y NO está bajo hold él mismo: lo único
// que lo protege es la nota que tiene dentro.
func TestElHoldSobreviveALaCascadaDelCuaderno(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	// De la ORGANIZACIÓN y de bob: así ada puede dejar dentro su nota, que es el
	// caso real —un cuaderno compartido del equipo— y el que hace interesante el
	// test: el cuaderno no lo protege su propio dueño, lo protege lo que contiene.
	nb, err := st.CreateNotebook(ctx, "bob", entradaCuaderno("Del caso", domain.VisOrganization))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	dentro, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "prueba del caso"})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if err := st.DeleteNotebook(ctx, nb.ID, "bob"); err != nil {
		t.Fatalf("DeleteNotebook: %v", err)
	}
	cuadernoEnPapeleraDesde(t, h, nb.ID.String(), 40*24*time.Hour)
	enPapeleraDesde(t, h, dentro.ID.String(), 40*24*time.Hour)

	// El hold cubre a `ada`, dueña de la nota. El cuaderno es de `bob`.
	if _, err := st.CreateHold(ctx, "ada", "litigio", "CASO-2", "admin"); err != nil {
		t.Fatalf("CreateHold: %v", err)
	}

	if _, err := st.PurgeExpiredTrash(t.Context(), 30*24*time.Hour); err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}

	if sigueViva(t, h, "notes", dentro.ID) != 1 {
		t.Fatal("la cascada del cuaderno destruyó la nota bajo hold: el hold no protege nada")
	}
	if sigueViva(t, h, "notebooks", nb.ID) != 1 {
		t.Fatal("el cuaderno se purgó pese a contener una nota preservada")
	}
}

// Liberar devuelve el contenido al ciclo normal: es la otra mitad, y sin ella un
// hold sería una condena perpetua para el disco.
func TestLiberarElHoldDevuelveAlCicloNormal(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("C", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	nota, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "x"})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if err := st.DeleteNote(ctx, nota.ID, "ada"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	enPapeleraDesde(t, h, nota.ID.String(), 40*24*time.Hour)

	hold, err := st.CreateHold(ctx, "ada", "", "CASO-3", "admin")
	if err != nil {
		t.Fatalf("CreateHold: %v", err)
	}
	if n, err := st.PurgeExpiredTrash(t.Context(), 30*24*time.Hour); err != nil || n != 0 {
		t.Fatalf("con el hold puesto no debía purgar nada: n=%d err=%v", n, err)
	}

	liberado, err := st.ReleaseHold(ctx, hold.ID, "admin")
	if err != nil {
		t.Fatalf("ReleaseHold: %v", err)
	}
	if liberado.Activo() {
		t.Fatal("el hold sigue activo después de liberarlo")
	}
	if n, err := st.PurgeExpiredTrash(t.Context(), 30*24*time.Hour); err != nil || n != 1 {
		t.Fatalf("liberado el hold debía purgar 1: n=%d err=%v", n, err)
	}
}

// El hold de una organización no preserva a otra. La purga corre SIN
// `app.tenant` fijado —es cross-tenant— así que lo único que mantiene el
// aislamiento aquí es el predicado `h.tenant = <tenant de la fila>`. Sin él, el
// hold de una organización congelaría la papelera de todas las demás.
func TestElHoldNoCruzaOrganizaciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)

	if _, err := st.CreateHold(tenantCtx("acme"), "ada", "", "CASO-4", "admin"); err != nil {
		t.Fatalf("CreateHold(acme): %v", err)
	}

	// Misma persona, `sub` idéntico, en otra organización: si el predicado de
	// tenant faltara, el hold de acme la cubriría aquí también.
	otra := tenantCtx("otra")
	nb, err := st.CreateNotebook(otra, "ada", entradaCuaderno("C", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook(otra): %v", err)
	}
	nota, err := st.CreateNote(otra, nb.ID, "ada", domain.NoteInput{Title: "x"})
	if err != nil {
		t.Fatalf("CreateNote(otra): %v", err)
	}
	if err := st.DeleteNote(otra, nota.ID, "ada"); err != nil {
		t.Fatalf("DeleteNote(otra): %v", err)
	}
	enPapeleraDesde(t, h, nota.ID.String(), 40*24*time.Hour)

	if n, err := st.PurgeExpiredTrash(t.Context(), 30*24*time.Hour); err != nil || n != 1 {
		t.Fatalf("el hold de acme no debía proteger a otra: n=%d err=%v", n, err)
	}
}
