// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/notes"
)

// abrirEnEquipo abre el repositorio de Sytue Office (NewLocal) sobre el almacén en claro.
func abrirEnEquipo(t *testing.T, dir string) (*notes.Repository, func()) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "notes", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := notes.NewLocal(rec.Config)
	if err != nil {
		rec.Cerrar()
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal("preparar:", err)
	}
	return repo, func() { repo.Close(); rec.Cerrar() }
}

// Una nota de imagen con la imagen DENTRO (N-06) y un enlace a un fichero del equipo en el
// cuerpo (N-07) solo existen en la edición local: NewLocal los guarda y los vuelve a leer
// al reabrir; New —la conectada— sigue rechazando la nota de imagen y quitando el
// `file:`. La nota de imagen de una dirección web (`data.url`) también entra en local,
// con su imagen dentro (bajada a petición, con la dirección por origen) o sin ella.
func TestLaEdicionLocalGuardaImagenesIncrustadasYEnlacesAFicheros(t *testing.T) {
	ctx := t.Context()
	imagen := json.RawMessage(`{"attachments":[{"id":"1-abc","name":"plano.png","size":4,"type":"image/png","dataUrl":"data:image/png;base64,iVBORw=="}]}`)
	cuerpo := `<p>Presupuesto: <a href="file:///home/ana/Documentos/presupuesto%202026.pdf">presupuesto 2026.pdf</a> y <a href="https://ejemplo.eu">la web</a></p>`

	dir := t.TempDir()
	repo, cerrar := abrirEnEquipo(t, dir)
	libro, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "Casa"})
	if err != nil {
		t.Fatal(err)
	}
	img, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Kind: "image", Title: "plano", Data: imagen})
	if err != nil {
		t.Fatal("la nota de imagen incrustada no entra en local:", err)
	}
	txt, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Kind: "text", Content: cuerpo})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(txt.Content, `href="file:///home/ana/Documentos/presupuesto%202026.pdf"`) {
		t.Fatalf("el enlace al fichero se perdió al guardar: %s", txt.Content)
	}
	if strings.Contains(txt.Content, `presupuesto%202026.pdf" target=`) {
		t.Errorf("un fichero no se abre en otra pestaña: %s", txt.Content)
	}
	// Lo demás del saneado sigue igual: un `javascript:` no pasa por ser local.
	malo, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Kind: "text", Content: `<p><a href="javascript:alert(1)">x</a><a href="file://otra-maquina/x">y</a></p>`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(malo.Content, "href") {
		t.Errorf("un esquema que no es un fichero de este equipo pasó: %s", malo.Content)
	}
	// La nota de imagen por URL: sin la imagen (aún no se ha cargado) y con ella dentro.
	web := json.RawMessage(`{"url":"https://example.com/x.png"}`)
	cargada := json.RawMessage(`{"url":"https://example.com/x.png","attachments":[{"id":"1","name":"x.png","size":1,"type":"image/png","dataUrl":"data:image/png;base64,iVBORw==","source":"https://example.com/x.png"}]}`)
	for _, d := range []json.RawMessage{web, cargada} {
		n, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Kind: "image", Data: d})
		if err != nil {
			t.Fatalf("la nota de imagen por URL no entra en local: %v", err)
		}
		if err := repo.DeleteNote(ctx, n.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.PurgeNote(ctx, n.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Kind: "image", Data: json.RawMessage(`{"url":"javascript:alert(1)"}`)}); err == nil {
		t.Error("una nota de imagen con una dirección que no es web entró")
	}
	// Borrarla del todo deja su lápida (sin `data`), que también es de este almacén.
	//
	// VISTO EN ROJO (2026-11-25, en la prueba visual): pidiendo la imagen dentro, la
	// lápida de una nota de imagen purgada no pasaba y «Eliminar para siempre» fallaba.
	otra, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Kind: "image", Title: "otra", Data: imagen})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteNote(ctx, otra.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.PurgeNote(ctx, otra.ID); err != nil {
		t.Fatal("purgar una nota de imagen:", err)
	}
	cerrar()

	repo, cerrar = abrirEnEquipo(t, dir)
	ns, err := repo.ListNotes(ctx, libro.ID)
	if err != nil || len(ns) != 3 {
		t.Fatal("al reabrir:", ns, err)
	}
	if papelera, err := repo.ListTrash(ctx, 50); err != nil || len(papelera) != 0 {
		t.Fatal("la purgada sigue en la papelera:", papelera, err)
	}
	for _, n := range ns {
		switch n.ID {
		case img.ID:
			if n.Kind != "image" || !strings.Contains(string(n.Data), "data:image/png;base64,iVBORw==") {
				t.Errorf("la imagen al reabrir: %+v", n)
			}
		case txt.ID:
			if !strings.Contains(n.Content, `href="file:///home/ana/`) {
				t.Errorf("el enlace al reabrir: %s", n.Content)
			}
		}
	}
	cerrar()

	// La conectada, con New: la imagen incrustada no entra y el `file:` se cae.
	otro := t.TempDir()
	conectada, cerrarConectada := abrirLocal(t, otro)
	defer cerrarConectada()
	libro2, err := conectada.CreateNotebook(ctx, api.NotebookInput{Title: "Casa"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conectada.CreateNote(ctx, libro2.ID, api.NoteInput{Kind: "image", Data: imagen}); !errors.Is(err, notes.ErrAttachmentPending) {
		t.Errorf("New aceptó una nota de imagen: %v", err)
	}
	n, err := conectada.CreateNote(ctx, libro2.ID, api.NoteInput{Kind: "text", Content: cuerpo})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(n.Content, "file:") || !strings.Contains(n.Content, `href="https://ejemplo.eu"`) {
		t.Errorf("New cambió de saneado: %s", n.Content)
	}
}
