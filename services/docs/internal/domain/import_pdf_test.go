// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de abrir un PDF como documento editable (ola P6: convertir).
//
// El caso que de verdad cierra el círculo es el último: PDF → documento → DOCX.
// «PDF a Word» no necesita un exportador nuevo porque, una vez el PDF entra por
// el import, ya es un documento de Docs y exportarlo es lo de siempre.
package domain

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// pdfConTexto genera un PDF con el escritor de la casa.
func pdfConTexto(t *testing.T, lineas ...string) []byte {
	t.Helper()
	w := pdfsimple.Nuevo()
	for _, l := range lineas {
		w.Texto(l, "F1", 12)
	}
	return w.Bytes()
}

func TestImportPDFRecuperaElTexto(t *testing.T) {
	datos := pdfConTexto(t, "Primera línea del contrato", "Segunda línea del contrato")

	res, err := Import("contrato.pdf", datos)
	kind, contenido := res.Kind, res.Content
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if kind != KindDocument {
		t.Errorf("kind = %q, quería document", kind)
	}
	if !strings.Contains(contenido, "Primera línea") {
		t.Errorf("no se recuperó el texto: %q", contenido)
	}
	if !strings.Contains(contenido, "<p>") {
		t.Errorf("el contenido no viene en párrafos: %q", contenido)
	}
}

func TestImportPDFDetectaPorFirmaAunqueLaExtensionMienta(t *testing.T) {
	// Un PDF llamado «.txt» leído como texto plano volcaría su fuente binaria
	// dentro del documento, que es un destrozo silencioso.
	datos := pdfConTexto(t, "soy un pdf")

	res, err := Import("disfrazado.txt", datos)
	kind, contenido := res.Kind, res.Content
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if kind != KindDocument {
		t.Fatalf("kind = %q", kind)
	}
	if strings.Contains(contenido, "%PDF") || strings.Contains(contenido, "endobj") {
		t.Error("se coló la fuente del PDF en el documento: se leyó como texto plano")
	}
	if !strings.Contains(contenido, "soy un pdf") {
		t.Errorf("no se recuperó el texto: %q", contenido)
	}
}

func TestImportPDFEscapaElHTML(t *testing.T) {
	// El texto del PDF acaba dentro de <p>: sin escapar, un documento con
	// «<script>» se convertiría en marcado dentro del editor.
	datos := pdfConTexto(t, "a < b & c > d")

	res, err := Import("x.pdf", datos)
	contenido := res.Content
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if strings.Contains(contenido, "<p>a < b") {
		t.Errorf("el texto no se escapó: %q", contenido)
	}
	if !strings.Contains(contenido, "&lt;") || !strings.Contains(contenido, "&amp;") {
		t.Errorf("faltan las entidades escapadas: %q", contenido)
	}
}

func TestImportPDFSeparaLasPaginas(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("página uno", "F1", 12)
	w.NuevaPagina()
	w.Texto("página dos", "F1", 12)

	res, err := Import("dos.pdf", w.Bytes())
	contenido := res.Content
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !strings.Contains(contenido, "<hr>") {
		t.Error("no hay separador entre páginas: queda un muro de texto")
	}
	if strings.Index(contenido, "una") > strings.Index(contenido, "dos") {
		t.Error("las páginas salieron en orden inverso")
	}
}

