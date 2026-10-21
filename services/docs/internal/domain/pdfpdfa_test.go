// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// pdfADePrueba es un PDF/A de una página con texto, que declara `parte` y
// `conformidad` en su XMP y lleva una intención de salida de `componentes`
// (0: ninguna) y, si `etiquetado`, un árbol de estructura.
func pdfADePrueba(t *testing.T, parte, conformidad string, componentes int, etiquetado bool) *PDFDoc {
	t.Helper()
	xmp := `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
		`<rdf:Description rdf:about="" xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/">` +
		`<pdfaid:part>` + parte + `</pdfaid:part><pdfaid:conformance>` + conformidad + `</pdfaid:conformance>` +
		`</rdf:Description></rdf:RDF></x:xmpmeta>`
	contenido := "BT /F1 12 Tf 10 50 Td (hola) Tj ET"
	catalogo := `<< /Type /Catalog /Pages 2 0 R /Metadata 5 0 R`
	if componentes > 0 {
		catalogo += ` /OutputIntents [<< /Type /OutputIntent /S /GTS_PDFA1 /OutputConditionIdentifier (x) /DestOutputProfile 6 0 R >>]`
	}
	if etiquetado {
		catalogo += ` /StructTreeRoot 7 0 R /MarkInfo << /Marked true >>`
	}
	objs := []string{
		catalogo + ` >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Contents 4 0 R /Resources << >> >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(contenido), contenido),
		fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream", len(xmp), xmp),
		fmt.Sprintf("<< /N %d /Length 0 >>\nstream\n\nendstream", max(componentes, 1)),
		`<< /Type /StructTreeRoot >>`,
	}
	d, err := LeerPDF(pdfMinimo(t, `<< /Size 8 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func releerPDFA(t *testing.T, salida []byte, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatal(err)
	}
	return d.DeclaraPDFA()
}

// Decisión del dueño: al censurar un PDF/A de nivel «a», que pierde la
// estructura etiquetada, la marca baja a «b» de la misma parte.
func TestPDFANivelABajaABAlCensurar(t *testing.T) {
	d := pdfADePrueba(t, "2", "A", 3, true)
	if got := d.DeclaraPDFA(); got != "2a" {
		t.Fatalf("la entrada declara %q, quería 2a", got)
	}
	salida, err := RedactarPDF(d, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: 40, X2: 200, Y2: 70}})
	if got := releerPDFA(t, salida, err); got != "2b" {
		t.Errorf("la salida censurada declara %q, quería 2b", got)
	}
}

// Una anotación nuestra dibuja en RGB: con una intención de salida CMYK ya no
// cumple (6.2.4.3), y la marca se quita; con una RGB, se conserva.
func TestPDFASeQuitaSiLaEdicionNoCumple(t *testing.T) {
	anots := []Anotacion{{Tipo: AnotRectangulo, Rect: [4]float64{10, 10, 50, 50}, Color: [3]float64{1, 0, 0}, Grosor: 1}}
	salida, err := AnadirAnotacionesPDF(pdfADePrueba(t, "2", "B", 4, false), anots)
	if got := releerPDFA(t, salida, err); got != "" {
		t.Errorf("con intención CMYK la salida sigue declarando %q", got)
	}
	salida, err = AnadirAnotacionesPDF(pdfADePrueba(t, "2", "B", 3, false), anots)
	if got := releerPDFA(t, salida, err); got != "2b" {
		t.Errorf("con intención RGB la salida declara %q, quería 2b", got)
	}
}

// PDF/A-4 exige la cabecera de PDF 2.0 (6.1.2); salía %PDF-1.7 y veraPDF la
// rechazaba en todas las salidas reescritas.
func TestPDFA4SaleConCabeceraDePDF20(t *testing.T) {
	salida, err := RotarPaginasPDF(pdfADePrueba(t, "4", "", 3, false), map[int]int{0: 90})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(salida, []byte("%PDF-2.0")) {
		t.Errorf("cabecera %q, quería %%PDF-2.0", salida[:8])
	}
	if got := releerPDFA(t, salida, nil); got != "4" {
		t.Errorf("declara %q, quería 4", got)
	}
}

// El texto libre y el sello se escriben con la Liberation Sans incrustada: un
// PDF/A anotado conserva la marca. Con la Helvetica sin incrustar de antes la
// perdía siempre (184 de 184 PDF/A del corpus). La Helvetica, que sigue siendo
// la de las apariencias que se dibujan al vuelo, la comprobación la ve sin
// incrustar.
func TestPDFATextoLibreConservaLaMarca(t *testing.T) {
	anots := []Anotacion{
		{Tipo: AnotTextoLibre, Rect: [4]float64{10, 10, 150, 30}, Texto: "hola Ωж", Color: [3]float64{0, 0, 0}, Opacidad: 1, Tamano: 12},
		{Tipo: AnotSello, Rect: [4]float64{10, 40, 150, 70}, Etiqueta: "APROBADO", Color: [3]float64{1, 0, 0}, Opacidad: 1},
	}
	salida, err := AnadirAnotacionesPDF(pdfADePrueba(t, "2", "B", 3, false), anots)
	if got := releerPDFA(t, salida, err); got != "2b" {
		t.Errorf("con la letra incrustada la salida declara %q, quería 2b", got)
	}
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatal(err)
	}
	texto, _ := d.TextoPlano(0)
	if !strings.Contains(texto, "hola Ωж") || !strings.Contains(texto, "APROBADO") {
		t.Errorf("el texto de las anotaciones no se lee: %q", texto)
	}
	if NuevoEnsambladorPDF().fuenteIncrustada(fuenteDeAnotacion()["SytueAnotF"].(pdfDict)) {
		t.Error("la Helvetica del visor no está incrustada")
	}
}

