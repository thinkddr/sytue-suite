// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del ensamblador de PDF (ola P4: combinar, dividir, reordenar, rotar,
// borrar y extraer páginas).
//
// El criterio de todos ellos es el mismo y es el único que vale: lo que sale se
// vuelve a LEER con el lector, y se comprueba que el contenido está donde debe.
// Un PDF que se escribe sin error pero que ningún lector abre —o que abre en
// blanco porque se perdieron los recursos por el camino— pasaría cualquier
// comprobación sobre los bytes; solo lo caza el viaje de ida y vuelta.
package domain

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// docDePrueba genera un PDF con una página por texto y lo abre.
func docDePrueba(t *testing.T, textos ...string) *PDFDoc {
	t.Helper()
	w := pdfsimple.Nuevo()
	for i, txt := range textos {
		if i > 0 {
			w.NuevaPagina()
		}
		w.Texto(txt, "F1", 12)
	}
	d, err := LeerPDF(w.Bytes())
	if err != nil {
		t.Fatalf("preparar el documento de prueba: %v", err)
	}
	if d.NumPaginas() != len(textos) {
		t.Fatalf("el documento de prueba tiene %d páginas y quería %d", d.NumPaginas(), len(textos))
	}
	return d
}

// textos devuelve el texto plano de cada página, para comparar de un vistazo.
func textos(t *testing.T, datos []byte) []string {
	t.Helper()
	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("releer el PDF producido: %v", err)
	}
	out := make([]string, d.NumPaginas())
	for i := range out {
		s, err := d.TextoPlano(i)
		if err != nil {
			t.Fatalf("TextoPlano(%d): %v", i, err)
		}
		out[i] = strings.TrimSpace(s)
	}
	return out
}

func TestExtraerPaginasConservaContenidoYOrden(t *testing.T) {
	doc := docDePrueba(t, "uno", "dos", "tres", "cuatro")

	datos, err := ExtraerPaginasPDF(doc, []int{2, 0})
	if err != nil {
		t.Fatalf("ExtraerPaginasPDF: %v", err)
	}
	got := textos(t, datos)
	if len(got) != 2 {
		t.Fatalf("páginas = %d, quería 2", len(got))
	}
	// El orden pedido manda: primero la tercera, luego la primera.
	if got[0] != "tres" || got[1] != "uno" {
		t.Errorf("contenido = %v, quería [tres uno]", got)
	}
}

func TestExtraerPaginaFueraDeRangoFalla(t *testing.T) {
	doc := docDePrueba(t, "sola")
	if _, err := ExtraerPaginasPDF(doc, []int{5}); err == nil {
		t.Error("extraer una página inexistente debería fallar, no producir un PDF raro")
	}
	if _, err := ExtraerPaginasPDF(doc, nil); err == nil {
		t.Error("extraer cero páginas debería fallar")
	}
}

func TestCombinarVariosDocumentos(t *testing.T) {
	a := docDePrueba(t, "alfa", "beta")
	b := docDePrueba(t, "gamma")

	datos, err := CombinarPDF(a, b)
	if err != nil {
		t.Fatalf("CombinarPDF: %v", err)
	}
	got := textos(t, datos)
	quiero := []string{"alfa", "beta", "gamma"}
	if len(got) != len(quiero) {
		t.Fatalf("páginas = %d, quería %d", len(got), len(quiero))
	}
	for i := range quiero {
		if got[i] != quiero[i] {
			t.Errorf("página %d = %q, quería %q", i, got[i], quiero[i])
		}
	}
}

func TestCombinarNoConfundeObjetosDelMismoNumero(t *testing.T) {
	// Dos documentos distintos tienen ambos un objeto 3, un objeto 4… Si el
	// ensamblador indexa lo copiado solo por número, el segundo documento reusa
	// por error los objetos del primero y sus páginas salen con el contenido
	// equivocado. Por eso la clave lleva también el documento de origen.
	a := docDePrueba(t, "primero")
	b := docDePrueba(t, "segundo")

	datos, err := CombinarPDF(a, b)
	if err != nil {
		t.Fatalf("CombinarPDF: %v", err)
	}
	got := textos(t, datos)
	if len(got) != 2 {
		t.Fatalf("páginas = %d, quería 2", len(got))
	}
	if got[0] != "primero" || got[1] != "segundo" {
		t.Errorf("contenido = %v, quería [primero segundo] — se han cruzado los objetos", got)
	}
}

