// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// Restos son lo del almacén en su carpeta y nada más: ni objetos/ (que una base puede
// seguir usando), ni el cerrojo (se quita al final), ni la base que lo sustituye, ni lo
// que haya puesto otro; Medir cuenta lo de dentro de las carpetas y Borrar lo quita.
func TestRestosSonSoloDelAlmacen(t *testing.T) {
	dir := t.TempDir()
	rec, err := almacen.Abrir(t.Context(), dir, "contacts", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	rec.Cerrar()
	for _, f := range []string{"libreta.sqlite", "libreta.sqlite-wal", ".compactando-123", ".origen.json.9.tmp", "otra.json", "origen.json"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("12345"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "agenda"), 0o700); err != nil {
		t.Fatal(err)
	}
	rutas, err := almacen.Restos(dir)
	if err != nil {
		t.Fatal(err)
	}
	nombres := make([]string, 0, len(rutas))
	for _, r := range rutas {
		nombres = append(nombres, filepath.Base(r))
	}
	slices.Sort(nombres)
	if quiero := []string{".compactando-123", ".origen.json.9.tmp", "diario", "diario.clave", "origen.json", "registros.jsonl"}; !slices.Equal(nombres, quiero) {
		t.Fatalf("restos %v; quiero %v", nombres, quiero)
	}
	n, b, err := almacen.Medir(rutas...)
	if err != nil || n < 5 || b < 15 {
		t.Fatal("medir:", n, b, err)
	}
	bn, bb, err := almacen.Borrar(rutas...)
	if err != nil || bn != n || bb != b {
		t.Fatal("borrar:", bn, bb, err, "medido", n, b)
	}
	almacen.QuitarCerrojo(dir)
	entradas, _ := os.ReadDir(dir)
	quedan := make([]string, 0, len(entradas))
	for _, e := range entradas {
		quedan = append(quedan, e.Name())
	}
	if quiero := []string{"agenda", "libreta.sqlite", "libreta.sqlite-wal", "objetos", "otra.json"}; !slices.Equal(quedan, quiero) {
		t.Fatalf("quedan %v; quiero %v", quedan, quiero)
	}
	if almacen.EsAlmacen(dir) {
		t.Fatal("sigue siendo un almacén")
	}
}
