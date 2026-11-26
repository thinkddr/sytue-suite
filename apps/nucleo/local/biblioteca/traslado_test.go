// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package biblioteca

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Un documento anclado a un fichero de la carpeta de datos pasa a la ruta nueva; uno
// anclado a un fichero de fuera y uno sin anclar no se tocan (ni se reescriben).
func TestTrasladar(t *testing.T) {
	base := t.TempDir()
	vieja, nueva := filepath.Join(base, "vieja"), filepath.Join(base, "nueva")
	dir := filepath.Join(nueva, "Documentos")
	escribir := func(id string, f *Fichero) string {
		t.Helper()
		r := &registro{Formato: formatoRegistro, Documento: api.Document{ID: id, Title: "Acta " + id}, Fichero: f}
		raw, err := json.MarshalIndent(r, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		ruta := filepath.Join(dir, "docs", id+".json")
		if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ruta, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return ruta
	}
	dentro := escribir("a", &Fichero{Ruta: filepath.Join(vieja, "Drive", "acta.docx"), Huella: "h"})
	fuera := escribir("b", &Fichero{Ruta: filepath.Join(base, "Documentos", "carta.docx")})
	suelto := escribir("c", nil)
	antesFuera, _ := os.ReadFile(fuera)
	antesSuelto, _ := os.ReadFile(suelto)
	if err := Trasladar(dir, vieja, nueva); err != nil {
		t.Fatal(err)
	}
	var r registro
	raw, _ := os.ReadFile(dentro)
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	if r.Fichero.Ruta != filepath.Join(nueva, "Drive", "acta.docx") || r.Fichero.Huella != "h" || r.Documento.Title != "Acta a" {
		t.Errorf("anclado dentro: %+v %+v", r.Fichero, r.Documento)
	}
	if ahora, _ := os.ReadFile(fuera); !bytes.Equal(ahora, antesFuera) {
		t.Error("ha reescrito el anclado de fuera")
	}
	if ahora, _ := os.ReadFile(suelto); !bytes.Equal(ahora, antesSuelto) {
		t.Error("ha reescrito el que no está anclado")
	}
	if err := Trasladar(filepath.Join(base, "no-existe"), vieja, nueva); err != nil {
		t.Errorf("sin biblioteca: %v", err)
	}
}
