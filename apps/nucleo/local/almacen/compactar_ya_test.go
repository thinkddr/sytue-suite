// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen_test

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// CompactarYa compacta EN EL ACTO tras «borrar del todo», en segundo plano: lo purgado se
// va del diario sin cerrar la app, muchas peticiones seguidas (vaciar la papelera, desde
// varias rutinas) se juntan en una o dos compactaciones, el repositorio refrescado sigue
// leyendo y escribiendo, y PararCompactar olvida lo que no ha empezado. Con otra instancia
// abierta no compacta ni falla.
func TestCompactarYaTrasBorrarDelTodo(t *testing.T) {
	antes := almacen.EsperaCompactarYa
	almacen.EsperaCompactarYa = 50 * time.Millisecond
	t.Cleanup(func() { almacen.EsperaCompactarYa = antes })

	dir := t.TempDir()
	repo, rec := abrirNotas(t, dir)
	ctx := t.Context()
	libro, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	viva, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Viva", Content: "<p>sigue</p>", Kind: "text"})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 5)
	for i := range 5 {
		n, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Secreta", Content: "<p>texto purgado " + strconv.Itoa(i) + "</p>", Kind: "text"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}
	diario := func() []byte {
		raw, _ := os.ReadFile(filepath.Join(dir, "registros.jsonl"))
		return raw
	}
	// Otra instancia abierta: no compacta, y no pasa nada.
	otro, err := almacen.AbrirPuerto(dir, rec.Config.Scope)
	if err != nil {
		t.Fatal(err)
	}
	rec.CompactarYa(repo.RefreshPrivateData)
	time.Sleep(300 * time.Millisecond)
	if !contieneEnClaro(t, diario(), "texto purgado 0") {
		t.Fatal("la prueba no prepara lo que dice")
	}
	if err := otro.Cerrar(); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for _, id := range ids {
		if err := repo.DeleteNote(ctx, id); err != nil {
			t.Fatal(err)
		}
		if err := repo.PurgeNote(ctx, id); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() { defer wg.Done(); rec.CompactarYa(repo.RefreshPrivateData) }()
	}
	wg.Wait()
	hechas := almacen.CompactacionesYa(rec)
	limite := time.Now().Add(10 * time.Second)
	for contieneEnClaro(t, diario(), "texto purgado") {
		if time.Now().After(limite) {
			t.Fatal("lo purgado sigue en el diario sin cerrar la app")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	if n := almacen.CompactacionesYa(rec) - hechas; n < 1 || n > 2 {
		t.Errorf("cinco peticiones seguidas deberían ser una o dos compactaciones: %d", n)
	}
	if !contieneEnClaro(t, diario(), "sigue") {
		t.Fatal("se perdió lo vivo")
	}
	if _, err := repo.UpdateNote(ctx, viva.ID, libro.ID, api.NoteInput{Title: "Viva", Content: "<p>sigue y cambia</p>", Kind: "text"}); err != nil {
		t.Fatal("escribir tras compactar en el acto:", err)
	}
	if ns, err := repo.ListNotes(ctx, libro.ID); err != nil || len(ns) != 1 || ns[0].Content != "<p>sigue y cambia</p>" {
		t.Fatal(ns, err)
	}

	// Parar olvida lo pedido que no ha empezado, sin esperar a su plazo.
	almacen.EsperaCompactarYa = time.Hour
	hechas = almacen.CompactacionesYa(rec)
	rec.CompactarYa(repo.RefreshPrivateData)
	empezo := time.Now()
	rec.PararCompactar()
	if time.Since(empezo) > 5*time.Second || almacen.CompactacionesYa(rec) != hechas {
		t.Fatal("parar esperó al plazo o compactó lo pendiente")
	}
	rec.CompactarYa(repo.RefreshPrivateData) // parado: no hace nada
	repo.Close()
	rec.Cerrar()
	if almacen.CompactacionesYa(rec) != hechas {
		t.Fatal("compactó en el acto después de parar")
	}
}
