// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build edicionlocal

package local

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/notes"
)

// TestLosDatosEstanEnClaro (R-CLARO, §8.1): lo que la persona escribe queda en su carpeta
// de datos SIN cifrar, por el camino de la cáscara (`l.Almacen`, `l.Biblioteca`): el texto
// de una nota se encuentra en el diario de `Datos/notes` (dentro de su JSON, en base64 o
// tal cual: base64 no es cifrado) y el de un documento de Docus, tal cual en su JSON de
// `Documentos/docs`. Ninguna clave hace falta para leerlos.
//
// VISTO EN ROJO (2026-11-25): buscando otro texto que el escrito, falló en los dos.
func TestLosDatosEstanEnClaro(t *testing.T) {
	casaDePrueba(t)
	l, err := abrirDePrueba(t, "notes")
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	const nota, documento = "comprar pan y aceite de oliva", "acta de la reunión del martes"

	rec, err := l.Almacen(ctx, "notes")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := notes.NewLocal(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := almacen.Preparar(ctx, repo); err != nil {
		t.Fatal(err)
	}
	libro, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "Casa"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Lista", Content: "<p>" + nota + "</p>", Kind: "text"}); err != nil {
		t.Fatal(err)
	}
	repo.Close()

	b, err := l.Biblioteca("docs")
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := b.CreateDocumentFromSource(ctx, "", "Actas", "")
	if err != nil {
		t.Fatal(err)
	}
	base, err := b.DocumentContent(ctx, d.ID, "doc:html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.SaveDocumentChecked(ctx, d.ID, "doc:html", "<p>"+documento+"</p>", base, api.DocDot{}, true); err != nil {
		t.Fatal(err)
	}
	l.Cerrar()

	if !enClaroBajo(t, l.Espacio.Rutas.Producto("notes"), nota, true) {
		t.Errorf("el texto de la nota no está en claro en %s", l.Espacio.Rutas.Producto("notes"))
	}
	if !enClaroBajo(t, l.Espacio.Rutas.Biblioteca(), documento, false) {
		t.Errorf("el texto del documento no está tal cual en %s", l.Espacio.Rutas.Biblioteca())
	}
}

// enClaroBajo dice si algún fichero bajo dir contiene texto: tal cual o, con
// conBase64, dentro de los textos en base64 de sus líneas JSON.
func enClaroBajo(t *testing.T, dir, texto string, conBase64 bool) bool {
	t.Helper()
	hay := false
	err := filepath.WalkDir(dir, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || hay {
			return err
		}
		crudo, err := os.ReadFile(ruta) //nolint:gosec // la carpeta de datos de la prueba
		if err != nil {
			return err
		}
		if bytes.Contains(crudo, []byte(texto)) {
			hay = true
			return nil
		}
		if conBase64 {
			for _, linea := range bytes.Split(crudo, []byte("\n")) {
				var v any
				if json.Unmarshal(linea, &v) == nil && contiene(v, texto, 0) {
					hay = true
					return nil
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hay
}

// contiene busca texto en v (un JSON descodificado), descodificando los textos en base64
// por capas.
func contiene(v any, texto string, nivel int) bool {
	if nivel > 4 {
		return false
	}
	switch x := v.(type) {
	case string:
		if strings.Contains(x, texto) {
			return true
		}
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			if b, err := enc.DecodeString(x); err == nil {
				if bytes.Contains(b, []byte(texto)) {
					return true
				}
				var dentro any
				if json.Unmarshal(b, &dentro) == nil && contiene(dentro, texto, nivel+1) {
					return true
				}
			}
		}
	case map[string]any:
		for _, e := range x {
			if contiene(e, texto, nivel+1) {
				return true
			}
		}
	case []any:
		for _, e := range x {
			if contiene(e, texto, nivel+1) {
				return true
			}
		}
	}
	return false
}
