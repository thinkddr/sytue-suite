// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

func lineasDe(t *testing.T, dir string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "registros.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(raw), "\n")
}

func objetosDe(t *testing.T, dir string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	_ = filepath.WalkDir(filepath.Join(dir, "objetos"), func(ruta string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			out[e.Name()] = true
		}
		return nil
	})
	return out
}

// Compactar deja solo lo vivo: las versiones viejas de una nota y lo que tenía una nota
// borrada para siempre se van del diario; lo vivo se lee igual al reabrir, y el motor
// sigue escribiendo encima, en la misma sesión y tras reabrir (acepta las cabezas sin
// padres).
func TestCompactarNoPierdeNadaVivoYQuitaLoBorradoDelTodo(t *testing.T) {
	dir := t.TempDir()
	repo, rec := abrirNotas(t, dir)
	ctx := t.Context()
	libro, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "Ideas"})
	if err != nil {
		t.Fatal(err)
	}
	nota, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Lista", Content: "<p>v0</p>", Kind: "text"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"<p>primera versión vieja</p>", "<p>segunda versión vieja</p>", "<p>comprar pan</p>"} {
		if nota, err = repo.UpdateNote(ctx, nota.ID, libro.ID, api.NoteInput{Title: "Lista", Content: c, Kind: "text"}); err != nil {
			t.Fatal(err)
		}
	}
	secreta, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Contraseña del wifi", Content: "<p>texto secreto</p>", Kind: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteNote(ctx, secreta.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.PurgeNote(ctx, secreta.ID); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "registros.jsonl"))
	if !contieneEnClaro(t, raw, "texto secreto") || !contieneEnClaro(t, raw, "primera versión vieja") {
		t.Fatal("la prueba no prepara lo que dice: las versiones viejas deberían estar antes de compactar")
	}
	antes := lineasDe(t, dir)

	res, err := rec.Compactar(ctx)
	if err != nil {
		t.Fatal("compactar:", err)
	}
	if !res.Algo() || res.LineasDespues >= res.LineasAntes || res.LineasAntes != antes || res.BytesDespues >= res.BytesAntes {
		t.Fatalf("no compactó: %+v", res)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "registros.jsonl"))
	for _, viejo := range []string{"texto secreto", "Contraseña del wifi", "primera versión vieja", "segunda versión vieja"} {
		if contieneEnClaro(t, raw, viejo) {
			t.Errorf("%q sigue en el diario tras compactar", viejo)
		}
	}
	if !contieneEnClaro(t, raw, "comprar pan") {
		t.Fatal("se perdió la versión viva")
	}
	// En la misma sesión el motor sigue: lee y escribe encima de la cabeza reescrita.
	if ns, err := repo.ListNotes(ctx, libro.ID); err != nil || len(ns) != 1 || ns[0].Content != "<p>comprar pan</p>" {
		t.Fatal("tras compactar, en la misma sesión:", ns, err)
	}
	if _, err := repo.UpdateNote(ctx, nota.ID, libro.ID, api.NoteInput{Title: "Lista", Content: "<p>comprar pan y leche</p>", Kind: "text"}); err != nil {
		t.Fatal("escribir tras compactar:", err)
	}
	repo.Close()
	rec.Cerrar()

	repo2, rec2 := abrirNotas(t, dir)
	ns, err := repo2.ListNotes(ctx, libro.ID)
	if err != nil || len(ns) != 1 || ns[0].Content != "<p>comprar pan y leche</p>" {
		t.Fatal("tras reabrir:", ns, err)
	}
	if libros, err := repo2.ListNotebooks(ctx); err != nil || len(libros) != 1 || libros[0].Title != "Ideas" {
		t.Fatal("cuadernos tras reabrir:", libros, err)
	}
	if papelera, err := repo2.ListTrash(ctx, 50); err != nil || len(papelera) != 0 {
		t.Fatal("papelera tras reabrir:", papelera, err)
	}
	// Una segunda vez solo quita la versión nueva que dejó la edición de después.
	if _, err := rec2.Compactar(ctx); err != nil {
		t.Fatal(err)
	}
	res, err = rec2.Compactar(ctx)
	if err != nil || res.Algo() {
		t.Fatalf("una compactación sin nada que hacer cambió algo: %+v %v", res, err)
	}
	repo2.Close()
	rec2.Cerrar()
	repo3, rec3 := abrirNotas(t, dir)
	defer rec3.Cerrar()
	defer repo3.Close()
	if ns, err := repo3.ListNotes(ctx, libro.ID); err != nil || len(ns) != 1 {
		t.Fatal("tras la segunda compactación:", ns, err)
	}
}