func TestBorrarPaginas(t *testing.T) {
	doc := docDePrueba(t, "uno", "dos", "tres")

	datos, err := BorrarPaginasPDF(doc, []int{1})
	if err != nil {
		t.Fatalf("BorrarPaginasPDF: %v", err)
	}
	got := textos(t, datos)
	if len(got) != 2 || got[0] != "uno" || got[1] != "tres" {
		t.Errorf("contenido = %v, quería [uno tres]", got)
	}
}

func TestBorrarTodasLasPaginasFalla(t *testing.T) {
	// Un PDF sin páginas no es un PDF válido; mejor negarse que escribir algo
	// que ningún lector abre.
	doc := docDePrueba(t, "unica")
	if _, err := BorrarPaginasPDF(doc, []int{0}); err == nil {
		t.Error("borrar todas las páginas debería fallar")
	}
}

func TestRotarPaginas(t *testing.T) {
	doc := docDePrueba(t, "gira", "quieta")
	original, _ := doc.Pagina(0)

	datos, err := RotarPaginasPDF(doc, map[int]int{0: 90})
	if err != nil {
		t.Fatalf("RotarPaginasPDF: %v", err)
	}
	d2, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	p0, _ := d2.Pagina(0)
	p1, _ := d2.Pagina(1)
	if p0.Rotacion != 90 {
		t.Errorf("rotación de la página 0 = %d, quería 90", p0.Rotacion)
	}
	if p1.Rotacion != 0 {
		t.Errorf("la página 1 no se pidió rotar y salió con %d", p1.Rotacion)
	}
	// Rotada un cuarto de vuelta, el alto pasa a ser el ancho.
	if int(p0.Ancho) != int(original.Alto) || int(p0.Alto) != int(original.Ancho) {
		t.Errorf("tamaño rotado = %.0f×%.0f, quería %.0f×%.0f",
			p0.Ancho, p0.Alto, original.Alto, original.Ancho)
	}
	// Y el contenido sigue ahí: rotar no debe perder el texto.
	if txt, _ := d2.TextoPlano(0); !strings.Contains(txt, "gira") {
		t.Errorf("tras rotar, el texto es %q", txt)
	}
}

func TestRotarAcumulaSobreLaRotacionExistente(t *testing.T) {
	doc := docDePrueba(t, "x")
	unaVez, err := RotarPaginasPDF(doc, map[int]int{0: 90})
	if err != nil {
		t.Fatalf("primera rotación: %v", err)
	}
	d2, _ := LeerPDF(unaVez)
	dosVeces, err := RotarPaginasPDF(d2, map[int]int{0: 90})
	if err != nil {
		t.Fatalf("segunda rotación: %v", err)
	}
	d3, _ := LeerPDF(dosVeces)
	p, _ := d3.Pagina(0)
	if p.Rotacion != 180 {
		t.Errorf("dos giros de 90° dieron %d, quería 180", p.Rotacion)
	}
}

func TestRotacionNegativaYMayorDe360SeNormaliza(t *testing.T) {
	doc := docDePrueba(t, "x")
	datos, err := RotarPaginasPDF(doc, map[int]int{0: -90})
	if err != nil {
		t.Fatalf("RotarPaginasPDF: %v", err)
	}
	d2, _ := LeerPDF(datos)
	p, _ := d2.Pagina(0)
	if p.Rotacion != 270 {
		t.Errorf("-90° dio %d, quería 270", p.Rotacion)
	}
}

func TestPaginaExtraidaLlevaSuMediaBoxAunqueFueraHeredado(t *testing.T) {
	// En el origen el MediaBox estaba en el nodo /Pages, que NO viaja al
	// documento nuevo. Sin materializarlo, la página saldría con el tamaño por
	// defecto (Letter) en vez del suyo.
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 300 500] >>`,
		`<< /Type /Page /Parent 2 0 R >>`,
	}
	doc, err := LeerPDF(pdfMinimo(t, `<< /Size 4 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	datos, err := ExtraerPaginasPDF(doc, []int{0})
	if err != nil {
		t.Fatalf("ExtraerPaginasPDF: %v", err)
	}
	d2, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	p, _ := d2.Pagina(0)
	if p.Ancho != 300 || p.Alto != 500 {
		t.Errorf("tamaño = %.0f×%.0f, quería 300×500 (el heredado del origen)", p.Ancho, p.Alto)
	}
}

