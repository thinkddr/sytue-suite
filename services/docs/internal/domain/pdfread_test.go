// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del lector de PDF. La mayoría van de ida y vuelta: se genera un PDF con
// el escritor de `export_pdf.go` y se lee con `pdfread.go`, así que no hay
// ficheros binarios en el repo ni fixtures que nadie sepa de dónde salieron.
//
// El resto son PDF mínimos escritos a mano en el propio test para llegar a los
// casos que el escritor de la casa NO produce —flujo xref comprimido, objetos
// dentro de un ObjStm, MediaBox heredado, páginas rotadas— que es justo donde un
// lector se rompe con ficheros de fuera.
package domain

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// --- Ida y vuelta con el escritor de la casa ---------------------------------

func TestLeerPDFIdaYVueltaConElEscritor(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("Hola mundo", "F1", 12)
	w.Texto("Segunda linea", "F1", 12)
	datos := w.Bytes()

	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if d.NumPaginas() != 1 {
		t.Fatalf("NumPaginas = %d, quería 1", d.NumPaginas())
	}
	p, err := d.Pagina(0)
	if err != nil {
		t.Fatalf("Pagina(0): %v", err)
	}
	// El escritor emite A4 en puntos.
	if p.Ancho < 500 || p.Alto < 700 {
		t.Errorf("tamaño = %.0f×%.0f, esperaba algo de tamaño A4", p.Ancho, p.Alto)
	}

	texto, err := d.TextoPlano(0)
	if err != nil {
		t.Fatalf("TextoPlano: %v", err)
	}
	if !strings.Contains(texto, "Hola mundo") {
		t.Errorf("el texto extraído no contiene «Hola mundo»: %q", texto)
	}
	if !strings.Contains(texto, "Segunda linea") {
		t.Errorf("el texto extraído no contiene «Segunda linea»: %q", texto)
	}
}

func TestLeerPDFVariasPaginas(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("pagina uno", "F1", 12)
	w.NuevaPagina()
	w.Texto("pagina dos", "F1", 12)
	w.NuevaPagina()
	w.Texto("pagina tres", "F1", 12)
	d, err := LeerPDF(w.Bytes())
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if d.NumPaginas() != 3 {
		t.Fatalf("NumPaginas = %d, quería 3", d.NumPaginas())
	}
	for i, quiero := range []string{"pagina uno", "pagina dos", "pagina tres"} {
		txt, err := d.TextoPlano(i)
		if err != nil {
			t.Fatalf("TextoPlano(%d): %v", i, err)
		}
		if !strings.Contains(txt, quiero) {
			t.Errorf("página %d = %q, quería que contuviera %q", i, txt, quiero)
		}
	}
}

func TestLeerPDFPaginaFueraDeRango(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("x", "F1", 12)
	d, err := LeerPDF(w.Bytes())
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if _, err := d.Pagina(7); err == nil {
		t.Error("Pagina(7) sobre un PDF de una página debería fallar")
	}
	if _, err := d.Pagina(-1); err == nil {
		t.Error("Pagina(-1) debería fallar")
	}
}

// --- Entradas que no son PDF o están rotas -----------------------------------

func TestLeerPDFRechazaLoQueNoEsPDF(t *testing.T) {
	casos := map[string][]byte{
		"vacío":        {},
		"texto suelto": []byte("esto no es un pdf, ni de lejos"),
		"png":          {0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13},
	}
	for nombre, datos := range casos {
		if _, err := LeerPDF(datos); !errors.Is(err, ErrPDFNoEsPDF) {
			t.Errorf("%s: err = %v, quería ErrPDFNoEsPDF", nombre, err)
		}
	}
}

func TestLeerPDFToleraBasuraDelanteDeLaCabecera(t *testing.T) {
	// Un PDF servido tras cabeceras sueltas sigue siendo legible; la
	// especificación pide tolerarlo dentro del primer kilobyte.
	w := pdfsimple.Nuevo()
	w.Texto("con basura delante", "F1", 12)
	datos := append([]byte("basura previa\r\n"), w.Bytes()...)

	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("LeerPDF con basura delante: %v", err)
	}
	txt, _ := d.TextoPlano(0)
	if !strings.Contains(txt, "con basura delante") {
		t.Errorf("texto = %q", txt)
	}
}

