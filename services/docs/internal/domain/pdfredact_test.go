// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de la censura real y del saneado de metadatos (ola P7).
//
// Aquí hay UN test que importa por encima de todos, y no es que la función
// devuelva algo: es que el texto censurado **ya no exista en el fichero**. Una
// censura que tapa sin borrar pasa cualquier comprobación razonable —el PDF abre,
// se ve el recuadro negro, la función no da error— y sigue entregando el texto a
// quien lo seleccione con el ratón. Por eso cada caso comprueba las DOS cosas:
//
//  1. que el extractor de texto ya no lo devuelve, y
//  2. que los BYTES no están en el fichero, que es lo que ve un editor hexadecimal.
//
// La segunda es la que de verdad distingue censurar de tapar: si el flujo viejo
// se copiara al documento nuevo y solo se dejara de apuntar, (1) pasaría y (2) no.
package domain

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// pdfConLineas escribe un PDF de una página con una línea por texto, y devuelve
// también la Y aproximada de cada una para poder marcarlas.
func pdfConLineas(t *testing.T, lineas ...string) *PDFDoc {
	t.Helper()
	w := pdfsimple.Nuevo()
	for _, l := range lineas {
		w.Texto(l, "F1", 12)
	}
	d, err := LeerPDF(w.Bytes())
	if err != nil {
		t.Fatalf("preparar el PDF: %v", err)
	}
	return d
}

// yDelTexto localiza la línea base de un fragmento por su texto.
func yDelTexto(t *testing.T, doc *PDFDoc, aguja string) float64 {
	t.Helper()
	frags, err := doc.TextoDePagina(0)
	if err != nil {
		t.Fatalf("TextoDePagina: %v", err)
	}
	for _, f := range frags {
		if strings.Contains(f.Texto, aguja) {
			return f.Y
		}
	}
	t.Fatalf("no encontré %q en la página", aguja)
	return 0
}

func TestRedactarBorraElTextoDeVerdad(t *testing.T) {
	const secreto = "NUMERO DE CUENTA ES1234567890"
	doc := pdfConLineas(t, "linea publica de arriba", secreto, "linea publica de abajo")

	y := yDelTexto(t, doc, "NUMERO")
	datos, err := RedactarPDF(doc, []AreaRedaccion{{
		Pagina: 0,
		X1:     0, Y1: y - 4,
		X2: 600, Y2: y + 12,
	}})
	if err != nil {
		t.Fatalf("RedactarPDF: %v", err)
	}

	// (1) El extractor ya no lo devuelve.
	d2, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("releer el PDF censurado: %v", err)
	}
	texto, err := d2.TextoPlano(0)
	if err != nil {
		t.Fatalf("TextoPlano: %v", err)
	}
	if strings.Contains(texto, "NUMERO") || strings.Contains(texto, "ES1234567890") {
		t.Errorf("el texto censurado sigue siendo extraíble: %q", texto)
	}

	// (2) Y los BYTES no están en el fichero. Esta es la que separa censurar de
	// tapar: con el flujo viejo copiado y solo desapuntado, (1) pasaría igual.
	if bytes.Contains(datos, []byte("ES1234567890")) {
		t.Error("el texto censurado sigue DENTRO del fichero: se tapó, no se borró")
	}

	// Y lo de alrededor sobrevive: censurar de más también es un fallo.
	if !strings.Contains(texto, "arriba") || !strings.Contains(texto, "abajo") {
		t.Errorf("se llevó por delante texto que no estaba marcado: %q", texto)
	}
}

func TestRedactarNoTocaLoQueEstaFueraDeLaZona(t *testing.T) {
	doc := pdfConLineas(t, "primera", "segunda", "tercera")
	y := yDelTexto(t, doc, "segunda")

	datos, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: y - 4, X2: 600, Y2: y + 12}})
	if err != nil {
		t.Fatalf("RedactarPDF: %v", err)
	}
	d2, _ := LeerPDF(datos)
	texto, _ := d2.TextoPlano(0)

	if strings.Contains(texto, "segunda") {
		t.Errorf("no se censuró la línea marcada: %q", texto)
	}
	for _, quiero := range []string{"primera", "tercera"} {
		if !strings.Contains(texto, quiero) {
			t.Errorf("desapareció %q, que estaba fuera de la zona: %q", quiero, texto)
		}
	}
}