func TestPaginaCopiadaConservaSusRecursos(t *testing.T) {
	// La prueba de que el grafo se copia entero: si solo viajara el diccionario
	// de la página, /Resources quedaría colgando de una referencia rota y el
	// texto no se pintaría con su fuente.
	doc := docDePrueba(t, "con fuente")
	datos, err := ExtraerPaginasPDF(doc, []int{0})
	if err != nil {
		t.Fatalf("ExtraerPaginasPDF: %v", err)
	}
	d2, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	p, _ := d2.Pagina(0)
	recursos, ok := d2.resolver(p.dict["Resources"]).(pdfDict)
	if !ok {
		t.Fatal("la página copiada no tiene /Resources resoluble")
	}
	fuentes, ok := d2.resolver(recursos["Font"]).(pdfDict)
	if !ok || len(fuentes) == 0 {
		t.Fatal("la página copiada no tiene fuentes en /Resources")
	}
	for nombre, ref := range fuentes {
		f, ok := d2.resolver(ref).(pdfDict)
		if !ok {
			t.Errorf("la fuente %s no resuelve a un diccionario", nombre)
			continue
		}
		if f["BaseFont"] == nil {
			t.Errorf("la fuente %s llegó sin /BaseFont", nombre)
		}
	}
}

func TestObjetoCompartidoSeCopiaUnaSolaVez(t *testing.T) {
	// Las tres páginas del origen comparten las mismas fuentes. Copiarlas una
	// vez por página inflaría el fichero sin motivo, y en un PDF con imágenes
	// grandes eso multiplica su tamaño.
	doc := docDePrueba(t, "a", "b", "c")
	datos, err := ExtraerPaginasPDF(doc, []int{0, 1, 2})
	if err != nil {
		t.Fatalf("ExtraerPaginasPDF: %v", err)
	}
	d2, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	// Las tres páginas deben apuntar al MISMO objeto de fuente.
	var refs []pdfRef
	for i := 0; i < 3; i++ {
		p, _ := d2.Pagina(i)
		rec, ok := d2.resolver(p.dict["Resources"]).(pdfDict)
		if !ok {
			t.Fatalf("página %d sin /Resources", i)
		}
		fuentes, ok := d2.resolver(rec["Font"]).(pdfDict)
		if !ok {
			t.Fatalf("página %d sin fuentes", i)
		}
		if r, ok := fuentes["F1"].(pdfRef); ok {
			refs = append(refs, r)
		}
	}
	if len(refs) == 3 && (refs[0] != refs[1] || refs[1] != refs[2]) {
		t.Errorf("la fuente compartida se duplicó: %v", refs)
	}
}

func TestEnsambladorSinPaginasFalla(t *testing.T) {
	e := NuevoEnsambladorPDF()
	if _, err := e.Bytes(); err == nil {
		t.Error("escribir un PDF sin páginas debería fallar")
	}
}