// Una nota grande (más de 16 KiB) guarda su contenido como objeto: al cambiarla, el
// objeto de la versión vieja queda huérfano y compactar lo quita; el de la viva se queda.
// También se van los restos de una subida cortada.
func TestCompactarLiberaLosObjetosHuerfanos(t *testing.T) {
	dir := t.TempDir()
	repo, rec := abrirNotas(t, dir)
	ctx := t.Context()
	libro, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "Grandes"})
	if err != nil {
		t.Fatal(err)
	}
	grande := func(c string) string { return "<p>" + strings.Repeat(c, 40<<10) + "</p>" }
	nota, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Larga", Content: grande("a"), Kind: "text"})
	if err != nil {
		t.Fatal(err)
	}
	primeros := objetosDe(t, dir)
	if len(primeros) == 0 {
		t.Fatal("la nota grande no dejó objeto")
	}
	if _, err := repo.UpdateNote(ctx, nota.ID, libro.ID, api.NoteInput{Title: "Larga", Content: grande("b"), Kind: "text"}); err != nil {
		t.Fatal(err)
	}
	resto := filepath.Join(dir, "objetos", ".subida-cortada")
	if err := os.WriteFile(resto, []byte("a medias"), 0o600); err != nil {
		t.Fatal(err)
	}
	antes := objetosDe(t, dir)
	res, err := rec.Compactar(ctx)
	if err != nil {
		t.Fatal(err)
	}
	despues := objetosDe(t, dir)
	if res.Objetos < 2 || res.Liberados <= 0 || len(despues) != len(antes)-res.Objetos {
		t.Fatalf("no liberó: %+v, antes %d, después %d", res, len(antes), len(despues))
	}
	for o := range primeros {
		if despues[o] {
			t.Error("sigue el objeto de la versión vieja", o)
		}
	}
	if _, err := os.Stat(resto); !errors.Is(err, os.ErrNotExist) {
		t.Error("sigue el resto de la subida cortada")
	}
	repo.Close()
	rec.Cerrar()
	repo2, rec2 := abrirNotas(t, dir)
	defer rec2.Cerrar()
	defer repo2.Close()
	ns, err := repo2.ListNotes(ctx, libro.ID)
	if err != nil || len(ns) != 1 || ns[0].Content != grande("b") {
		t.Fatal("la versión viva no se lee tras compactar y reabrir", err)
	}
}

// Un corte a mitad de compactar no estropea nada: un temporal abandonado se tira al
// abrir; y si el corte llega tras renombrar el diario y antes de barrer, quedan
// huérfanos que barre la compactación siguiente.
func TestCompactarSobreviveAUnCorteAMedias(t *testing.T) {
	dir := t.TempDir()
	repo, rec := abrirNotas(t, dir)
	ctx := t.Context()
	libro, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	nota, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Title: "N", Content: "<p>" + strings.Repeat("x", 20<<10) + "</p>", Kind: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateNote(ctx, nota.ID, libro.ID, api.NoteInput{Title: "N", Content: "<p>corta</p>", Kind: "text"}); err != nil {
		t.Fatal(err)
	}
	repo.Close()
	rec.Cerrar()

	// Un temporal de una compactación que se cortó: medio diario sin terminar.
	temporal := filepath.Join(dir, ".compactando-123")
	if err := os.WriteFile(temporal, []byte(`{"t":"registro","ref":{"room_id":"!x`), 0o600); err != nil {
		t.Fatal(err)
	}
	repo, rec = abrirNotas(t, dir)
	if _, err := os.Stat(temporal); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("el temporal abandonado sigue")
	}
	if ns, err := repo.ListNotes(ctx, libro.ID); err != nil || len(ns) != 1 || ns[0].Content != "<p>corta</p>" {
		t.Fatal(ns, err)
	}

	// Corte después de renombrar: el diario ya es el nuevo y el objeto viejo sigue.
	corte := errors.New("corte de luz")
	almacen.AntesDeBarrer(t, func() error { return corte })
	if _, err := rec.Compactar(ctx); !errors.Is(err, corte) {
		t.Fatal("el corte no llegó:", err)
	}
	if len(objetosDe(t, dir)) == 0 {
		t.Fatal("la prueba esperaba el objeto huérfano tras el corte")
	}
	repo.Close()
	rec.Cerrar()
	almacen.AntesDeBarrer(t, nil)
	repo, rec = abrirNotas(t, dir)
	defer rec.Cerrar()
	defer repo.Close()
	if ns, err := repo.ListNotes(ctx, libro.ID); err != nil || len(ns) != 1 || ns[0].Content != "<p>corta</p>" {
		t.Fatal("tras el corte:", ns, err)
	}
	res, err := rec.Compactar(ctx)
	if err != nil || res.Objetos != 1 || len(objetosDe(t, dir)) != 0 {
		t.Fatalf("la compactación siguiente no barrió: %+v %v", res, err)
	}
}

