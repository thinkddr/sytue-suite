// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
)

// Exportar la biblioteca deja un .docx de verdad (un ZIP de OOXML) por documento, con su
// título por nombre y sin pisar dos que se llamen igual.
func TestExportarBiblioteca(t *testing.T) {
	b, err := biblioteca.Abrir(t.TempDir(), "docs", biblioteca.Persona{})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, _, err := b.CreateDocumentFromSource(context.Background(), "", "Plan", ""); err != nil {
			t.Fatal(err)
		}
	}
	carpeta := t.TempDir()
	if err := ExportarBiblioteca(b, "docx")(context.Background(), carpeta); err != nil {
		t.Fatal(err)
	}
	ficheros, _ := filepath.Glob(filepath.Join(carpeta, "*.docx"))
	if len(ficheros) != 2 {
		t.Fatalf("exportados: %v", ficheros)
	}
	for _, f := range ficheros {
		z, err := zip.OpenReader(f)
		if err != nil {
			t.Fatalf("%s no es un ZIP: %v", f, err)
		}
		hay := false
		for _, e := range z.File {
			hay = hay || e.Name == "word/document.xml"
		}
		_ = z.Close()
		if !hay {
			t.Errorf("%s no es un .docx", f)
		}
	}
	if _, err := os.Stat(filepath.Join(carpeta, "Plan (2).docx")); err != nil {
		t.Errorf("el segundo no se llama «Plan (2).docx»: %v", ficheros)
	}
}