func TestLeerPDFSinStartxref(t *testing.T) {
	datos := []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF")
	if _, err := LeerPDF(datos); !errors.Is(err, ErrPDFCorrupto) {
		t.Errorf("err = %v, quería ErrPDFCorrupto", err)
	}
}

func TestLeerPDFCifradoSeRechazaConClaridad(t *testing.T) {
	// Un PDF con /Encrypt debe fallar con un error propio y no devolver texto
	// basura: la UI necesita poder decir «pide la contraseña», no «vacío».
	datos := pdfMinimo(t, `<< /Size 5 /Root 1 0 R /Encrypt 4 0 R >>`, nil)
	_, err := LeerPDF(datos)
	if !errors.Is(err, ErrPDFCifrado) {
		t.Errorf("err = %v, quería ErrPDFCifrado", err)
	}
}

// --- Casos que el escritor de la casa no genera ------------------------------

func TestLeerPDFMediaBoxHeredadoDelNodoPages(t *testing.T) {
	// Lo normal en PDF real: el MediaBox se declara UNA vez en /Pages y las
	// páginas no lo repiten. Sin herencia, todas medirían el tamaño por defecto.
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 200 400] >>`,
		`<< /Type /Page /Parent 2 0 R >>`,
	}
	d, err := LeerPDF(pdfMinimo(t, `<< /Size 4 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	p, _ := d.Pagina(0)
	if p.Ancho != 200 || p.Alto != 400 {
		t.Errorf("tamaño = %.0f×%.0f, quería 200×400 heredado de /Pages", p.Ancho, p.Alto)
	}
}

func TestLeerPDFPaginaRotadaIntercambiaLados(t *testing.T) {
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 400] /Rotate 90 >>`,
	}
	d, err := LeerPDF(pdfMinimo(t, `<< /Size 4 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	p, _ := d.Pagina(0)
	if p.Rotacion != 90 {
		t.Errorf("Rotacion = %d, quería 90", p.Rotacion)
	}
	// Rotada un cuarto de vuelta, lo que mide 200 de ancho se ve de 400.
	if p.Ancho != 400 || p.Alto != 200 {
		t.Errorf("tamaño = %.0f×%.0f, quería 400×200 al rotar 90°", p.Ancho, p.Alto)
	}
}

func TestLeerPDFMediaBoxConEsquinasAlReves(t *testing.T) {
	// [x1 y1 x0 y0] es válido: la caja se define por dos esquinas opuestas, sin
	// exigir orden. Restar sin más daría medidas negativas.
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [200 400 0 0] >>`,
	}
	d, err := LeerPDF(pdfMinimo(t, `<< /Size 4 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	p, _ := d.Pagina(0)
	if p.Ancho != 200 || p.Alto != 400 {
		t.Errorf("tamaño = %.0f×%.0f, quería 200×400", p.Ancho, p.Alto)
	}
}

func TestLeerPDFArbolDePaginasAnidado(t *testing.T) {
	// Un PDF grande agrupa las páginas en subárboles; aplanarlo mal deja páginas
	// fuera o las devuelve desordenadas.
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R 6 0 R] /Count 3 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Pages /Parent 2 0 R /Kids [4 0 R 5 0 R] /Count 2 >>`,
		`<< /Type /Page /Parent 3 0 R >>`,
		`<< /Type /Page /Parent 3 0 R >>`,
		`<< /Type /Page /Parent 2 0 R >>`,
	}
	d, err := LeerPDF(pdfMinimo(t, `<< /Size 7 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if d.NumPaginas() != 3 {
		t.Errorf("NumPaginas = %d, quería 3 (dos en el subárbol y una suelta)", d.NumPaginas())
	}
}

// --- Cadenas y codificación --------------------------------------------------

func TestDecodificarTextoPDF(t *testing.T) {
	casos := []struct {
		nombre string
		in     pdfString
		quiero string
	}{
		{"ascii", pdfString("Hola"), "Hola"},
		{"latin1 con acento", pdfString{'a', 0xF1, 'o'}, "año"},
		{"comilla tipográfica de WinAnsi", pdfString{0x93, 'a', 0x94}, "“a”"},
		{"euro en 0x80", pdfString{0x80}, "€"},
		{"utf-16 con BOM", pdfString{0xFE, 0xFF, 0x00, 0x48, 0x00, 0x69}, "Hi"},
	}
	for _, c := range casos {
		if got := decodificarTextoPDF(c.in); got != c.quiero {
			t.Errorf("%s: %q, quería %q", c.nombre, got, c.quiero)
		}
	}
}

