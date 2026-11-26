// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/notes"
)

func abrirNotas(t *testing.T, dir string) (*notes.Repository, *almacen.Recursos) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "notes", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := notes.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal("preparar:", err)
	}
	return repo, rec
}

// El repositorio de notas de la edición conectada, sin tocarlo, sobre el puerto en claro.
func TestElRepositorioDeNotasSobreviveAReabrirSinCifrar(t *testing.T) {
	dir := t.TempDir()
	repo, rec := abrirNotas(t, dir)
	ctx := t.Context()
	libro, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "Ideas"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Lista", Content: "<p>comprar pan</p>", Kind: "text"}); err != nil {
		t.Fatal(err)
	}
	repo.Close()
	rec.Cerrar()

	repo2, rec2 := abrirNotas(t, dir)
	defer rec2.Cerrar()
	defer repo2.Close()
	libros, err := repo2.ListNotebooks(ctx)
	if err != nil || len(libros) != 1 || libros[0].Title != "Ideas" {
		t.Fatal("cuadernos tras reabrir", libros, err)
	}
	ns, err := repo2.ListNotes(ctx, libro.ID)
	if err != nil || len(ns) != 1 || ns[0].Title != "Lista" {
		t.Fatal("notas tras reabrir", ns, err)
	}
	// En claro: la nota se lee en el diario del puerto (dentro de su JSON, en base64 o
	// en texto, pero sin cifrar). Se busca su contenido descodificando las líneas.
	raw, err := os.ReadFile(filepath.Join(dir, "registros.jsonl"))
	if err != nil || len(raw) == 0 {
		t.Fatal(err)
	}
	if !contieneEnClaro(t, raw, "comprar pan") {
		t.Fatal("el contenido no está en claro en el almacén")
	}
}

func TestUnaLineaCortadaNoTumbaElAlmacen(t *testing.T) {
	dir := t.TempDir()
	repo, rec := abrirNotas(t, dir)
	if _, err := repo.CreateNotebook(t.Context(), api.NotebookInput{Title: "A"}); err != nil {
		t.Fatal(err)
	}
	repo.Close()
	rec.Cerrar()
	f, err := os.OpenFile(filepath.Join(dir, "registros.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"t":"registro","ref":{"room_id":"!x`) // corte a mitad
	f.Close()
	repo2, rec2 := abrirNotas(t, dir)
	if l, err := repo2.ListNotebooks(t.Context()); err != nil || len(l) != 1 {
		t.Fatal(l, err)
	}
	// Lo que se escribe después se pega al trozo cortado (no tiene salto de línea): el
	// diario sigue abriendo, sin el trozo y con lo nuevo.
	//
	// VISTO EN ROJO (2026-11-25): leyendo la línea entera, sin buscar en ella la que se
	// le pegó (leerLinea), al volver a abrir «está dañado».
	if _, err := repo2.CreateNotebook(t.Context(), api.NotebookInput{Title: "B"}); err != nil {
		t.Fatal(err)
	}
	repo2.Close()
	rec2.Cerrar()
	repo3, rec3 := abrirNotas(t, dir)
	defer rec3.Cerrar()
	defer repo3.Close()
	if l, err := repo3.ListNotebooks(t.Context()); err != nil || len(l) != 2 {
		t.Fatal("lo escrito tras una línea cortada:", l, err)
	}
	if l, err := almacen.Leer(dir); err != nil || len(l) == 0 {
		t.Fatal("el lector, tras una línea cortada:", l, err)
	}
}

// Dos instancias sobre el mismo almacén (dos procesos: una app y el «Traer» del Hub):
// cada una ve lo que escribe la otra en cuanto el motor vuelve a leer, y la que cambia un
// cuaderno después de otra lo cambia sobre lo de esa otra, así que al reabrir abre.
//
// VISTO EN ROJO (2026-11-25): sin seguir el diario (Puerto.seguir), B no veía el cuaderno
// de A al refrescar, y tras cambiar los dos el mismo cuaderno el almacén ya no abría
// («private revisions require resolution»).
func TestDosInstanciasVenLoQueEscribeLaOtra(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	a, recA := abrirNotas(t, dir)
	b, recB := abrirNotas(t, dir)
	libro, err := a.CreateNotebook(ctx, api.NotebookInput{Title: "De A"})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.RefreshPrivateData(ctx); err != nil {
		t.Fatal(err)
	}
	if l, err := b.ListNotebooks(ctx); err != nil || len(l) != 1 || l[0].Title != "De A" {
		t.Fatal("B no ve lo de A:", l, err)
	}
	in := api.NotebookInput{Title: "A lo cambia"}
	if _, err := a.UpdateNotebook(ctx, libro.ID, in); err != nil {
		t.Fatal(err)
	}
	in.Title = "B lo cambia después"
	if _, err := b.UpdateNotebook(ctx, libro.ID, in); err != nil {
		t.Fatal(err)
	}
	a.Close()
	recA.Cerrar()
	b.Close()
	recB.Cerrar()
	c, recC := abrirNotas(t, dir)
	defer recC.Cerrar()
	defer c.Close()
	if l, err := c.ListNotebooks(ctx); err != nil || len(l) != 1 || l[0].Title != "B lo cambia después" {
		t.Fatal("al reabrir, gana el último:", l, err)
	}
}

func contieneEnClaro(t *testing.T, raw []byte, texto string) bool {
	t.Helper()
	if strings.Contains(string(raw), texto) {
		return true
	}
	// Las cargas del motor van en base64 (para no redondear números); se descodifican
	// por capas y se busca el texto: base64 no es cifrado.
	for _, linea := range strings.Split(string(raw), "\n") {
		if buscar(linea, texto, 0) {
			return true
		}
	}
	return false
}

func buscar(s, texto string, nivel int) bool {
	if strings.Contains(s, texto) {
		return true
	}
	if nivel > 4 {
		return false
	}
	var v any
	if json.Unmarshal([]byte(s), &v) != nil {
		if b, err := base64.StdEncoding.DecodeString(s); err == nil {
			return buscar(string(b), texto, nivel+1)
		}
		return false
	}
	return recorrer(v, texto, nivel)
}

func recorrer(v any, texto string, nivel int) bool {
	switch x := v.(type) {
	case string:
		return buscar(x, texto, nivel+1)
	case map[string]any:
		for _, y := range x {
			if recorrer(y, texto, nivel) {
				return true
			}
		}
	case []any:
		for _, y := range x {
			if recorrer(y, texto, nivel) {
				return true
			}
		}
	}
	return false
}
