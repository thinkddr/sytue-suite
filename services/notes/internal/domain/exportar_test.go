// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

// Lo que se fija aquí es el CONTRATO de la exportación, que es lo que alguien
// recibe cuando ejerce su derecho a llevarse los datos: si esto se rompe, no da
// error —sale un fichero, se descarga— y el fallo solo se ve al abrirlo, cuando
// ya no hay a quién reclamar.

func TestExportarOrdenaComoSeVeEnPantalla(t *testing.T) {
	// A propósito desordenadas respecto al orden final: si el volcado siguiera el
	// orden de entrada, este test pasaría igual con la lista ya ordenada.
	notas := []Note{
		{Title: "segunda de la primera columna", Content: "b", Col: 0, Position: 2},
		{Title: "primera de la segunda columna", Content: "c", Col: 1, Position: 1},
		{Title: "primera de la primera columna", Content: "a", Col: 0, Position: 1},
	}
	md := NotebookAMarkdown(Notebook{Title: "Cuaderno"}, notas)

	i1 := strings.Index(md, "primera de la primera")
	i2 := strings.Index(md, "segunda de la primera")
	i3 := strings.Index(md, "primera de la segunda")
	if i1 >= i2 || i2 >= i3 {
		t.Fatalf("el orden no es columna→posición:\n%s", md)
	}
}

// UNA NOTA CIFRADA NO SE EXPORTA Y SE DICE. Omitirla en silencio sería peor que
// no exportar: quien recibe el fichero creería que ya lo tiene todo.
func TestExportarAvisaDeLoCifrado(t *testing.T) {
	md := NotebookAMarkdown(Notebook{Title: "C"}, []Note{
		{Title: "secreta", ContentCipher: "encv1:AAAA"},
	})
	if strings.Contains(md, "encv1:") {
		t.Error("el criptograma salió en el volcado: son bytes que nadie puede abrir")
	}
	if !strings.Contains(md, "cifrada") {
		t.Errorf("no avisa de que la nota está cifrada:\n%s", md)
	}
	if !strings.Contains(md, "secreta") {
		t.Error("se perdió el título, que NO está cifrado y sí se puede exportar")
	}
}

func TestExportarIncluyeEtiquetasYEnlaces(t *testing.T) {
	md := NotebookAMarkdown(Notebook{Title: "C"}, []Note{{
		Title:   "con contexto",
		Content: "<p>cuerpo</p>",
		States:  []NoteState{{TagName: "Prioridad", Name: "Alta"}},
		Links: []NoteLink{
			{TargetKind: "calendar_event", TargetID: "uid-1", Label: "Reunión"},
			{TargetKind: "drive_file", TargetID: "f-2", Label: "Informe", URL: "https://drive.sytue.eu/f/2"},
		},
	}})
	// El estado va con su etiqueta: «Alta» a secas no significa nada fuera de la
	// interfaz, donde el color lo desambigua.
	if !strings.Contains(md, "Prioridad: Alta") {
		t.Errorf("falta la etiqueta junto a su estado:\n%s", md)
	}
	// Un enlace sin URL conserva su destino: sin el identificador, el volcado dice
	// que había un enlace pero no a dónde.
	if !strings.Contains(md, "uid-1") {
		t.Errorf("un enlace sin URL perdió su destino:\n%s", md)
	}
	if !strings.Contains(md, "[Informe](https://drive.sytue.eu/f/2)") {
		t.Errorf("el enlace con URL no salió como enlace Markdown:\n%s", md)
	}
}

func TestExportarNoDejaHuecosSinNombre(t *testing.T) {
	md := NotebookAMarkdown(Notebook{}, []Note{{}})
	// Un «## » suelto rompe el índice de cualquier lector de Markdown.
	for _, l := range strings.Split(md, "\n") {
		if strings.TrimRight(l, " ") == "#" || strings.TrimRight(l, " ") == "##" {
			t.Fatalf("encabezado sin texto:\n%s", md)
		}
	}
	if !strings.Contains(md, "sin título") {
		t.Errorf("no pone un nombre por defecto:\n%s", md)
	}
	if !strings.Contains(md, "vacía") {
		t.Errorf("una nota sin contenido tiene que decirlo:\n%s", md)
	}
}

func TestExportarCuadernoSinNotas(t *testing.T) {
	md := NotebookAMarkdown(Notebook{Title: "Vacío"}, nil)
	if !strings.HasPrefix(md, "# Vacío") {
		t.Errorf("un cuaderno sin notas debe exportar al menos su título:\n%q", md)
	}
}