func TestCadenaLiteralConEscapes(t *testing.T) {
	d := &PDFDoc{}
	casos := map[string]string{
		`(sencilla)`:             "sencilla",
		`(con \(parentesis\))`:   "con (parentesis)",
		`(anidados (dentro) ya)`: "anidados (dentro) ya",
		`(salto\nlinea)`:         "salto\nlinea",
		`(octal \101\102)`:       "octal AB",
		`(barra \\ sola)`:        `barra \ sola`,
	}
	for entrada, quiero := range casos {
		lx := &pdfLexer{buf: []byte(entrada)}
		o, err := d.parsearObjeto(lx)
		if err != nil {
			t.Errorf("%s: %v", entrada, err)
			continue
		}
		s, ok := o.(pdfString)
		if !ok {
			t.Errorf("%s: no salió una cadena sino %T", entrada, o)
			continue
		}
		if string(s) != quiero {
			t.Errorf("%s = %q, quería %q", entrada, s, quiero)
		}
	}
}

func TestNombreConEscapeAlmohadilla(t *testing.T) {
	d := &PDFDoc{}
	lx := &pdfLexer{buf: []byte(`/Con#20Espacio`)}
	o, _ := d.parsearObjeto(lx)
	if n, ok := o.(pdfName); !ok || string(n) != "Con Espacio" {
		t.Errorf("nombre = %v (%T), quería «Con Espacio»", o, o)
	}
}

func TestReferenciaIndirectaNoSeConfundeConDosEnteros(t *testing.T) {
	d := &PDFDoc{}
	lx := &pdfLexer{buf: []byte(`12 0 R`)}
	o, _ := d.parsearObjeto(lx)
	ref, ok := o.(pdfRef)
	if !ok {
		t.Fatalf("no se reconoció la referencia: %T", o)
	}
	if ref.Num != 12 || ref.Gen != 0 {
		t.Errorf("ref = %+v, quería {12 0}", ref)
	}
	// Y dos enteros sueltos NO son una referencia.
	lx2 := &pdfLexer{buf: []byte(`12 0`)}
	if o2, _ := d.parsearObjeto(lx2); o2 != any(int64(12)) {
		t.Errorf("«12 0» dio %v (%T), quería el entero 12", o2, o2)
	}
}

// --- Extracción de texto -----------------------------------------------------

// helvetica es el recurso de fuente de los tests de texto: una de las 14
// estándar, sin incrustar, con sus anchos de siempre.
const (
	helvetica    = `/Resources << /Font << /F1 5 0 R >> >>`
	objHelvetica = `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>`
)

// fragmentosDe abre el PDF de una página con ese contenido y devuelve sus fragmentos.
func fragmentosDe(t *testing.T, contenido, extrasPagina string, objsExtra ...string) []PDFFragmento {
	t.Helper()
	d, err := LeerPDF(pdfConContenido(t, contenido, extrasPagina, objsExtra...))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	frags, err := d.TextoDePagina(0)
	if err != nil {
		t.Fatalf("TextoDePagina: %v", err)
	}
	return frags
}

func TestExtraerTextoOperadorTJConSeparacion(t *testing.T) {
	// Un ajuste de -300 milésimas entre palabras es un espacio; -10 no lo es.
	frags := fragmentosDe(t, `BT /F1 12 Tf 10 700 Td [(Hola)-300(mundo)-10(ya)] TJ ET`, helvetica, objHelvetica)
	if len(frags) != 1 {
		t.Fatalf("fragmentos = %d, quería 1: %+v", len(frags), frags)
	}
	if frags[0].Texto != "Hola mundoya" {
		t.Errorf("texto = %q, quería «Hola mundoya»", frags[0].Texto)
	}
	if frags[0].X != 10 || frags[0].Y != 700 {
		t.Errorf("posición = (%.0f, %.0f), quería (10, 700)", frags[0].X, frags[0].Y)
	}
}

func TestExtraerTextoTmFijaLaPosicion(t *testing.T) {
	frags := fragmentosDe(t, `BT /F1 10 Tf 1 0 0 1 55 66 Tm (situado) Tj ET`, helvetica, objHelvetica)
	if len(frags) != 1 {
		t.Fatalf("fragmentos = %d", len(frags))
	}
	if frags[0].X != 55 || frags[0].Y != 66 {
		t.Errorf("posición = (%.0f, %.0f), quería (55, 66)", frags[0].X, frags[0].Y)
	}
	if frags[0].Tamano != 10 {
		t.Errorf("tamaño = %.0f, quería 10", frags[0].Tamano)
	}
}