// Con otra instancia abierta (otro proceso con su cerrojo compartido) no se compacta:
// podría estar añadiendo al diario. En cuanto se cierra, sí. Y con trabajo a medias en
// el diario transitorio del motor, tampoco.
func TestCompactarRespetaOtraInstanciaYElTrabajoAMedias(t *testing.T) {
	dir := t.TempDir()
	repo, rec := abrirNotas(t, dir)
	ctx := t.Context()
	libro, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateNotebook(ctx, libro.ID, api.NotebookInput{Title: "B"}); err != nil {
		t.Fatal(err)
	}
	otro, err := almacen.AbrirPuerto(dir, rec.Config.Scope)
	if err != nil {
		t.Fatal("una segunda instancia debe poder abrir:", err)
	}
	if _, err := rec.Compactar(ctx); !errors.Is(err, almacen.ErrEnUso) {
		t.Fatal("compactó con otra instancia abierta:", err)
	}
	// El que no pudo compactar sigue escribiendo como siempre.
	if _, err := repo.UpdateNotebook(ctx, libro.ID, api.NotebookInput{Title: "C"}); err != nil {
		t.Fatal(err)
	}
	if err := otro.Cerrar(); err != nil {
		t.Fatal(err)
	}
	if err := rec.Config.Journal.Write("alguien", "algo", "a-medias", map[string]string{"x": "y"}); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Compactar(ctx); !errors.Is(err, almacen.ErrPendiente) {
		t.Fatal("compactó con trabajo a medias:", err)
	}
	if err := rec.Config.Journal.Delete("alguien", "algo", "a-medias"); err != nil {
		t.Fatal(err)
	}
	res, err := rec.Compactar(ctx)
	if err != nil || !res.Algo() {
		t.Fatalf("sin nadie más, debe compactar: %+v %v", res, err)
	}
	repo.Close()
	rec.Cerrar()
	repo2, rec2 := abrirNotas(t, dir)
	defer rec2.Cerrar()
	defer repo2.Close()
	if l, err := repo2.ListNotebooks(ctx); err != nil || len(l) != 1 || l[0].Title != "C" {
		t.Fatal(l, err)
	}
}

// CompactarAlCerrar compacta al cerrar, con el repositorio ya cerrado.
func TestCompactarAlCerrar(t *testing.T) {
	dir := t.TempDir()
	repo, rec := abrirNotas(t, dir)
	ctx := t.Context()
	libro, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	for _, titulo := range []string{"B", "C", "D"} {
		if _, err := repo.UpdateNotebook(ctx, libro.ID, api.NotebookInput{Title: titulo}); err != nil {
			t.Fatal(err)
		}
	}
	antes := lineasDe(t, dir)
	rec.CompactarAlCerrar = true
	repo.Close()
	rec.Cerrar()
	if despues := lineasDe(t, dir); despues >= antes {
		t.Fatalf("no compactó al cerrar: %d → %d", antes, despues)
	}
	repo2, rec2 := abrirNotas(t, dir)
	defer rec2.Cerrar()
	defer repo2.Close()
	if l, err := repo2.ListNotebooks(ctx); err != nil || len(l) != 1 || l[0].Title != "D" {
		t.Fatal(l, err)
	}
}