// El título del cuaderno acaba dentro de una cabecera `Content-Disposition`, así
// que una comilla o un salto de línea no son un nombre feo: parten la cabecera en
// dos y lo que venga detrás lo interpreta el navegador como otra cosa.
func TestNombreDeFicheroNoRompeLaCabecera(t *testing.T) {
	casos := []struct{ titulo, prohibido string }{
		{`Notas "importantes"`, `"`},
		{"Con\nsalto", "\n"},
		{"Con\rretorno", "\r"},
		{"ruta/con/barras", "/"},
		{`con\contrabarra`, `\`},
	}
	for _, c := range casos {
		got := NombreDeFicheroExportado(c.titulo, FormatoMarkdown)
		if strings.Contains(got, c.prohibido) {
			t.Errorf("nombreDeFichero(%q) = %q; conserva %q", c.titulo, got, c.prohibido)
		}
		if !strings.HasSuffix(got, ".md") {
			t.Errorf("nombreDeFichero(%q) = %q; sin extensión", c.titulo, got)
		}
	}
	if got := NombreDeFicheroExportado("   ", FormatoMarkdown); got != "cuaderno.md" {
		t.Errorf("un título vacío debe caer a un nombre por defecto, y dio %q", got)
	}
}

// `.md`, `.MD` y `.markdown` son el MISMO formato con otro nombre: lo que cambia
// es la extensión del fichero. Tratarlas como formatos distintos obligaría a
// mantener tres generadores que se separan al primer cambio.
func TestFormatoDeAceptaLasExtensionesQueLaGenteEscribe(t *testing.T) {
	casos := []struct {
		entrada string
		quiero  FormatoExport
		ext     string
	}{
		// Sin formato sale el NATIVO: quien no elige quiere su cuaderno entero.
		{"", FormatoHTML, ".html"},
		{"  ", FormatoHTML, ".html"},
		{"md", FormatoMarkdown, ".md"},
		{"MD", FormatoMarkdown, ".md"},
		{".md", FormatoMarkdown, ".md"},
		{".MD", FormatoMarkdown, ".md"},
		{"markdown", FormatoMarkdownLargo, ".markdown"},
		{".Markdown", FormatoMarkdownLargo, ".markdown"},
		{"txt", FormatoTextoPlano, ".txt"},
		{".TXT", FormatoTextoPlano, ".txt"},
		{"  txt  ", FormatoTextoPlano, ".txt"},
		{"onex", FormatoNativo, ".onex"},
		{".ONEX", FormatoNativo, ".onex"},
		{"pdf", FormatoPDF, ".pdf"},
		{".PDF", FormatoPDF, ".pdf"},
	}
	for _, c := range casos {
		got, ok := FormatoDe(c.entrada)
		if !ok || got != c.quiero {
			t.Errorf("FormatoDe(%q) = (%q, %v); quiero %q", c.entrada, got, ok, c.quiero)
			continue
		}
		if got.Extension() != c.ext {
			t.Errorf("FormatoDe(%q).Extension() = %q; quiero %q", c.entrada, got.Extension(), c.ext)
		}
	}
	// Un formato que no existe se RECHAZA en vez de caer a Markdown en silencio:
	// quien pide `.docx` prefiere un error a un fichero que no es lo que pidió.
	// `.one` sigue fuera a propósito: es el formato BINARIO de OneNote, no una
	// variante de `.onex`, y aceptarlo prometería una compatibilidad que no hay.
	for _, malo := range []string{"docx", "one", "odt", "rtf"} {
		if _, ok := FormatoDe(malo); ok {
			t.Errorf("FormatoDe(%q) se aceptó y no está implementado", malo)
		}
	}
}

// El volcado a texto plano NO es el Markdown sin asteriscos: el contenido es
// HTML y hay que recorrerlo, o todo el cuaderno sale como un párrafo corrido.
func TestTextoPlanoConservaLosCortesDeLinea(t *testing.T) {
	md := NotebookATextoPlano(Notebook{Title: "C"}, []Note{{
		Title:   "lista",
		Content: "<p>Primero</p><p>Segundo</p><ul><li>uno</li><li>dos</li></ul>",
	}})
	if strings.Contains(md, "PrimeroSegundo") {
		t.Errorf("dos párrafos se pegaron en uno:\n%s", md)
	}
	if strings.Contains(md, "unodos") {
		t.Errorf("dos elementos de lista se pegaron:\n%s", md)
	}
	// Y no queda marcado suelto: es texto plano.
	if strings.Contains(md, "<p>") || strings.Contains(md, "<li>") {
		t.Errorf("quedó HTML en el texto plano:\n%s", md)
	}
}

func TestTextoPlanoResuelveEntidadesYEnlaces(t *testing.T) {
	out := NotebookATextoPlano(Notebook{Title: "C"}, []Note{{
		Title:   "n",
		Content: "<p>Uno &amp; otro</p>",
		Links:   []NoteLink{{TargetKind: "drive_file", TargetID: "f", Label: "Informe", URL: "https://x/y"}},
	}})
	if !strings.Contains(out, "Uno & otro") {
		t.Errorf("la entidad no se resolvió:\n%s", out)
	}
	// En texto plano un enlace se lee «rótulo (url)»: los corchetes de Markdown
	// necesitan un lector que los resuelva, y aquí no lo hay.
	if !strings.Contains(out, "Informe (https://x/y)") {
		t.Errorf("el enlace no quedó legible sin Markdown:\n%s", out)
	}
	if strings.Contains(out, "](") {
		t.Errorf("quedó sintaxis de Markdown en el texto plano:\n%s", out)
	}
}

// Lo cifrado se avisa en los DOS formatos: quien exporta a texto no recibe menos
// información sobre lo que no ha podido llevarse.
func TestTextoPlanoAvisaDeLoCifrado(t *testing.T) {
	out := NotebookATextoPlano(Notebook{Title: "C"}, []Note{{Title: "s", ContentCipher: "encv1:AAA"}})
	if strings.Contains(out, "encv1:") {
		t.Error("el criptograma salió en el volcado de texto")
	}
	if !strings.Contains(out, "cifrada") {
		t.Errorf("no avisa de la nota cifrada:\n%s", out)
	}
}

func TestExtensionDelNombreSigueAlFormato(t *testing.T) {
	for _, c := range []struct {
		f   FormatoExport
		ext string
	}{{FormatoMarkdown, ".md"}, {FormatoMarkdownLargo, ".markdown"}, {FormatoTextoPlano, ".txt"}} {
		if got := NombreDeFicheroExportado("Cuaderno", c.f); !strings.HasSuffix(got, c.ext) {
			t.Errorf("NombreDeFicheroExportado(_, %q) = %q; quiero que acabe en %q", c.f, got, c.ext)
		}
	}
}
