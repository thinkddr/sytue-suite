// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestElClonNoCompartenEscrituras: el clon tiene los mismos bytes, es OTRO fichero y
// escribir en él no toca el original. Solo corre donde el sistema de ficheros de
// TMPDIR clona (APFS, Btrfs, XFS); en el resto, paraCast copia y lo prueba
// TestAbrirEnCastDejaElVideoCopiadoEnLaCarpetaDeCast.
func TestElClonNoCompartenEscrituras(t *testing.T) {
	dir := t.TempDir()
	origen := filepath.Join(dir, "original.webm")
	datos := bytes.Repeat([]byte("sytue-office "), 4096)
	if err := os.WriteFile(origen, datos, 0o600); err != nil {
		t.Fatal(err)
	}
	clon := nombreOculto(dir, ".webm")
	if err := clonarFichero(origen, clon); err != nil {
		if _, serr := os.Stat(clon); !errors.Is(serr, os.ErrNotExist) {
			t.Fatal("un clon fallido deja el destino a medias:", serr)
		}
		t.Skipf("este sistema de ficheros no clona (%v): paraCast copia", err)
	}
	if err := clonarFichero(origen, clon); err == nil {
		t.Fatal("clonar encima de un fichero que ya está debe fallar")
	}
	a, _ := os.Stat(origen)
	b, _ := os.Stat(clon)
	if os.SameFile(a, b) {
		t.Fatal("el clon es el mismo fichero")
	}
	if en, _ := os.ReadFile(clon); !bytes.Equal(en, datos) {
		t.Fatal("el clon no tiene los bytes del original")
	}
	fd, err := os.OpenFile(clon, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fd.WriteAt([]byte("XXXX"), 0); err != nil {
		t.Fatal(err)
	}
	_ = fd.Close()
	if en, _ := os.ReadFile(origen); !bytes.Equal(en, datos) {
		t.Fatal("escribir en el clon cambió el original")
	}
}
