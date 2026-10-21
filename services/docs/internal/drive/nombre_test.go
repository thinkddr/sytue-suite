// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSaneado(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		quiero string
	}{
		{"título normal", "Informe anual", "Informe anual"},
		// La barra es lo que haría que Drive rechazara el nombre entero: un título
		// como este es de lo más corriente y no puede impedir crear un documento.
		{"barra a guion", "Ingresos 2026/2027", "Ingresos 2026-2027"},
		{"barra invertida", `C:\notas`, "C:-notas"},
		{"control a espacio", "Acta\ndel\tconsejo", "Acta del consejo"},
		{"espacios colapsados", "  dos   espacios  ", "dos espacios"},
		{"puntos en los extremos", "...oculto...", "oculto"},
		{"vacío", "   ", "Sin título"},
		{"solo separadores", "///", "Sin título"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := Saneado(c.in); got != c.quiero {
				t.Errorf("Saneado(%q) = %q, quiero %q", c.in, got, c.quiero)
			}
		})
	}
}

// El recorte por longitud es por BYTES (como la comprobación de Drive) y no puede
// partir un carácter multibyte: un nombre con UTF-8 roto es un nombre inválido.
func TestSaneadoRecortaSinRomperUTF8(t *testing.T) {
	got := Saneado(strings.Repeat("ñ", 300))
	if len(got) > maxNombre {
		t.Errorf("longitud en bytes = %d, quiero <= %d", len(got), maxNombre)
	}
	if !utf8.ValidString(got) {
		t.Errorf("Saneado devolvió UTF-8 inválido: %q", got)
	}
	if got == "" {
		t.Error("un título largo no puede quedar vacío")
	}
}

// La razón de ser de NombreLibre: en Drive, subir con un nombre existente
// SUSTITUYE el fichero. Sin sufijo, dos documentos «Sin título» compartirían
// fichero y —por el índice único de source_ref (mig 0006)— acabarían siendo uno.
func TestNombreLibreEvitaLaSustitucion(t *testing.T) {
	ya := []Fichero{{Name: "Sin título.odt"}, {Name: "Sin título (2).odt"}}
	if got := NombreLibre(ya, "Sin título", "odt"); got != "Sin título (3).odt" {
		t.Errorf("NombreLibre = %q, quiero %q", got, "Sin título (3).odt")
	}
	if got := NombreLibre(ya, "Otro", "odt"); got != "Otro.odt" {
		t.Errorf("carpeta sin conflicto: NombreLibre = %q", got)
	}
	if got := NombreLibre(nil, "", "ods"); got != "Sin título.ods" {
		t.Errorf("carpeta vacía y título vacío: NombreLibre = %q", got)
	}
}

// Drive resuelve los conflictos de nombre SIN distinguir mayúsculas: comparar
// sensible aquí dejaría pasar «INFORME.odt» contra «informe.odt» y volveríamos a
// la sustitución silenciosa.
func TestNombreLibreIgnoraMayusculas(t *testing.T) {
	ya := []Fichero{{Name: "INFORME.ODT"}}
	if got := NombreLibre(ya, "Informe", "odt"); got != "Informe (2).odt" {
		t.Errorf("NombreLibre = %q, quiero %q", got, "Informe (2).odt")
	}
}

func TestSufijoDesambiguador(t *testing.T) {
	casos := []struct {
		nombre      string
		titulo, ext string
		final       string
		quiero      string
	}{
		{
			"sin colisión no añade nada", "Documento sin título", "odt",
			"Documento sin título.odt", "",
		},
		{
			"el segundo lleva su (2)", "Documento sin título", "odt",
			"Documento sin título (2).odt", " (2)",
		},
		{
			"y el décimo su (10)", "Hoja sin título", "ods",
			"Hoja sin título (10).ods", " (10)",
		},
		// El nombre del fichero pasó por Saneado, así que la barra se volvió guion.
		// El sufijo se calcula contra el nombre SANEADO, no contra el título crudo:
		// si se comparara contra el crudo, no casaría el prefijo y se perdería el
		// «(2)» justo en los títulos con caracteres que hay que limpiar.
		{
			"con el título saneado sigue casando", "Informe 2026/2027", "odt",
			"Informe 2026-2027 (2).odt", " (2)",
		},
		// Un fichero que ya se llamaba «algo (2)» por su cuenta no debe hacer que se
		// invente un sufijo: el título pedido era ESE, y no hubo desambiguación.
		{
			"un paréntesis propio no es un sufijo", "Acta (2)", "odt",
			"Acta (2).odt", "",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SufijoDesambiguador(c.titulo, c.ext, c.final); got != c.quiero {
				t.Errorf("SufijoDesambiguador(%q, %q, %q) = %q, quiero %q",
					c.titulo, c.ext, c.final, got, c.quiero)
			}
		})
	}
}
