// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/storage"
)

// TestLoElegidoEnElMovilLlegaComoRuta: lo que da el selector del toolkit (en el móvil,
// un `content://`) queda copiado entero en una carpeta temporal con su nombre, que es lo
// que esperan las doce llamadas a `ficheros.Elegir`. VISTO EN ROJO sin el `io.Copy`.
func TestLoElegidoEnElMovilLlegaComoRuta(t *testing.T) {
	origen := filepath.Join(t.TempDir(), "acta firmada.pdf")
	if err := os.WriteFile(origen, []byte("%PDF-1.7 acta"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := storage.Reader(storage.NewFileURI(origen))
	if err != nil {
		t.Fatal(err)
	}
	ruta, err := copiarElegido(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(ruta)) })
	if filepath.Base(ruta) != "acta firmada.pdf" || ruta == origen {
		t.Errorf("ha quedado en %s", ruta)
	}
	if b, _ := os.ReadFile(ruta); string(b) != "%PDF-1.7 acta" {
		t.Errorf("la copia dice %q", b)
	}

	// Soltarla borra la copia y su carpeta, y nunca un fichero que no sea una copia.
	SoltarElegido(origen)
	SoltarElegido(ruta)
	if _, err := os.Stat(filepath.Dir(ruta)); !os.IsNotExist(err) {
		t.Errorf("la copia sigue en %s", ruta)
	}
	if _, err := os.Stat(origen); err != nil {
		t.Errorf("soltar una ruta que no es copia ha borrado el original: %v", err)
	}
}

// TestLosVideosSeEligenDeLaGaleria: lo que solo acepta vídeo (subir a Cast) o solo
// imagen (el póster, Photos) se filtra por tipo, que es lo que abre la galería en el
// teléfono; una mezcla sigue por extensión. VISTO EN ROJO devolviendo siempre nil.
func TestLosVideosSeEligenDeLaGaleria(t *testing.T) {
	for _, c := range []struct {
		ext  []string
		tipo string
	}{
		{[]string{"mp4", "m4v", "mov", "mkv", "webm", "avi"}, "video/*"},
		{[]string{"jpg", ".PNG", "webp"}, "image/*"},
		{[]string{"mp4", "jpg"}, ""},
		{[]string{"pdf"}, ""},
		{nil, ""},
	} {
		got := tiposMovil(c.ext)
		if (c.tipo == "" && got != nil) || (c.tipo != "" && (len(got) != 1 || got[0] != c.tipo)) {
			t.Errorf("%v filtra por %v; se esperaba %q", c.ext, got, c.tipo)
		}
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
