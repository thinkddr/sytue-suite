// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// RutaEnClaro da la ruta de un binario por su huella solo si está y es un fichero, y no
// acepta nada que no sea una huella (ni «../» ni una huella cortada). Que el original de
// un vídeo copiado está ahí lo prueba Photos (TestUnVideoCopiadoEntraEnLaFototeca).
func TestRutaEnClaroSoloDaBinariosQueEstan(t *testing.T) {
	dir := t.TempDir()
	rec, err := almacen.Abrir(context.Background(), dir, "photos", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Cerrar()
	datos := []byte("un vídeo")
	suma := sha256.Sum256(datos)
	sha := hex.EncodeToString(suma[:])
	if _, ok := rec.Puerto.RutaEnClaro(sha); ok {
		t.Fatal("una huella sin binario")
	}
	ruta := filepath.Join(dir, "objetos", sha[:2], sha)
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, datos, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := rec.Puerto.RutaEnClaro(strings.ToUpper(sha)); !ok || got != ruta {
		t.Fatal("la huella en mayúsculas es la misma:", got, ok)
	}
	for _, mala := range []string{"", "../registros.jsonl", sha[:62], sha + "00", strings.Repeat("z", 64)} {
		if got, ok := rec.Puerto.RutaEnClaro(mala); ok {
			t.Errorf("%q no es una huella y da %q", mala, got)
		}
	}
	if err := os.Remove(ruta); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(ruta, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec.Puerto.RutaEnClaro(sha); ok {
		t.Fatal("una carpeta no es un binario")
	}
}