func TestSalidaEsDeterminista(t *testing.T) {
	// Dos ejecuciones con la misma entrada tienen que dar los mismos bytes: si
	// el orden de las claves de un diccionario dependiera del mapa de Go, no
	// habría forma de comparar dos salidas ni de reproducir un resultado.
	doc := docDePrueba(t, "estable")
	a, err := ExtraerPaginasPDF(doc, []int{0})
	if err != nil {
		t.Fatalf("primera: %v", err)
	}
	b, err := ExtraerPaginasPDF(doc, []int{0})
	if err != nil {
		t.Fatalf("segunda: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Error("dos ensamblados de la misma entrada dieron bytes distintos")
	}
}

func TestSerializarNumerosSinNotacionCientifica(t *testing.T) {
	// «1e+06» no es un número válido en PDF: rompería el fichero entero.
	got := string(serializarObjetoPDF(pdfArray{1000000.0, 0.5, int64(42)}))
	if strings.ContainsAny(got, "eE") {
		t.Errorf("salió notación científica: %s", got)
	}
	if !strings.Contains(got, "1000000") {
		t.Errorf("el número grande no se escribió entero: %s", got)
	}
}

func TestSerializarCadenaBinariaEnHexadecimal(t *testing.T) {
	// Una cadena con bytes no imprimibles escrita como literal se corrompe.
	got := string(serializarObjetoPDF(pdfString{0x00, 0xFF, 0x41}))
	if !strings.HasPrefix(got, "<") {
		t.Errorf("una cadena binaria debería ir en hexadecimal: %s", got)
	}
	// Y una imprimible va como literal, que es más legible.
	got2 := string(serializarObjetoPDF(pdfString("hola")))
	if got2 != "(hola)" {
		t.Errorf("cadena imprimible = %s, quería (hola)", got2)
	}
}

func TestSerializarEscapaParentesisEnCadenas(t *testing.T) {
	got := string(serializarObjetoPDF(pdfString("con (parentesis)")))
	if got != `(con \(parentesis\))` {
		t.Errorf("escape = %s", got)
	}
}

func TestSerializarNombreConCaracteresRaros(t *testing.T) {
	got := string(serializarObjetoPDF(pdfName("Con Espacio")))
	if got != "/Con#20Espacio" {
		t.Errorf("nombre = %s, quería /Con#20Espacio", got)
	}
}

// --- Lo que el documento ES fuera de sus páginas ----------------------------

// docCompleto es un documento de tres páginas con índice, etiquetado, idioma,
// etiquetas de página y un enlace del índice de la página 1 a la 3: lo que un
// Word o un Acrobat dejan en cualquier informe.
func docCompleto(t *testing.T) *PDFDoc {
	t.Helper()
	return docConIndice(t,
		`/Lang (es-ES) /PageLabels << /Nums [0 << /S /r >>] >> /MarkInfo << /Marked true >> /StructTreeRoot 9 0 R`,
		`<< /Type /Outlines /First 7 0 R /Last 8 0 R /Count 2 >>`,
		`<< /Title (Uno) /Parent 6 0 R /Next 8 0 R /Dest [3 0 R /Fit] >>`,
		`<< /Title (Tres) /Parent 6 0 R /Prev 7 0 R /Dest [5 0 R /Fit] >>`,
		`<< /Type /StructTreeRoot /K [10 0 R] >>`,
		`<< /Type /StructElem /S /P /P 9 0 R /Pg 3 0 R /K 0 >>`)
}

// Anotar, girar o rellenar REESCRIBEN el fichero. Antes, el documento nuevo
// salía sin índice, sin etiquetado y sin idioma: perdía su accesibilidad y su
// navegación por añadirle una nota (medido con scripts/certipdf-compat).
func TestGuardarConservaElCatalogo(t *testing.T) {
	d := docCompleto(t)
	casos := map[string]func() ([]byte, error){
		"girar": func() ([]byte, error) { return RotarPaginasPDF(d, map[int]int{1: 90}) },
		"anotar": func() ([]byte, error) {
			return AnadirAnotacionesPDF(d, []Anotacion{{Pagina: 0, Tipo: AnotNota, Rect: [4]float64{10, 10, 30, 30}, Texto: "x"}})
		},
	}
	for nombre, f := range casos {
		salida, err := f()
		if err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}
		d2, err := LeerPDF(salida)
		if err != nil {
			t.Fatalf("%s: releer: %v", nombre, err)
		}
		idx := d2.IndiceDelDocumento()
		if len(idx) != 2 || idx[0].Pagina != 0 || idx[1].Pagina != 2 {
			t.Errorf("%s: índice = %+v, quería «Uno» → p1 y «Tres» → p3", nombre, idx)
		}
		raiz, _ := d2.resolver(d2.trailer["Root"]).(pdfDict)
		for _, k := range []pdfName{"Lang", "PageLabels", "MarkInfo", "StructTreeRoot"} {
			if _, hay := raiz[k]; !hay {
				t.Errorf("%s: el catálogo nuevo perdió /%s", nombre, k)
			}
		}
	}
}