func TestExtraerTextoLineasSeparanEnTextoPlano(t *testing.T) {
	frags := fragmentosDe(t, `BT /F1 12 Tf 10 700 Td (primera) Tj 0 -20 Td (segunda) Tj ET`, helvetica, objHelvetica)
	if len(frags) != 2 {
		t.Fatalf("fragmentos = %d, quería 2", len(frags))
	}
	if frags[1].Y != 680 {
		t.Errorf("la segunda línea está en Y=%.0f, quería 680", frags[1].Y)
	}
}

// Una palabra pintada en dos Tj seguidos —Word y Chrome lo hacen— es UN
// fragmento: el buscador de la web busca dentro de cada uno.
func TestExtraerTextoJuntaLosTrozosDeUnaPalabra(t *testing.T) {
	frags := fragmentosDe(t, `BT /F1 12 Tf 10 50 Td (con) Tj (trato) Tj ET`, helvetica, objHelvetica)
	if len(frags) != 1 || frags[0].Texto != "contrato" {
		t.Fatalf("fragmentos = %+v, quería uno con «contrato»", frags)
	}
}

// Un rótulo que separa dos palabras con espaciado entre letras (Tc) DENTRO de
// una cadena —«(SS) Tj» con 8,6 Tc en pdf.js issue20513— lleva ahí un espacio.
// Un Tc pequeño, como el de un texto apretado, no. Antes salía «PARTSSOLUTION».
func TestExtraerTextoEspacioPorTcDentroDeLaCadena(t *testing.T) {
	frags := fragmentosDe(t, `BT /F1 50 Tf 10 50 Td -1.5 Tc (PART) Tj 8.6 Tc 120 0 Td (SS) Tj 0 Tc 75 0 Td (OLUTION) Tj ET`, helvetica, objHelvetica)
	if len(frags) != 1 || frags[0].Texto != "PARTS SOLUTION" {
		t.Fatalf("fragmentos = %+v, quería uno con «PARTS SOLUTION»", frags)
	}
}

// En chino el espaciado entre letras no separa palabras: «中文» con 0,227 em de
// Tc (pdf.js issue19182) sigue junto, como lo deja Poppler.
func TestExtraerTextoTcNoPartePalabrasChinas(t *testing.T) {
	objs := objsFuenteCID()
	objs[2] = strings.NewReplacer("<0048>", "<4E2D>", "<006F>", "<6587>", "<006C>", "<59D3>").Replace(objs[2])
	frags := fragmentosDe(t, `BT /F1 1 Tf 0.227 Tc 9 0 0 9 49 721 Tm <0011001200130014> Tj ET`, helvetica, objs...)
	if len(frags) != 1 || frags[0].Texto != "中文\u59D3\u59D4" {
		t.Fatalf("fragmentos = %+v, quería uno con «中文姓…» sin espacios", frags)
	}
}

// Escritura vertical (Identity-V, §9.7.4.3): cada glifo BAJA. Con −40 Tc las
// letras quedan separadas por más de un em y son palabras sueltas, como en
// pdf.js issue20930; con 0 Tc se juntan en una. Antes se tomaban en horizontal.
func TestExtraerTextoEscrituraVertical(t *testing.T) {
	objs := objsFuenteCID()
	objs[0] = strings.Replace(objs[0], "/Identity-H", "/Identity-V", 1)
	frags := fragmentosDe(t, `BT /F1 24 Tf -40 Tc 1 0 0 1 100 340 Tm <0011001200130014> Tj ET`, helvetica, objs...)
	if len(frags) != 1 || frags[0].Texto != "H o l m" {
		t.Fatalf("con −40 Tc: %+v, quería uno con «H o l m»", frags)
	}
	frags = fragmentosDe(t, `BT /F1 24 Tf 1 0 0 1 100 340 Tm <00110012> Tj <00130014> Tj ET`, helvetica, objs...)
	if len(frags) != 1 || frags[0].Texto != "Holm" {
		t.Fatalf("seguidas: %+v, quería uno con «Holm»", frags)
	}
	frags = fragmentosDe(t, `BT /F1 24 Tf 1 0 0 1 100 340 Tm <00110012> Tj 1 0 0 1 100 300 Tm <00130014> Tj ET`, helvetica, objs...)
	if len(frags) != 1 || frags[0].Texto != "Holm" {
		t.Fatalf("la segunda cadena sigue en la misma columna, 40 más abajo: %+v", frags)
	}
}

