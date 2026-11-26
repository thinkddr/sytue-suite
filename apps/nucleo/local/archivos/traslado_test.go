// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Tras mover la carpeta de datos entera (copiarla y Trasladar), Drive abre en la nueva
// con sus versiones, favoritos, etiquetas, recientes y papelera, y nada apunta a la vieja.
func TestTrasladarLlevaElIndiceYLasVersiones(t *testing.T) {
	base := t.TempDir()
	vieja, nueva := filepath.Join(base, "vieja"), filepath.Join(base, "nueva")
	a, err := Abrir(filepath.Join(vieja, "Drive"), Opciones{Instalacion: "inst-1", Autor: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	f, err := a.Upload(ctx, idDe(a.Raiz()), "acta.txt", "text/plain", strings.NewReader("uno"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if f, err = a.ReplaceFile(ctx, f, "text/plain", strings.NewReader("dos"), 3); err != nil {
		t.Fatal(err)
	}
	if err := a.SetFavorite(ctx, api.DriveFile, f.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := a.PonerEtiquetas(ctx, f.ID, []string{"trabajo"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Recordar(f.Path); err != nil {
		t.Fatal(err)
	}
	// Un reciente de fuera de la carpeta de datos se queda como estaba.
	fuera := filepath.Join(base, "fuera.txt")
	if err := os.WriteFile(fuera, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.Recordar(fuera); err != nil {
		t.Fatal(err)
	}
	g, err := a.Upload(ctx, idDe(a.Raiz()), "borrador.txt", "text/plain", strings.NewReader("b"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Delete(ctx, api.DriveFile, g.ID); err != nil {
		t.Fatal(err)
	}

	if err := os.CopyFS(nueva, os.DirFS(vieja)); err != nil {
		t.Fatal(err)
	}
	if err := Trasladar(filepath.Join(nueva, "Drive"), vieja, nueva); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(nueva, "Drive", Meta, "indice.json"))
	if strings.Contains(string(raw), vieja+string(filepath.Separator)) {
		t.Errorf("el índice sigue con la carpeta vieja:\n%s", raw)
	}
	if !strings.Contains(string(raw), fuera) {
		t.Errorf("el reciente de fuera ha cambiado:\n%s", raw)
	}

	b, err := Abrir(filepath.Join(nueva, "Drive"), Opciones{Instalacion: "inst-1", Autor: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	enNueva := filepath.Join(nueva, "Drive", "acta.txt")
	id := idDe(enNueva)
	vs, _, err := b.Versions(ctx, id, "")
	if err != nil || len(vs) != 2 {
		t.Fatalf("versiones en la nueva: %v, %v", vs, err)
	}
	favs, err := b.Favorites(ctx)
	if err != nil || len(favs) != 1 || favs[0].Path != enNueva {
		t.Errorf("favoritos: %+v, %v", favs, err)
	}
	if es, _ := b.Etiquetas(ctx, id); !slices.Equal(es, []string{"trabajo"}) {
		t.Errorf("etiquetas: %v", es)
	}
	papelera, err := b.Trash(ctx)
	if err != nil || len(papelera.Files) != 1 {
		t.Fatalf("papelera: %+v, %v", papelera, err)
	}
	if err := b.Restore(ctx, api.DriveFile, papelera.Files[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(nueva, "Drive", "borrador.txt")); err != nil {
		t.Errorf("la papelera no devuelve a la carpeta nueva: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vieja, "Drive", "borrador.txt")); err == nil {
		t.Error("la papelera ha devuelto a la carpeta vieja")
	}
	// Sin índice, nada que hacer.
	if err := Trasladar(t.TempDir(), vieja, nueva); err != nil {
		t.Errorf("sin índice: %v", err)
	}
}