func TestImportPDFSinTextoLoDiceClaro(t *testing.T) {
	// Un PDF escaneado no es un error del fichero: es que aún no sabemos leerlo,
	// y decirlo es mejor que devolver un documento vacío que parece un fallo.
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>`,
	}
	_, err := Import("escaneo.pdf", pdfMinimo(t, `<< /Size 4 /Root 1 0 R >>`, objs))
	if err == nil {
		t.Fatal("un PDF sin texto debería dar error explicativo")
	}
	if !strings.Contains(err.Error(), "OCR") {
		t.Errorf("el mensaje no orienta sobre por qué: %v", err)
	}
}

func TestImportPDFCifradoDaErrorDeFormato(t *testing.T) {
	datos := pdfMinimo(t, `<< /Size 5 /Root 1 0 R /Encrypt 4 0 R >>`, nil)
	if _, err := Import("protegido.pdf", datos); err == nil {
		t.Error("un PDF cifrado debería fallar al importar")
	}
}

// --- Reconstrucción de párrafos ---------------------------------------------

func TestParrafosAgrupanLaMismaLinea(t *testing.T) {
	// Un cambio de fuente parte una línea en varios fragmentos; si no se
	// reagrupan, cada trozo saldría como un párrafo suelto.
	frags := []PDFFragmento{
		{Texto: "Hola", X: 10, Y: 700, Tamano: 12},
		{Texto: "mundo", X: 50, Y: 700, Tamano: 12},
	}
	got := parrafosDeFragmentos(frags)
	if len(got) != 1 || got[0] != "Hola mundo" {
		t.Errorf("= %v, quería [«Hola mundo»]", got)
	}
}

func TestParrafosOrdenanPorPosicionNoPorLlegada(t *testing.T) {
	// El orden del flujo de contenido no tiene por qué ser el de lectura.
	frags := []PDFFragmento{
		{Texto: "segunda", X: 10, Y: 680, Tamano: 12},
		{Texto: "primera", X: 10, Y: 700, Tamano: 12},
		{Texto: "b", X: 60, Y: 700, Tamano: 12},
	}
	got := parrafosDeFragmentos(frags)
	if len(got) == 0 || !strings.HasPrefix(got[0], "primera b") {
		t.Errorf("= %v; la Y crece hacia arriba, así que «primera» va antes", got)
	}
}

func TestParrafosSeparanPorSaltoGrande(t *testing.T) {
	// Interlineado de 14 y un salto de 40: eso es un párrafo nuevo.
	frags := []PDFFragmento{
		{Texto: "linea uno", X: 10, Y: 700, Tamano: 12},
		{Texto: "linea dos", X: 10, Y: 686, Tamano: 12},
		{Texto: "linea tres", X: 10, Y: 672, Tamano: 12},
		{Texto: "otro bloque", X: 10, Y: 632, Tamano: 12},
	}
	got := parrafosDeFragmentos(frags)
	if len(got) != 2 {
		t.Fatalf("= %v, quería dos párrafos", got)
	}
	if !strings.Contains(got[0], "linea tres") || got[1] != "otro bloque" {
		t.Errorf("mal repartido: %v", got)
	}
}

func TestUmbralSaleDelDocumentoYNoDeUnaConstante(t *testing.T) {
	// Un PDF a doble espacio tiene interlineados enormes: con un umbral fijo,
	// CADA línea sería un párrafo. El umbral se calcula del propio documento.
	frags := []PDFFragmento{
		{Texto: "uno", X: 10, Y: 700, Tamano: 12},
		{Texto: "dos", X: 10, Y: 660, Tamano: 12}, // interlineado de 40
		{Texto: "tres", X: 10, Y: 620, Tamano: 12},
	}
	got := parrafosDeFragmentos(frags)
	if len(got) != 1 {
		t.Errorf("= %v; con interlineado uniforme de 40 todo es UN párrafo", got)
	}
}

func TestUmbralConUnaSolaLinea(t *testing.T) {
	got := parrafosDeFragmentos([]PDFFragmento{{Texto: "sola", X: 10, Y: 700}})
	if len(got) != 1 || got[0] != "sola" {
		t.Errorf("= %v, quería [«sola»]", got)
	}
}

func TestJuntarCosePalabrasPartidasConGuion(t *testing.T) {
	// Un texto justificado parte palabras al final de línea; pegarlas con el
	// guion dejaría «docu- mento» dentro del documento convertido.
	got := juntar([]string{"esto es un docu-", "mento partido"})
	if got != "esto es un documento partido" {
		t.Errorf("= %q, quería «esto es un documento partido»", got)
	}
	// Y sin guion se une con espacio, no pegado.
	if got := juntar([]string{"dos", "palabras"}); got != "dos palabras" {
		t.Errorf("= %q", got)
	}
}

// --- El círculo completo -----------------------------------------------------

func TestPDFaWordCierraElCirculo(t *testing.T) {
	// Lo que esta ola prometía: abrir un PDF y guardarlo como .docx. No hace
	// falta exportador nuevo — en cuanto el PDF es un documento, ya se exporta.
	datos := pdfConTexto(t, "Contenido que viaja del PDF a Word")

	res, err := Import("origen.pdf", datos)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	estado := NewState()
	if _, err := estado.Apply(Op{
		Kind:    OpSet,
		Dot:     Dot{Actor: "test", Counter: 1},
		Key:     "content",
		Value:   []byte(`"` + strings.ReplaceAll(res.Content, `"`, `\"`) + `"`),
		Lamport: 1,
	}); err != nil {
		t.Fatalf("sembrar el contenido: %v", err)
	}

	salida, contentType, ext, err := Export(res.Kind, res.Title, estado, FormatOOXML)
	if err != nil {
		t.Fatalf("Export a DOCX: %v", err)
	}
	if ext != "docx" {
		t.Errorf("extensión = %q, quería docx", ext)
	}
	if !strings.Contains(contentType, "wordprocessingml") {
		t.Errorf("content-type = %q", contentType)
	}
	// Un .docx es un ZIP: si no empieza por PK, no se ha producido un documento.
	if len(salida) < 4 || string(salida[:2]) != "PK" {
		t.Fatal("la salida no es un paquete OOXML")
	}
}
