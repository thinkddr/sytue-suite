// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

func abrirLocal(t *testing.T, dir string) (*notes.Repository, func()) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "notes", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := notes.New(rec.Config)
	if err != nil {
		rec.Cerrar()
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal("preparar:", err)
	}
	return repo, func() { repo.Close(); rec.Cerrar() }
}

const enexDePrueba = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE en-export SYSTEM "http://xml.evernote.com/pub/evernote-export3.dtd">
<en-export export-date="20260101T000000Z" application="Evernote" version="10">
<note><title>Lista de la compra</title><content><![CDATA[<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE en-note SYSTEM "http://xml.evernote.com/pub/enml2.dtd"><en-note><div>pan y leche</div></en-note>]]></content></note>
<note><title>Viaje</title><content><![CDATA[<?xml version="1.0" encoding="UTF-8"?><en-note><div>billetes</div></en-note>]]></content></note>
</en-export>`

// Importar sin servicio: Markdown y Evernote entran como cuaderno nuevo, en el puerto en
// claro de Sytue Office, y siguen ahí al reabrir.
func TestImportarVolcadoSinServicio(t *testing.T) {
	dir := t.TempDir()
	repo, cerrar := abrirLocal(t, dir)
	ctx := t.Context()

	vp, err := repo.PreviewNotebook(ctx, []byte(enexDePrueba), "Evernote.enex")
	if err != nil || len(vp.Notas) != 2 {
		t.Fatal("ensayo", vp, err)
	}
	if libros, _ := repo.ListNotebooks(ctx); len(libros) != 0 {
		t.Fatal("el ensayo no debe crear nada", libros)
	}
	imp, err := repo.ImportNotebook(ctx, []byte(enexDePrueba), "Evernote.enex")
	if err != nil || imp.Importadas != 2 || imp.Descartadas != 0 {
		t.Fatal("importar enex", imp, err)
	}
	md, err := repo.ImportNotebook(ctx, []byte("# Ideas\n\nescribir **más**\n"), "Ideas.md")
	if err != nil || md.Importadas != 1 {
		t.Fatal("importar markdown", md, err)
	}
	cerrar()

	repo2, cerrar2 := abrirLocal(t, dir)
	defer cerrar2()
	libros, err := repo2.ListNotebooks(ctx)
	if err != nil || len(libros) != 2 {
		t.Fatal("cuadernos tras reabrir", libros, err)
	}
	ns, err := repo2.ListNotes(ctx, imp.Notebook.ID)
	if err != nil || len(ns) != 2 {
		t.Fatal("notas tras reabrir", ns, err)
	}
	if !strings.Contains(ns[0].Content+ns[1].Content, "pan y leche") {
		t.Fatal("el contenido no ha entrado", ns)
	}
}

func TestImportarRechazaLoIlegible(t *testing.T) {
	repo, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	if _, err := repo.ImportNotebook(t.Context(), []byte("{no es un volcado"), "x.onex"); err == nil {
		t.Fatal("un .onex roto debería fallar")
	}
	if _, err := repo.ImportNotebook(t.Context(), make([]byte, 33<<20), "x.onex"); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("el tope del volcado", err)
	}
}
