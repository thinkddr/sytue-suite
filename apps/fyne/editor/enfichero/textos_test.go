// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package enfichero

import "testing"

// TestLaTablaDeEnficheroCuadra: los textos de «Guardar», «Guardar como…» y sus preguntas
// están en los 26 idiomas (C-12), con los mismos %s y en el mismo orden que el inglés.
func TestLaTablaDeEnficheroCuadra(t *testing.T) {
	for _, f := range tabla.Faltan() {
		t.Error(f)
	}
}

// Cada editor habla el idioma de la app; uno que no está cae al inglés.
func TestEnficheroPorIdioma(t *testing.T) {
	for idioma, quiero := range map[string]string{
		"es": "Guardado en «a.docx»", "en": "Saved to “a.docx”", "de": "Gespeichert in „a.docx“",
		"fr": "Enregistré dans « a.docx »", "pt-BR": "Guardado em «a.docx»", "": "Saved to “a.docx”",
	} {
		e := Nuevo(Opciones{Idioma: idioma})
		if got := e.t("guardado", "a.docx"); got != quiero {
			t.Errorf("%q: %q; quiero %q", idioma, got, quiero)
		}
	}
	if got := Nuevo(Opciones{Idioma: "de"}).Editor().RotuloGuardarComo; got != "Speichern unter…" {
		t.Errorf("rótulo: %q", got)
	}
}
