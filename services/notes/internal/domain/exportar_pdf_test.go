// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"strings"
	"testing"
)

// El PDF se comprueba mirándolo por dentro: pdfsimple no comprime los flujos de
// contenido, así que el texto está en claro y no hace falta un lector de PDF —ni
// la dependencia que vendría con él— para saber si salió.

func cuadernoDeMuestra() (Notebook, []Note) {
	return Notebook{Title: "Mudanza"}, []Note{
		{
			Title: "Cocina", Content: "<p>Vajilla</p><ul><li>Platos</li><li>Vasos</li></ul>",
			Col: 0, Position: 1,
			States: []NoteState{{TagName: "Prioridad", Name: "Alta"}},
			Links:  []NoteLink{{TargetKind: "drive_file", TargetID: "f1", Label: "Presupuesto", URL: "https://drive.example/f1"}},
		},
		{Title: "Salón", Content: "<p>Libros</p>", Col: 0, Position: 0},
		{Title: "Secreta", ContentCipher: "AAAA", Col: 1, Position: 0},
	}
}

func TestPDFEsUnPDFYLlevaElCuaderno(t *testing.T) {
	nb, notas := cuadernoDeMuestra()
	b := NotebookAPDF(nb, notas)

	// Un PDF sin cabecera o sin `%%EOF` es un fichero que el visor rechaza entero,
	// y eso no se ve leyendo el texto: hay que mirar los extremos.
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("no empieza por %%PDF-: %q", b[:min(16, len(b))])
	}
	if !bytes.Contains(b, []byte("%%EOF")) {
		t.Error("falta la marca de fin de fichero; el visor no lo abriría")
	}

	doc := string(b)
	for _, quiero := range []string{"Mudanza", "Cocina", "Sal", "Platos", "Vasos", "Libros"} {
		if !strings.Contains(doc, quiero) {
			t.Errorf("el PDF no lleva %q", quiero)
		}
	}
	// Las listas del HTML tienen que salir en LÍNEAS: si `Platos` y `Vasos`
	// acabaran en el mismo `Tj`, el volcado sería un renglón corrido, que es
	// justo lo que hace inútil una lista impresa.
	if strings.Contains(doc, "PlatosVasos") || strings.Contains(doc, "Platos Vasos") {
		t.Error("los elementos de la lista salieron pegados en una línea")
	}
}

func TestPDFRespetaElOrdenDePantalla(t *testing.T) {
	nb, notas := cuadernoDeMuestra()
	doc := string(NotebookAPDF(nb, notas))
	// `Salón` está en la posición 0 y `Cocina` en la 1: en el papel manda lo que
	// se ve, no el orden en que llegaron de la base de datos.
	if strings.Index(doc, "Sal") > strings.Index(doc, "Cocina") {
		t.Error("las notas salieron en el orden de la lista y no en el de pantalla")
	}
}

// Lo importante de una nota cifrada en papel es que se NOMBRE. Si se omitiera,
// quien imprime el cuaderno creería que lo tiene entero.
func TestPDFDiceQueHayUnaNotaCifradaYNoVuelcaElCriptograma(t *testing.T) {
	nb, notas := cuadernoDeMuestra()
	doc := string(NotebookAPDF(nb, notas))
	if !strings.Contains(doc, "cifrada") {
		t.Error("la nota cifrada desapareció sin decirlo")
	}
	if strings.Contains(doc, "AAAA") {
		t.Error("el criptograma acabó en el PDF; ahí son bytes que nadie puede abrir")
	}
}

// Los enlaces se imprimen con su URL entera: en papel no se puede pulsar nada, y
// un rótulo suelto no lleva a ningún sitio.
func TestPDFImprimeLaURLDeLosEnlaces(t *testing.T) {
	nb, notas := cuadernoDeMuestra()
	doc := string(NotebookAPDF(nb, notas))
	if !strings.Contains(doc, "https://drive.example/f1") {
		t.Error("el enlace salió sin su URL")
	}
	if !strings.Contains(doc, "Prioridad: Alta") {
		t.Error("faltan las etiquetas; sin ellas dos estados que se llamen igual no se distinguen")
	}
}

func TestPDFDeCuadernoVacio(t *testing.T) {
	// Sin notas sigue teniendo que ser un PDF abrible: un cuaderno recién creado
	// se exporta igual, y devolver algo roto sería peor que devolver una hoja.
	b := NotebookAPDF(Notebook{}, nil)
	if !bytes.HasPrefix(b, []byte("%PDF-")) || !bytes.Contains(b, []byte("%%EOF")) {
		t.Fatal("el PDF de un cuaderno vacío no es un PDF")
	}
	if !strings.Contains(string(b), "Cuaderno sin t") {
		t.Error("un cuaderno sin título salió sin título en vez de con el de respaldo")
	}
}

// EL HTML NO SE SIRVE `inline`. Lleva contenido de usuario y darle el origen de
// la API sería darle un sitio desde el que hablarle a la sesión de quien lo abre.
func TestSoloElHTMLSeQuedaFueraDeLaVistaEnLinea(t *testing.T) {
	if FormatoHTML.PuedeVerseEnLinea() {
		t.Error("el HTML se serviría inline en el origen de la API")
	}
	for _, f := range []FormatoExport{FormatoPDF, FormatoMarkdown, FormatoMarkdownLargo, FormatoTextoPlano, FormatoNativo} {
		if !f.PuedeVerseEnLinea() {
			t.Errorf("%q no se puede ver en línea y no ejecuta nada", f)
		}
	}
}

// Exportar tiene que enrutar cada formato al suyo: un `switch` al que se le
// olvida un caso cae al Markdown de respaldo y entrega un fichero que no es el
// que se pidió, con la extensión que se pidió.
func TestExportarEnrutaCadaFormato(t *testing.T) {
	nb, notas := cuadernoDeMuestra()
	casos := []struct {
		f      FormatoExport
		prefix string
	}{
		{FormatoPDF, "%PDF-"},
		{FormatoNativo, "{"},
		{FormatoHTML, "<!doctype html>"},
		{FormatoMarkdown, "# Mudanza"},
		{FormatoMarkdownLargo, "# Mudanza"},
		{FormatoTextoPlano, "Mudanza"},
	}
	for _, c := range casos {
		if got := string(Exportar(nb, notas, c.f)); !strings.HasPrefix(got, c.prefix) {
			t.Errorf("Exportar(%q) empieza por %q; quiero %q", c.f, got[:min(20, len(got))], c.prefix)
		}
	}
	if got := FormatoPDF.TipoMIME(); got != "application/pdf" {
		t.Errorf("TipoMIME del PDF = %q", got)
	}
}