// Un PDF/A FIRMADO se anota anexando, sin tocar el catálogo: la comprobación no
// pasaba por ahí y la salida seguía declarando PDF/A con una fuente sin
// incrustar (7 salidas de BOE firmados en PDF/A-1a que veraPDF rechazaba). Ahora
// se reescribe solo el flujo de metadatos, sin la marca, y la firma sigue
// íntegra; si lo anexado cumple, la marca se queda.
func TestPDFAFirmadoAnexadoQuitaLaMarcaSiNoCumple(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	firmado, err := FirmarIncrementalPDF(pdfADePrueba(t, "2", "B", 3, false),
		OpcionesFirma{CertificadoPEM: certPEM, ClavePEM: clavePEM, Nombre: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	anotar := func(a Anotacion) []byte {
		t.Helper()
		d, err := LeerPDF(firmado)
		if err != nil {
			t.Fatal(err)
		}
		salida, err := AnadirAnotacionesPDF(d, []Anotacion{a})
		if err != nil {
			t.Fatal(err)
		}
		conservaLaFirma(t, "anotar", firmado, salida)
		return salida
	}
	libre := anotar(Anotacion{Tipo: AnotTextoLibre, Rect: [4]float64{10, 10, 150, 30}, Texto: "hola", Opacidad: 1, Tamano: 12})
	if got := releerPDFA(t, libre, nil); got != "2b" {
		t.Errorf("texto libre con la letra incrustada: la salida firmada declara %q, quería 2b", got)
	}
	// La transparencia no cabe en PDF/A-1: anexada a un firmado, quita la marca.
	firmado1, err := FirmarIncrementalPDF(pdfADePrueba(t, "1", "B", 3, false),
		OpcionesFirma{CertificadoPEM: certPEM, ClavePEM: clavePEM, Nombre: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	d1, err := LeerPDF(firmado1)
	if err != nil {
		t.Fatal(err)
	}
	velado, err := AnadirAnotacionesPDF(d1, []Anotacion{{Tipo: AnotRectangulo, Rect: [4]float64{10, 10, 50, 50}, Color: [3]float64{1, 0, 0}, Opacidad: 0.5, Grosor: 1}})
	if got := releerPDFA(t, velado, err); got != "" {
		t.Errorf("transparencia en PDF/A-1: la salida firmada sigue declarando %q", got)
	}
	rect := anotar(Anotacion{Tipo: AnotRectangulo, Rect: [4]float64{10, 10, 50, 50}, Color: [3]float64{1, 0, 0}, Opacidad: 1, Grosor: 1})
	if got := releerPDFA(t, rect, nil); got != "2b" {
		t.Errorf("rectángulo que cumple: la salida firmada declara %q, quería 2b", got)
	}
}

// Una apariencia nueva puede usar una fuente del original —la de /DR que usa
// un texto libre— que el original no incrustaba porque no la usaba. Estaba
// copiada, así que no se miraba, y la marca se quedaba (issue16553 de pdf.js,
// 6.2.11.4.1).
func TestPDFAMiraLaFuenteEnlazadaDesdeLoNuevo(t *testing.T) {
	origen := pdfADePrueba(t, "3", "B", 3, false)
	dec, _ := origen.declaraPDFA()
	e := NuevoEnsambladorPDF()
	e.cuerpos = []objetoSalida{
		{valor: pdfDict{"Type": pdfName("Font"), "Subtype": pdfName("Type1"), "BaseFont": pdfName("Helvetica")}},
		{
			valor: pdfDict{
				"Type": pdfName("XObject"), "Subtype": pdfName("Form"),
				"Resources": pdfDict{"Font": pdfDict{"F1": pdfRef{Num: 1}}},
			},
			flujo: []byte("BT /F1 12 Tf (x) Tj ET"), esFlujo: true,
		},
	}
	e.copiados = map[origenObj]int{{doc: origen, num: 9}: 1}
	if _, motivo := e.incumplePDFA(origen, dec, pdfDict{}); motivo == "" {
		t.Error("la fuente sin incrustar a la que apunta la apariencia nueva no se vio")
	}
}

// Reescribir un PDF/A firmado (con permiso) deja la firma sin cubrir el fichero,
// y desde la parte 2 eso incumple (6.4.3): la marca se quita.
func TestPDFAFirmadoReescritoQuitaLaMarca(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	firmado, err := FirmarIncrementalPDF(pdfADePrueba(t, "2", "B", 3, false),
		OpcionesFirma{CertificadoPEM: certPEM, ClavePEM: clavePEM, Nombre: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := LeerPDF(firmado)
	if err != nil {
		t.Fatal(err)
	}
	d.PermitirInvalidarFirmas()
	salida, err := RotarPaginasPDF(d, map[int]int{0: 90})
	if got := releerPDFA(t, salida, err); got != "" {
		t.Errorf("girar un PDF/A firmado invalidando la firma: la salida sigue declarando %q", got)
	}
}

// Los recuadros negros de la censura se pintaban en RGB («0 0 0 rg»): con una
// intención de salida CMYK eso incumple (6.2.4.3) y censurar quitaba la marca.
// En gris de dispositivo es el mismo negro y la marca se queda.
func TestPDFACensurarConIntencionCMYKConservaLaMarca(t *testing.T) {
	salida, err := RedactarPDF(pdfADePrueba(t, "2", "B", 4, false), []AreaRedaccion{{Pagina: 0, X1: 0, Y1: 40, X2: 200, Y2: 70}})
	if got := releerPDFA(t, salida, err); got != "2b" {
		t.Errorf("censurar con intención CMYK: la salida declara %q, quería 2b", got)
	}
}
