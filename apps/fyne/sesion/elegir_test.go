// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestSoltarElegidoDelegaEnAnfitrion: la edición conectada suelta la copia del selector
// del teléfono por el mismo camino que Sytue Office (anfitrion.SoltarElegido).
func TestSoltarElegidoDelegaEnAnfitrion(t *testing.T) {
	dir, err := os.MkdirTemp("", "sytue-elegido-")
	if err != nil {
		t.Fatal(err)
	}
	ruta := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(ruta, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	SoltarElegido(ruta)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		_ = os.RemoveAll(dir)
		t.Fatalf("la copia sigue en %s", dir)
	}
}

func TestLeerLocalSeguroYDirecto(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "informe.pdf")
	contenido := []byte("%PDF-1.7 prueba leer local")
	if err := os.WriteFile(f, contenido, 0o600); err != nil {
		t.Fatal(err)
	}

	nom, datos, err := LeerLocal("file://" + f)
	if err != nil {
		t.Fatalf("no se pudo leer file:// con LeerLocal: %v", err)
	}
	if nom != "informe.pdf" {
		t.Errorf("nombre esperado informe.pdf, obtenido %q", nom)
	}
	if !bytes.Equal(datos, contenido) {
		t.Errorf("datos inesperados: %q", string(datos))
	}
}