func TestRedactarSoloAfectaALaPaginaMarcada(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("secreto en la una", "F1", 12)
	w.NuevaPagina()
	w.Texto("secreto en la dos", "F1", 12)
	doc, err := LeerPDF(w.Bytes())
	if err != nil {
		t.Fatalf("preparar: %v", err)
	}
	y := yDelTexto(t, doc, "una")

	datos, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: y - 4, X2: 600, Y2: y + 12}})
	if err != nil {
		t.Fatalf("RedactarPDF: %v", err)
	}
	d2, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	if d2.NumPaginas() != 2 {
		t.Fatalf("páginas = %d, quería 2", d2.NumPaginas())
	}
	if t0, _ := d2.TextoPlano(0); strings.Contains(t0, "una") {
		t.Errorf("la página 0 no se censuró: %q", t0)
	}
	if t1, _ := d2.TextoPlano(1); !strings.Contains(t1, "dos") {
		t.Errorf("la página 1 se censuró sin estar marcada: %q", t1)
	}
}

func TestRedactarRectanguloAlReves(t *testing.T) {
	// Dibujar de derecha a izquierda o de abajo arriba es la mitad de las veces.
	// Sin normalizar, la superficie sale negativa y no se censuraría NADA — con
	// el recuadro negro pintado igual, que es la peor combinación posible.
	doc := pdfConLineas(t, "arriba", "el dato sensible", "abajo")
	y := yDelTexto(t, doc, "sensible")

	datos, err := RedactarPDF(doc, []AreaRedaccion{{
		Pagina: 0,
		X1:     600, Y1: y + 12, // esquinas invertidas a propósito
		X2: 0, Y2: y - 4,
	}})
	if err != nil {
		t.Fatalf("RedactarPDF: %v", err)
	}
	d2, _ := LeerPDF(datos)
	if texto, _ := d2.TextoPlano(0); strings.Contains(texto, "sensible") {
		t.Errorf("un rectángulo con las esquinas invertidas no censuró: %q", texto)
	}
}

func TestRedactarSinZonasFalla(t *testing.T) {
	doc := pdfConLineas(t, "x")
	if _, err := RedactarPDF(doc, nil); err == nil {
		t.Error("censurar sin marcar ninguna zona debería fallar, no devolver el original")
	}
}

func TestRedactarZonaSinSuperficieFalla(t *testing.T) {
	// Es el caso más traicionero: un rectángulo de área cero pintaría el recuadro
	// negro y no borraría nada. Falla en vez de mentir.
	doc := pdfConLineas(t, "x")
	casos := []AreaRedaccion{
		{Pagina: 0, X1: 10, Y1: 10, X2: 10, Y2: 50}, // ancho cero
		{Pagina: 0, X1: 10, Y1: 10, X2: 50, Y2: 10}, // alto cero
		{Pagina: 0}, // todo a cero
	}
	for i, a := range casos {
		if _, err := RedactarPDF(doc, []AreaRedaccion{a}); err == nil {
			t.Errorf("caso %d: una zona sin superficie debería fallar", i)
		}
	}
}

func TestRedactarPaginaInexistenteFalla(t *testing.T) {
	doc := pdfConLineas(t, "x")
	_, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 7, X1: 0, Y1: 0, X2: 10, Y2: 10}})
	if err == nil {
		t.Fatal("marcar una página que no existe debería fallar")
	}
	if !strings.Contains(err.Error(), "no existe") {
		t.Errorf("mensaje = %q; debería decir que esa página no existe", err.Error())
	}
}

func TestRedactarDevuelveUnPDFQueSeAbre(t *testing.T) {
	// El criterio de este motor: lo que sale se vuelve a LEER. Un fichero que se
	// escribe sin error pero que ningún lector abre pasaría cualquier
	// comprobación sobre los bytes.
	doc := pdfConLineas(t, "uno", "dos")
	y := yDelTexto(t, doc, "uno")
	datos, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: y - 4, X2: 600, Y2: y + 12}})
	if err != nil {
		t.Fatalf("RedactarPDF: %v", err)
	}
	d2, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("el PDF censurado no se puede releer: %v", err)
	}
	if d2.NumPaginas() != 1 {
		t.Errorf("páginas = %d, quería 1", d2.NumPaginas())
	}
	p, err := d2.Pagina(0)
	if err != nil {
		t.Fatalf("Pagina(0): %v", err)
	}
	if p.Ancho <= 0 || p.Alto <= 0 {
		t.Errorf("la página censurada quedó sin tamaño: %.0f×%.0f", p.Ancho, p.Alto)
	}
}

// --- Saneado de metadatos ----------------------------------------------------

