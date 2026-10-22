// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

func TestSeGuardaEnElFormatoConElQueVino(t *testing.T) {
	// Un .docx que se abre y se guarda sigue siendo un .docx. Cambiarle el formato
	// al fichero de alguien porque el editor prefiere otro es alterar su archivo,
	// no editarlo — y encima el nombre seguiría diciendo lo de antes.
	casos := map[string]domain.BlobFormat{
		"Informe.docx":           domain.FormatOOXML,
		"Cuentas.XLSX":           domain.FormatOOXML,
		"Charla.pptx":            domain.FormatOOXML,
		"Notas.odt":              domain.FormatODF,
		"Presupuesto.ods":        domain.FormatODF,
		"pagina.html":            domain.FormatHTML,
		"apuntes.md":             domain.FormatMarkdown,
		"con.varios.puntos.docx": domain.FormatOOXML,
	}
	for nombre, quiero := range casos {
		if got := formatoDe(nombre); got != quiero {
			t.Errorf("formatoDe(%q) = %q, quería %q", nombre, got, quiero)
		}
	}
}

func TestUnFormatoQueNoSeSabeEscribirNoSeGUARDA(t *testing.T) {
	// Devolver un formato «por defecto» aquí sería lo peor que puede pasarle a un
	// fichero: seguiría en su sitio, con su nombre de siempre, y dentro el
	// contenido de otra cosa. Vacío significa «no guardes», y el handler responde
	// 422 en vez de escribir.
	for _, nombre := range []string{"foto.png", "datos.csv", "libro.epub", "sin-extension", ""} {
		if got := formatoDe(nombre); got != "" {
			t.Errorf("formatoDe(%q) = %q, quería vacío", nombre, got)
		}
	}
}

func TestLaClaveDelModeloEsLaMismaQueUsaElEditor(t *testing.T) {
	// Si estas claves se separan de las del frontend (CONTENT_KEY), el guardado
	// exportaría un documento vacío sin fallar: el exportador buscaría en una clave
	// donde no hay nada y devolvería un fichero válido y en blanco.
	if claveDe(domain.KindDocument) != "doc:html" {
		t.Errorf("documento: %q", claveDe(domain.KindDocument))
	}
	if claveDe(domain.KindSpreadsheet) != "doc:sheet" {
		t.Errorf("hoja: %q", claveDe(domain.KindSpreadsheet))
	}
	if claveDe(domain.KindPresentation) != "doc:slides" {
		t.Errorf("presentación: %q", claveDe(domain.KindPresentation))
	}
}