// Un enlace a otra página tiene que seguir llevando a ESA página del documento
// nuevo, no a una copia suelta fuera del árbol de páginas que ningún lector
// enseña —que es lo que salía al copiar la página referenciada por el grafo—.
func TestGuardarNoDuplicaLasPaginasReferenciadas(t *testing.T) {
	d := docConEnlaces(t, `<< /Type /Annot /Subtype /Link /Rect [10 40 90 60] /Dest [4 0 R /Fit] >>`)
	salida, err := RotarPaginasPDF(d, map[int]int{0: 90})
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(salida, []byte("/Type /Page ")) + bytes.Count(salida, []byte("/Type /Page>")) +
		bytes.Count(salida, []byte("/Type /Page/")); n != 2 {
		t.Errorf("el fichero tiene %d objetos página, quería 2 (uno por página)", n)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatal(err)
	}
	e := d2.EnlacesDelDocumento()
	if len(e) != 1 || e[0].Destino != 1 {
		t.Fatalf("enlaces = %+v, quería uno a la página 2", e)
	}
}

// Una página que NO va al documento nuevo no se cuela por un enlace que la
// nombra: extraer la primera página no puede arrastrar la segunda.
func TestExtraerNoArrastraLaPaginaEnlazada(t *testing.T) {
	d := docConEnlaces(t, `<< /Type /Annot /Subtype /Link /Rect [10 40 90 60] /Dest [4 0 R /Fit] >>`)
	salida, err := ExtraerPaginasPDF(d, []int{0})
	if err != nil {
		t.Fatal(err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatal(err)
	}
	if d2.NumPaginas() != 1 {
		t.Fatalf("páginas = %d, quería 1", d2.NumPaginas())
	}
	if n := bytes.Count(salida, []byte("/Type /Page ")); n > 1 {
		t.Errorf("la página 2 viajó entera por el enlace: %d objetos página", n)
	}
}

// Censurar y sanear existen para que NO viaje nada que no se haya pedido: el
// árbol de estructura puede repetir en /ActualText lo que se acaba de tapar.
// Censurar sí conserva lo que no lleva texto, como el idioma
// (TestRedactarConservaDelCatalogoLoQueNoLlevaTexto); sanear, ni eso.
func TestCensurarYSanearNoConservanElCatalogo(t *testing.T) {
	d := docCompleto(t)
	sanear, err := SanearMetadatosPDF(d)
	if err != nil {
		t.Fatal(err)
	}
	censurar, err := RedactarPDF(d, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: 0, X2: 10, Y2: 10}})
	if err != nil {
		t.Fatal(err)
	}
	for nombre, salida := range map[string][]byte{"sanear": sanear, "censurar": censurar} {
		for _, k := range []string{"/StructTreeRoot", "/Outlines", "/Lang"} {
			if k == "/Lang" && nombre == "censurar" {
				continue
			}
			if bytes.Contains(salida, []byte(k)) {
				t.Errorf("%s: la salida lleva %s", nombre, k)
			}
		}
	}
}

// El XMP no viaja porque es historial, pero con él se iba la declaración PDF/A:
// girar una página de un PDF/A-2u dejaba de ser PDF/A para veraPDF (921 de 921
// salidas reescritas del corpus). Se conserva solo el identificador.
func TestGuardarConservaLaDeclaracionPDFA(t *testing.T) {
	xmp := `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
		`<rdf:Description rdf:about="" xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/" pdfaid:part="2" pdfaid:conformance="U"/>` +
		`<rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/"><xmp:CreatorTool>Herramienta interna</xmp:CreatorTool></rdf:Description>` +
		`</rdf:RDF></x:xmpmeta>`
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>`,
		fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream", len(xmp), xmp),
	}
	d, err := LeerPDF(pdfMinimo(t, `<< /Size 5 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatal(err)
	}
	salida, err := RotarPaginasPDF(d, map[int]int{0: 90})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(salida, []byte("Herramienta interna")) {
		t.Error("el historial del XMP viajó")
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatal(err)
	}
	if dec, ok := d2.declaraPDFA(); !ok || dec.parte != "2" || dec.conformidad != "U" {
		t.Errorf("declaración PDF/A = %+v, %v; quería 2U", dec, ok)
	}

	sin, err := RotarPaginasPDF(func() *PDFDoc { x, _ := LeerPDF(pdfMinimo(t, `<< /Size 4 /Root 1 0 R >>`, nil)); return x }(), map[int]int{0: 90})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sin, []byte("/Metadata")) {
		t.Error("un documento que no era PDF/A sale con XMP")
	}
}
