// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"errors"
	"net/url"
	"testing"
)

// TestAbrirFicheroEnOfficeNoCaeALaWeb: el fichero del equipo va por el esquema de Sytue Office
// y, si el sistema no lo atiende, el error vuelve a quien llama sin abrir el navegador.
func TestAbrirFicheroEnOfficeNoCaeALaWeb(t *testing.T) {
	a := &abridorDoble{}
	if err := AbrirFicheroEnOffice(a, "docs", "file:///home/ana/Sytue%20Office/Drive/acta.docx"); err != nil {
		t.Fatal(err)
	}
	if len(a.urls) != 1 {
		t.Fatal(a.urls)
	}
	u, _ := url.Parse(a.urls[0])
	if u.Scheme != "sytueoffice-docs" || u.Path != "/abrir" || u.Query().Get("local") != "file:///home/ana/Sytue%20Office/Drive/acta.docx" {
		t.Fatal("dirección", a.urls[0])
	}
	sin := &abridorDoble{falla: func(string) bool { return true }}
	if err := AbrirFicheroEnOffice(sin, "pdf", "file:///tmp/x.pdf"); err == nil || len(sin.urls) != 1 {
		t.Fatal("sin app, el error vuelve y no se prueba nada más", err, sin.urls)
	}
	for _, mal := range [][2]string{{"docs", "https://evil.example/x"}, {"Docs", "file:///x"}, {"do-cs", "file:///x"}, {"", "file:///x"}, {"docs", "file://"}} {
		if err := AbrirFicheroEnOffice(&abridorDoble{}, mal[0], mal[1]); !errors.Is(err, ErrNoSeAbre) {
			t.Fatal("debía rechazarse", mal, err)
		}
	}
}
