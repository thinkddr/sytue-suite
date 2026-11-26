// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"strings"
	"testing"
)

// Solo un `file:` de ESTE equipo con ruta absoluta es un enlace a un fichero (N-07), y
// solo el saneado de la edición local lo deja; el de siempre lo quita como antes.
func TestEnlacesAFicherosSoloEnElSaneadoLocal(t *testing.T) {
	for raw, quiere := range map[string]bool{
		"file:///home/ana/x.pdf":                   true,
		"file:///C:/Users/ana/presupuesto%202.pdf": true,
		"file://localhost/home/ana/x.pdf":          true,
		"FILE:///tmp/x":                            true,
		"file://otra-maquina/x.pdf":                false,
		"file:x.pdf":                               false,
		"file:///":                                 false,
		"file:///x?a=1":                            false,
		"file:///x#y":                              false,
		"file://ana@localhost/x":                   false,
		"https://ejemplo.eu":                       false,
		"javascript:alert(1)":                      false,
		"file:///x\n":                              false,
		"":                                         false,
	} {
		if got := EnlaceAFichero(raw); got != quiere {
			t.Errorf("EnlaceAFichero(%q) = %v", raw, got)
		}
	}
	in := `<p><a href="file:///home/ana/x.pdf" target="_blank">x</a> <a href="https://ejemplo.eu">web</a> <a href="javascript:y()">z</a></p>`
	if got := SanitizeHTMLConFicheros(in); got != `<p><a href="file:///home/ana/x.pdf">x</a> <a href="https://ejemplo.eu" target="_blank" rel="noopener noreferrer">web</a> <a>z</a></p>` {
		t.Errorf("local: %s", got)
	}
	if got := SanitizeHTML(in); strings.Contains(got, "file:") {
		t.Errorf("el saneado de siempre dejó el fichero: %s", got)
	}
	n := NoteInput{Kind: KindText, Content: in}
	if err := n.NormalizeConFicheros(false); err != nil || !strings.Contains(n.Content, `href="file:///home/ana/x.pdf"`) {
		t.Errorf("NormalizeConFicheros: %v %s", err, n.Content)
	}
	n = NoteInput{Kind: KindText, Content: in}
	if err := n.Normalize(false); err != nil || strings.Contains(n.Content, "file:") {
		t.Errorf("Normalize: %v %s", err, n.Content)
	}
}