// objsFuenteCID son los objetos 5, 6 y 7 de una fuente compuesta Identity-H con
// /ToUnicode, como la del BOE: los bytes de la cadena son CID de dos bytes y
// solo la CMap dice qué letra es cada uno. Lleva H, o, l y m (0011-0014).
func objsFuenteCID() []string {
	cmap := `/CIDInit /ProcSet findresource begin 12 dict begin begincmap
/CMapName /Prueba-UCMap def /CMapType 2 def
1 begincodespacerange <0003> <00ce> endcodespacerange
2 beginbfchar <0011> <0048> <0012> <006F> endbfchar
1 beginbfrange <0013> <0014> <006C> endbfrange
endcmap CMapName currentdict /CMap defineresource pop end end`
	return []string{
		`<< /Type /Font /Subtype /Type0 /BaseFont /ABCDEF+Prueba /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>`,
		`<< /Type /Font /Subtype /CIDFontType2 /BaseFont /ABCDEF+Prueba /CIDToGIDMap /Identity ` +
			`/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 500 >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap),
	}
}

// Antes salían los códigos en crudo («\x00\x11\x00\x12…»).
func TestExtraerTextoDeFuenteCIDConToUnicode(t *testing.T) {
	frags := fragmentosDe(t, `BT /F1 12 Tf 10 50 Td <0011001200130014> Tj ET`, helvetica, objsFuenteCID()...)
	if len(frags) != 1 || frags[0].Texto != "Holm" {
		t.Fatalf("fragmentos = %+v, quería uno con «Holm»", frags)
	}
}

// El texto dentro de un XObject de formulario —membretes, pies, sellos— es
// texto de la página. El extractor de antes no seguía `Do`.
func TestExtraerTextoDentroDeUnFormulario(t *testing.T) {
	form := `BT /F1 12 Tf 5 5 Td (membrete) Tj ET`
	frags := fragmentosDe(t, `q 1 0 0 1 20 30 cm /X1 Do Q`,
		`/Resources << /XObject << /X1 5 0 R >> >>`,
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Resources << /Font << /F1 6 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
		objHelvetica)
	if len(frags) != 1 || frags[0].Texto != "membrete" {
		t.Fatalf("fragmentos = %+v, quería «membrete»", frags)
	}
	// La `cm` de fuera cuenta: el texto está en (25, 35) de la página.
	if frags[0].X != 25 || frags[0].Y != 35 {
		t.Errorf("posición = (%.0f, %.0f), quería (25, 35)", frags[0].X, frags[0].Y)
	}
}

func TestExtraerTextoNoSeAtascaConEntradaRara(t *testing.T) {
	// Un flujo con operadores desconocidos, paréntesis sueltos y binario debe
	// terminar igualmente: aquí un bucle infinito colgaría al servidor.
	entradas := []string{
		`BT ET`,
		`q 1 0 0 1 0 0 cm /Im0 Do Q`,
		`BT (sin cerrar`,
		`]]] >> >> ) ) `,
		"\x00\xFF\xFE(x",
		`BT /F1 12 Tf (a) ' (b) " ET`,
	}
	for i, e := range entradas {
		d, err := LeerPDF(pdfConContenido(t, e, helvetica, objHelvetica))
		if err != nil {
			t.Fatalf("entrada %d: %v", i, err)
		}
		hecho := make(chan struct{})
		go func() {
			defer close(hecho)
			_, _ = d.TextoDePagina(0)
		}()
		select {
		case <-hecho:
		case <-timeoutCorto():
			t.Fatalf("entrada %d: TextoDePagina no terminó (posible bucle infinito)", i)
		}
	}
}

// --- Flujos comprimidos ------------------------------------------------------

func TestDatosDeFlujoFlateDecode(t *testing.T) {
	var comprimido bytes.Buffer
	zw := zlib.NewWriter(&comprimido)
	original := "contenido que viaja comprimido dentro del PDF"
	_, _ = zw.Write([]byte(original))
	_ = zw.Close()

	d := &PDFDoc{}
	st := &pdfStream{
		Dict: pdfDict{"Filter": pdfName("FlateDecode")},
		Raw:  comprimido.Bytes(),
	}
	datos, err := d.datosDeFlujo(st)
	if err != nil {
		t.Fatalf("datosDeFlujo: %v", err)
	}
	if string(datos) != original {
		t.Errorf("descomprimido = %q, quería %q", datos, original)
	}
}

