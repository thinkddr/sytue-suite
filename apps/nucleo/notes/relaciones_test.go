// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// En el almacén en claro de Sytue Office el motor recuerda lo comprobado y Notepads
// guarda las relaciones ya leídas de cada nota (relacionesLeidas): guardar no vuelve a
// descodificar las demás, y lo que la comprobación de grupos rechazaba lo sigue
// rechazando, con las relaciones guardadas y después de cambiarlas.
func TestLasRelacionesGuardadasRechazanLoMismo(t *testing.T) {
	ctx := t.Context()
	rec, err := almacen.Abrir(context.Background(), t.TempDir(), "notes", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Cerrar()
	r, err := NewLocal(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := almacen.Preparar(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.engine.VerifiedContentGeneration(); !ok {
		t.Fatal("el almacén local no pide recordar lo comprobado")
	}
	a, err := r.CreateNotebook(ctx, api.NotebookInput{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.CreateNotebook(ctx, api.NotebookInput{Title: "B"})
	if err != nil {
		t.Fatal(err)
	}
	padre, err := r.CreateNote(ctx, a.ID, api.NoteInput{Kind: "todo", Title: "Padre"})
	if err != nil {
		t.Fatal(err)
	}
	hija, err := r.CreateNote(ctx, a.ID, api.NoteInput{Kind: "todo", Title: "Hija", GroupID: &padre.ID})
	if err != nil {
		t.Fatal(err)
	}
	otra, err := r.CreateNote(ctx, a.ID, api.NoteInput{Kind: "text", Title: "Otra", Content: "<p>uno</p>"})
	if err != nil {
		t.Fatal(err)
	}
	// Guardados válidos: las relaciones quedan guardadas.
	for i, texto := range []string{"<p>dos</p>", "<p>tres</p>"} {
		in := otra.Input()
		in.Content = texto
		if otra, err = r.UpdateNote(ctx, otra.ID, a.ID, in); err != nil {
			t.Fatal(i, err)
		}
	}
	r.relaciones.mu.Lock()
	guardadas := len(r.relaciones.filas)
	r.relaciones.mu.Unlock()
	if guardadas == 0 {
		t.Fatal("no se guardó ninguna relación")
	}
	rechaza := func(paso, id, cuaderno string, in api.NoteInput) {
		t.Helper()
		if _, err := r.UpdateNote(ctx, id, cuaderno, in); !errors.Is(err, ErrSource) {
			t.Fatalf("%s: se aceptó (%v)", paso, err)
		}
	}
	// Mover el padre a otro cuaderno dejaría a la hija colgando de otro cuaderno.
	rechaza("mover el padre de un grupo", padre.ID, b.ID, padre.Input())
	// Una nota no es su propio grupo, ni de uno que no existe.
	en := hija.Input()
	en.GroupID = &hija.ID
	rechaza("grupo de sí misma", hija.ID, a.ID, en)
	falta := "no-existe"
	en.GroupID = &falta
	rechaza("grupo que no existe", hija.ID, a.ID, en)
	// Sacar a la hija del grupo y luego mover al padre sí vale, y con las relaciones ya
	// cambiadas el ciclo sigue sin valer.
	en = hija.Input()
	en.GroupID = nil
	if hija, err = r.UpdateNote(ctx, hija.ID, a.ID, en); err != nil {
		t.Fatal(err)
	}
	if _, err := r.UpdateNote(ctx, padre.ID, b.ID, padre.Input()); err != nil {
		t.Fatal("mover el padre sin hijas:", err)
	}
	en = hija.Input()
	en.GroupID = &padre.ID
	rechaza("grupo en otro cuaderno", hija.ID, a.ID, en)

	// Al releer el almacén (compactar) cambia la generación y lo guardado se olvida.
	antes, _ := r.engine.VerifiedContentGeneration()
	if _, err := rec.Puerto.Compactar(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.RefreshPrivateData(ctx); err != nil {
		t.Fatal(err)
	}
	despues, _ := r.engine.VerifiedContentGeneration()
	if despues == antes {
		t.Fatal("releer el almacén no cambió la generación")
	}
	in := otra.Input()
	in.Content = "<p>cuatro</p>"
	if _, err := r.UpdateNote(ctx, otra.ID, a.ID, in); err != nil {
		t.Fatal(err)
	}
	r.relaciones.mu.Lock()
	generacion := r.relaciones.generacion
	r.relaciones.mu.Unlock()
	if generacion != despues {
		t.Fatal("las relaciones guardadas siguen en la generación vieja", generacion, despues)
	}
}