func TestSanearMetadatosQuitaLaProcedencia(t *testing.T) {
	// /Info con autor y aplicación de origen: lo que delata de dónde salió un
	// documento que se envía fuera.
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 200 200] >>`,
		`<< /Type /Page /Parent 2 0 R >>`,
		`<< /Author (Nombre Apellido) /Producer (AplicacionInterna 4.2) /Title (Borrador confidencial) >>`,
	}
	doc, err := LeerPDF(pdfMinimo(t, `<< /Size 5 /Root 1 0 R /Info 4 0 R >>`, objs))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}

	datos, err := SanearMetadatosPDF(doc)
	if err != nil {
		t.Fatalf("SanearMetadatosPDF: %v", err)
	}
	for _, rastro := range []string{"Nombre Apellido", "AplicacionInterna", "Borrador confidencial"} {
		if bytes.Contains(datos, []byte(rastro)) {
			t.Errorf("sigue en el fichero: %q", rastro)
		}
	}
	// Y el documento sigue siendo un PDF legible.
	if _, err := LeerPDF(datos); err != nil {
		t.Errorf("el PDF saneado no se puede releer: %v", err)
	}
}

func TestSanearNoBorraElContenido(t *testing.T) {
	// Sanear no es censurar: el texto del documento tiene que seguir ahí.
	doc := pdfConLineas(t, "esto es el contenido legítimo")
	datos, err := SanearMetadatosPDF(doc)
	if err != nil {
		t.Fatalf("SanearMetadatosPDF: %v", err)
	}
	d2, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	texto, _ := d2.TextoPlano(0)
	if !strings.Contains(texto, "contenido") {
		t.Errorf("sanear se llevó el contenido: %q", texto)
	}
}

func TestErrRedaccionNoGarantizadaEsDistinguible(t *testing.T) {
	// La interfaz tiene que poder distinguir «no se pudo garantizar» de un fallo
	// cualquiera, para ofrecer una salida (rasterizar, recortar) en vez de un
	// error genérico. Si el centinela deja de existir, esto no compila.
	if !errors.Is(ErrRedaccionNoGarantizada, ErrRedaccionNoGarantizada) {
		t.Error("el centinela no es comparable con errors.Is")
	}
	if ErrRedaccionNoGarantizada.Error() == "" {
		t.Error("el centinela no dice nada")
	}
}

// Lo que se quita se sustituye por lo que el operador hacía además de pintar
// (el salto de línea de «'»). Se escribía pegado al operador anterior —«TjT*»—,
// y ese operador desconocido se llevaba por delante el Tj de la línea de arriba.
func TestRedactarNoPegaElSustitutoAlOperadorAnterior(t *testing.T) {
	d, err := LeerPDF(pdfConContenido(t, `BT /F1 12 Tf 14 TL 10 80 Td (uno) Tj(secreto)'(dos)' ET`, helvetica, objHelvetica))
	if err != nil {
		t.Fatal(err)
	}
	// «secreto» va en la línea de y = 66; la caja de «dos» (y = 52) llega a 62,8.
	salida, err := RedactarPDF(d, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: 64, X2: 100, Y2: 70}})
	if err != nil {
		t.Fatal(err)
	}
	frags := fragmentosDeSalida(t, salida)
	if len(frags) != 2 || frags[0].Texto != "uno" || frags[1].Texto != "dos" || frags[1].Y != 52 {
		t.Fatalf("fragmentos = %+v, quería «uno» en 80 y «dos» en 52", frags)
	}
}

// Censurar quitaba el catálogo entero: con él se iban también el idioma, cómo
// se abre el documento y la numeración de sus páginas, que no pueden repetir lo
// tapado. Eso se conserva; el índice, que sí puede, no.
func TestRedactarConservaDelCatalogoLoQueNoLlevaTexto(t *testing.T) {
	contenido := "BT /F1 12 Tf 10 50 Td (hola) Tj ET"
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R /Lang (es-ES) /PageLayout /TwoColumnLeft ` +
			`/ViewerPreferences << /DisplayDocTitle true >> /PageLabels << /Nums [0 << /S /r >>] >> ` +
			`/OpenAction [3 0 R /Fit] /Outlines 5 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Contents 4 0 R ` +
			`/Resources << /Font << /F1 7 0 R >> >> >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(contenido), contenido),
		`<< /Type /Outlines /First 6 0 R /Last 6 0 R /Count 1 >>`,
		`<< /Title (Expediente secreto) /Parent 5 0 R /Dest [3 0 R /Fit] >>`,
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>`,
	}
	doc, err := LeerPDF(pdfMinimo(t, `<< /Size 8 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatal(err)
	}
	datos, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: 40, X2: 200, Y2: 70}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(datos, []byte("Expediente secreto")) {
		t.Error("el índice viajó con la censura")
	}
	d2, err := LeerPDF(datos)
	if err != nil {
		t.Fatal(err)
	}
	raiz, _ := d2.resolver(d2.trailer["Root"]).(pdfDict)
	for _, k := range []pdfName{"Lang", "PageLayout", "ViewerPreferences", "PageLabels", "OpenAction"} {
		if raiz[k] == nil {
			t.Errorf("el catálogo censurado perdió /%s", k)
		}
	}
	if raiz["Outlines"] != nil {
		t.Error("el catálogo censurado conserva /Outlines")
	}
}

// Decisión del dueño, «conservar los de fuera»: viajan los campos cuyos widgets
// quedan todos fuera de la zona; el que toca la zona se quita entero, valor
// incluido; el que depende de uno quitado (un total calculado) también; y el XFA
// no viaja. Antes se quitaba el /AcroForm y los campos de fuera dejaban de
// verse, mientras el DNI quitado seguía en el fichero por los /Kids de su padre.
func TestRedactarConservaLosCamposDeFuera(t *testing.T) {
	xfa := "<xdp>SecretoXFA</xdp>"
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R 6 0 R 7 0 R 9 0 R] /DA (/Helv 0 Tf 0 g) ` +
			`/DR << /Font << /Helv 12 0 R >> >> /XFA 8 0 R >> >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Contents 4 0 R /Annots [5 0 R 6 0 R 7 0 R 10 0 R 11 0 R] >>`,
		"<< /Length 0 >>\nstream\n\nendstream",
		`<< /Type /Annot /Subtype /Widget /FT /Tx /T (fuera) /V (Visible) /Rect [20 200 120 220] >>`,
		`<< /Type /Annot /Subtype /Widget /FT /Tx /T (dentro) /V (Secreto) /Rect [20 100 120 120] >>`,
		`<< /Type /Annot /Subtype /Widget /FT /Tx /T (total) /V (Calculado) /Rect [20 50 120 70] ` +
			`/AA << /C << /S /JavaScript /JS (AFSimple_Calculate\("SUM", new Array \("dentro"\)\);) >> >> >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(xfa), xfa),
		`<< /T (persona) /Kids [10 0 R 11 0 R] >>`,
		`<< /Type /Annot /Subtype /Widget /FT /Tx /T (nombre) /V (Ana) /Parent 9 0 R /Rect [150 200 250 220] >>`,
		`<< /Type /Annot /Subtype /Widget /FT /Tx /T (dni) /V (12345678Z) /Parent 9 0 R /Rect [150 100 250 120] >>`,
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>`,
	}
	d, err := LeerPDF(pdfMinimo(t, `<< /Size 13 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatal(err)
	}
	datos, err := RedactarPDF(d, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: 95, X2: 300, Y2: 125}})
	if err != nil {
		t.Fatal(err)
	}
	for _, fuga := range []string{"Secreto", "12345678Z", "Calculado", "SecretoXFA"} {
		if bytes.Contains(datos, []byte(fuga)) {
			t.Errorf("sigue en el fichero: %q", fuga)
		}
	}
	d2, err := LeerPDF(datos)
	if err != nil {
		t.Fatal(err)
	}
	campos := d2.CamposDelFormulario()
	nombres := make([]string, 0, len(campos))
	for _, c := range campos {
		nombres = append(nombres, c.Nombre+"="+c.Valor)
	}
	if fmt.Sprint(nombres) != "[fuera=Visible persona.nombre=Ana]" {
		t.Errorf("campos de la salida = %v, quería [fuera=Visible persona.nombre=Ana]", nombres)
	}
}

// El hueco de lo censurado no pasa del tope de un real de PDF/A-1 (32767): se
// parte en varios números del mismo TJ, que suman lo mismo, y lo que sigue en la
// línea se queda donde estaba.
func TestRedactarHuecoLargoNoPasaDelTopeDePDFA(t *testing.T) {
	largo := strings.Repeat("W", 80) // 80 × 944 milésimas: 75 520
	anchos := make([]string, 0, 24)
	for c := 'W'; c <= 'n'; c++ {
		anchos = append(anchos, strconv.Itoa(int(anchosHelvetica[c-32])))
	}
	fuente := `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /FirstChar 87 /LastChar 110 /Widths [` +
		strings.Join(anchos, " ") + `] >>`
	d, err := LeerPDF(pdfConContenido(t, `BT /F1 1 Tf 10 50 Td (`+largo+`) Tj (fin) Tj ET`, helvetica, fuente))
	if err != nil {
		t.Fatal(err)
	}
	salida, err := RedactarPDF(d, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: 49, X2: 70, Y2: 52}})
	if err != nil {
		t.Fatal(err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatal(err)
	}
	p, err := d2.Pagina(0)
	if err != nil {
		t.Fatal(err)
	}
	contenido, err := d2.contenidoDePagina(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`-?\d+(\.\d+)?`).FindAllString(string(contenido), -1) {
		if v, _ := strconv.ParseFloat(m, 64); math.Abs(v) > 32767 {
			t.Fatalf("número fuera del tope de PDF/A-1: %s en %q", m, contenido)
		}
	}
	frags := fragmentosDeSalida(t, salida)
	if len(frags) != 1 || frags[0].Texto != "fin" || math.Abs(frags[0].X-85.52) > 0.01 {
		t.Fatalf("fragmentos = %+v, quería «fin» en x = 85,52", frags)
	}
}