func TestDatosDeFlujoFiltroNoSoportado(t *testing.T) {
	d := &PDFDoc{}
	st := &pdfStream{Dict: pdfDict{"Filter": pdfName("JPXDecode")}, Raw: []byte("x")}
	if _, err := d.datosDeFlujo(st); !errors.Is(err, ErrPDFNoSoportado) {
		t.Errorf("err = %v, quería ErrPDFNoSoportado", err)
	}
}

func TestDecodificarHexASCII(t *testing.T) {
	casos := map[string]string{
		"48656C6C6F": "Hello",
		"48 65 6C>":  "Hel",
		"4":          "@", // dígito impar: se completa con 0 → 0x40
		"":           "",
	}
	for entrada, quiero := range casos {
		if got := string(decodificarHexASCII([]byte(entrada))); got != quiero {
			t.Errorf("%q → %q, quería %q", entrada, got, quiero)
		}
	}
}

func TestPredictorPNGUp(t *testing.T) {
	// Filtro «Up» (tipo 2): cada byte es la diferencia con el de la fila anterior.
	// Sin deshacerlo, la tabla xref de un PDF 1.5 sale desplazada y apunta a
	// cualquier parte, que es un fallo silencioso y difícil de ver.
	datos := []byte{
		2, 10, 20, 30, // fila 1: 0+10, 0+20, 0+30
		2, 1, 2, 3, // fila 2: 10+1, 20+2, 30+3
	}
	got := aplicarPredictor(datos, 12, 3, 1, 8)
	quiero := []byte{10, 20, 30, 11, 22, 33}
	if !bytes.Equal(got, quiero) {
		t.Errorf("predictor Up = %v, quería %v", got, quiero)
	}
}

// --- Utilidades del test -----------------------------------------------------

// timeoutCorto acota la espera de los checks anti-bucle: si el parser se atasca,
// el test falla con un mensaje claro en vez de agotar el timeout global de `go
// test` sin decir cuál de las entradas lo colgó.
func timeoutCorto() <-chan time.Time { return time.After(5 * time.Second) }

// pdfMinimo compone un PDF válido con los objetos dados y el trailer indicado,
// calculando la tabla xref. Sirve para llegar a casos que el escritor de la casa
// no genera nunca.
func pdfMinimo(t *testing.T, trailer string, objs []string) []byte {
	t.Helper()
	if objs == nil {
		objs = []string{
			`<< /Type /Catalog /Pages 2 0 R >>`,
			`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
			`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>`,
		}
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1)
	for i, cuerpo := range objs {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, cuerpo)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objs)+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n%s\nstartxref\n%d\n%%%%EOF", trailer, xref)
	return buf.Bytes()
}

// Poppler abre un PDF sin la línea %PDF- si el resto está (bug1606566 del
// corpus de pdf.js); nosotros decíamos que no era un PDF.
func TestLeerPDFSinCabecera(t *testing.T) {
	datos := pdfMinimo(t, `<< /Size 4 /Root 1 0 R >>`, nil)
	copy(datos, "%\xe2\xe3\xcf\xd3\n\n\n\n") // misma longitud: los desplazamientos siguen valiendo
	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if d.NumPaginas() != 1 {
		t.Errorf("NumPaginas = %d, quería 1", d.NumPaginas())
	}
}

// Un /Pages por nivel, más de cien niveles (deep-pages de qpdf): el tope de 64
// cortaba el árbol y el documento «no declaraba ninguna página».
func TestLeerPDFArbolDePaginasHondo(t *testing.T) {
	const niveles = 150
	objs := []string{`<< /Type /Catalog /Pages 2 0 R >>`}
	for i := 0; i < niveles; i++ {
		objs = append(objs, fmt.Sprintf(`<< /Type /Pages /Kids [%d 0 R] /Count 1 >>`, i+3))
	}
	objs = append(objs, `<< /Type /Page /MediaBox [0 0 100 100] >>`)
	d, err := LeerPDF(pdfMinimo(t, fmt.Sprintf(`<< /Size %d /Root 1 0 R >>`, len(objs)+1), objs))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if d.NumPaginas() != 1 {
		t.Errorf("NumPaginas = %d, quería 1", d.NumPaginas())
	}
}

// Un /Pages que no existe (issue19484 de pdf.js) o un árbol circular no dejan
// sin páginas un documento que las tiene: se buscan por su /Type y heredan de su
// /Parent.
func TestLeerPDFPaginasSinArbol(t *testing.T) {
	objs := []string{
		`<< /Type /Catalog /Pages 9 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 300 200] >>`,
		`<< /Type /Page /Parent 2 0 R >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 50 50] >>`,
	}
	d, err := LeerPDF(pdfMinimo(t, `<< /Size 5 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if d.NumPaginas() != 2 {
		t.Fatalf("NumPaginas = %d, quería 2", d.NumPaginas())
	}
	if p, _ := d.Pagina(0); p.Ancho != 300 || p.Alto != 200 {
		t.Errorf("la página 1 mide %v×%v, quería 300×200 (heredado del /Parent)", p.Ancho, p.Alto)
	}

	circular := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R 2 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>`,
	}
	d, err = LeerPDF(pdfMinimo(t, `<< /Size 4 /Root 1 0 R >>`, circular))
	if err != nil {
		t.Fatalf("LeerPDF circular: %v", err)
	}
	if d.NumPaginas() != 1 {
		t.Errorf("circular: NumPaginas = %d, quería 1", d.NumPaginas())
	}
}

// El predictor 2 (TIFF) se ignoraba: un flujo xref con él salía como basura y el
// documento «no tenía /Root» (tiff-predictor de qpdf).
func TestPredictorTIFF(t *testing.T) {
	casos := []struct {
		nombre                 string
		datos, quiero          []byte
		columnas, colores, bpc int
	}{
		{"8 bits", []byte{1, 1, 1, 1, 5, 0, 0, 0}, []byte{1, 2, 3, 4, 5, 5, 5, 5}, 4, 1, 8},
		{"8 bits, dos componentes", []byte{1, 10, 1, 1}, []byte{1, 10, 2, 11}, 2, 2, 8},
		{"16 bits", []byte{0, 1, 0, 255}, []byte{0, 1, 1, 0}, 2, 1, 16},
		{"1 bit", []byte{0b10000000}, []byte{0xFF}, 8, 1, 1},
	}
	for _, c := range casos {
		if got := aplicarPredictor(c.datos, 2, c.columnas, c.colores, c.bpc); !bytes.Equal(got, c.quiero) {
			t.Errorf("%s: %v, quería %v", c.nombre, got, c.quiero)
		}
	}
}

// Un fragmento árabe o hebreo sale en orden visual (los glifos van de izquierda
// a derecha): hay que darle la vuelta para que se lea y se encuentre.
func TestOrdenLogico(t *testing.T) {
	casos := map[string]string{
		"ةيبرعلا": "العربية",
		"A line with an Arabic ةيبرعلا word.": "A line with an Arabic العربية word.",
		"ינא םולש":               "שלום אני",
		"(رياني)":                "(يناير)",
		"١٤٣٤ ليربإ":             "إبريل ١٤٣٤",
		"sin nada que girar (1)": "sin nada que girar (1)",
	}
	for entrada, quiero := range casos {
		if got := ordenLogico(entrada); got != quiero {
			t.Errorf("ordenLogico(%q) = %q, quería %q", entrada, got, quiero)
		}
	}
	tu := `begincmap 1 begincodespacerange <00> <FF> endcodespacerange
4 beginbfchar <41> <05DD> <42> <05D5> <43> <05DC> <44> <05E9> endbfchar endcmap`
	frags := fragmentosDe(t, `BT /F1 12 Tf 10 50 Td (ABCD) Tj ET`, helvetica,
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode 6 0 R >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(tu), tu))
	if len(frags) != 1 || frags[0].Texto != "שלום" {
		t.Fatalf("fragmentos = %+v, quería «שלום»", frags)
	}
	// Con la matriz volteada los glifos avanzan hacia la izquierda: la cadena ya
	// va en orden de lectura y no se toca.
	frags = fragmentosDe(t, `BT /F1 12 Tf -1 0 0 1 100 50 Tm (DCBA) Tj ET`, helvetica,
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode 6 0 R >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(tu), tu))
	if len(frags) != 1 || frags[0].Texto != "שלום" {
		t.Fatalf("volteada: fragmentos = %+v, quería «שלום»", frags)
	}
}
