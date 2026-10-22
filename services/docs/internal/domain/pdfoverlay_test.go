// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de la superposición de texto (marcas de agua, encabezados/pies y
// numeración Bates).
//
// Lo que hay que demostrar aquí es que la capa se AÑADE y no SUSTITUYE: el
// contenido original tiene que seguir estando después. Un fallo en esa dirección
// no daría ningún error —el PDF sería válido— y se llevaría por delante el
// documento del usuario, así que cada caso vuelve a leer la salida y comprueba
// que lo de antes sigue ahí.
package domain

import (
	"strings"
	"testing"
)

// textoDeTodasLasPaginas concatena el texto plano de un PDF ya escrito.
func textoDeTodasLasPaginas(t *testing.T, datos []byte) []string {
	t.Helper()
	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("releer el PDF: %v", err)
	}
	out := make([]string, d.NumPaginas())
	for i := range out {
		s, err := d.TextoPlano(i)
		if err != nil {
			t.Fatalf("TextoPlano(%d): %v", i, err)
		}
		out[i] = s
	}
	return out
}

func TestMarcaDeAguaNoBorraElContenido(t *testing.T) {
	doc := docDePrueba(t, "contrato importante", "segunda hoja")

	datos, err := MarcaDeAguaPDF(doc, "BORRADOR")
	if err != nil {
		t.Fatalf("MarcaDeAguaPDF: %v", err)
	}
	paginas := textoDeTodasLasPaginas(t, datos)
	if len(paginas) != 2 {
		t.Fatalf("páginas = %d, quería 2", len(paginas))
	}
	for i, quiero := range []string{"contrato importante", "segunda hoja"} {
		if !strings.Contains(paginas[i], quiero) {
			t.Errorf("la página %d perdió su contenido: %q", i, paginas[i])
		}
		if !strings.Contains(paginas[i], "BORRADOR") {
			t.Errorf("la página %d no lleva la marca de agua: %q", i, paginas[i])
		}
	}
}

func TestMarcaDeAguaLlevaOpacidad(t *testing.T) {
	// Sin ExtGState la marca taparía el texto de debajo en vez de velarlo.
	doc := docDePrueba(t, "x")
	datos, err := MarcaDeAguaPDF(doc, "CONFIDENCIAL")
	if err != nil {
		t.Fatalf("MarcaDeAguaPDF: %v", err)
	}
	if !strings.Contains(string(datos), "ExtGState") {
		t.Error("la marca de agua no declara /ExtGState: saldría opaca")
	}
	if !strings.Contains(string(datos), "/SytueCapaGS gs") {
		t.Error("el flujo de la capa no aplica el estado gráfico de opacidad")
	}
}

func TestNumerarPaginas(t *testing.T) {
	doc := docDePrueba(t, "a", "b", "c")

	datos, err := NumerarPaginasPDF(doc, "", "")
	if err != nil {
		t.Fatalf("NumerarPaginasPDF: %v", err)
	}
	paginas := textoDeTodasLasPaginas(t, datos)
	for i, quiero := range []string{"1 / 3", "2 / 3", "3 / 3"} {
		if !strings.Contains(paginas[i], quiero) {
			t.Errorf("página %d = %q, quería que contuviera %q", i, paginas[i], quiero)
		}
	}
}

func TestNumeracionBatesRellenaConCeros(t *testing.T) {
	// La numeración Bates identifica cada página de un expediente de forma única;
	// el relleno con ceros es lo que hace que ordenen bien como texto.
	doc := docDePrueba(t, "a", "b")

	datos, err := NumeracionBatesPDF(doc, "ACME-", 998, 6)
	if err != nil {
		t.Fatalf("NumeracionBatesPDF: %v", err)
	}
	paginas := textoDeTodasLasPaginas(t, datos)
	if !strings.Contains(paginas[0], "ACME-000998") {
		t.Errorf("página 0 = %q, quería ACME-000998", paginas[0])
	}
	// Y el correlativo sigue al pasar de página, empezando donde se le dijo.
	if !strings.Contains(paginas[1], "ACME-000999") {
		t.Errorf("página 1 = %q, quería ACME-000999", paginas[1])
	}
}

func TestBatesPorDefectoSeisDigitosDesdeUno(t *testing.T) {
	doc := docDePrueba(t, "a")
	datos, err := NumeracionBatesPDF(doc, "EXP", 0, 0)
	if err != nil {
		t.Fatalf("NumeracionBatesPDF: %v", err)
	}
	if got := textoDeTodasLasPaginas(t, datos)[0]; !strings.Contains(got, "EXP000001") {
		t.Errorf("= %q, quería EXP000001", got)
	}
}

func TestEncabezadoYPie(t *testing.T) {
	doc := docDePrueba(t, "cuerpo")

	datos, err := EncabezadoPiePDF(doc, "Confidencial", CapaSuperiorCen)
	if err != nil {
		t.Fatalf("EncabezadoPiePDF: %v", err)
	}
	got := textoDeTodasLasPaginas(t, datos)[0]
	if !strings.Contains(got, "Confidencial") || !strings.Contains(got, "cuerpo") {
		t.Errorf("= %q; deberían estar el encabezado y el cuerpo", got)
	}
}

func TestCapaSoloEnAlgunasPaginas(t *testing.T) {
	doc := docDePrueba(t, "uno", "dos", "tres")

	datos, err := SuperponerTextoPDF(doc, CapaTexto{
		Texto:       "SOLO AQUI",
		Posicion:    CapaInferiorCen,
		SoloPaginas: []int{1},
	})
	if err != nil {
		t.Fatalf("SuperponerTextoPDF: %v", err)
	}
	paginas := textoDeTodasLasPaginas(t, datos)
	if strings.Contains(paginas[0], "SOLO AQUI") {
		t.Error("la página 0 no debía llevar la capa")
	}
	if !strings.Contains(paginas[1], "SOLO AQUI") {
		t.Error("la página 1 debía llevar la capa")
	}
	if strings.Contains(paginas[2], "SOLO AQUI") {
		t.Error("la página 2 no debía llevar la capa")
	}
	// Y ninguna pierde lo suyo.
	for i, quiero := range []string{"uno", "dos", "tres"} {
		if !strings.Contains(paginas[i], quiero) {
			t.Errorf("la página %d perdió su contenido: %q", i, paginas[i])
		}
	}
}

func TestCapaSinTextoFalla(t *testing.T) {
	doc := docDePrueba(t, "x")
	if _, err := SuperponerTextoPDF(doc, CapaTexto{Texto: "   "}); err == nil {
		t.Error("una capa sin texto debería fallar en vez de escribir una vacía")
	}
}

func TestCapaConservaLaFuenteOriginal(t *testing.T) {
	// El recurso de la capa se llama /SytueCapaF a propósito: si reutilizara /F1
	// —que es lo que usa cualquier documento— cambiaría la fuente del texto
	// original, y eso reescribe el documento sin decirlo.
	doc := docDePrueba(t, "texto con su fuente")
	datos, err := EncabezadoPiePDF(doc, "pie", CapaInferiorCen)
	if err != nil {
		t.Fatalf("EncabezadoPiePDF: %v", err)
	}
	s := string(datos)
	if !strings.Contains(s, "/SytueCapaF") {
		t.Error("la capa no declara su propia fuente")
	}
	d2, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	p, _ := d2.Pagina(0)
	rec, ok := d2.resolver(p.dict["Resources"]).(pdfDict)
	if !ok {
		t.Fatal("la página no tiene /Resources")
	}
	fuentes, ok := d2.resolver(rec["Font"]).(pdfDict)
	if !ok {
		t.Fatal("la página no tiene fuentes")
	}
	if _, hay := fuentes["SytueCapaF"]; !hay {
		t.Error("la fuente de la capa no llegó a /Resources: el texto no se pintaría")
	}
	if _, hay := fuentes["F1"]; !hay {
		t.Error("se perdió la fuente original F1 al inyectar la de la capa")
	}
}

func TestCapaSobrePaginaRotadaUsaLasMedidasSinRotar(t *testing.T) {
	// El visor aplica /Rotate DESPUÉS de dibujar, así que colocar el pie con las
	// medidas ya rotadas lo sacaría del papel en una página apaisada.
	doc := docDePrueba(t, "apaisada")
	rotado, err := RotarPaginasPDF(doc, map[int]int{0: 90})
	if err != nil {
		t.Fatalf("RotarPaginasPDF: %v", err)
	}
	d2, err := LeerPDF(rotado)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	datos, err := EncabezadoPiePDF(d2, "pie", CapaInferiorCen)
	if err != nil {
		t.Fatalf("EncabezadoPiePDF: %v", err)
	}
	d3, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("releer con capa: %v", err)
	}
	p, _ := d3.Pagina(0)
	if p.Rotacion != 90 {
		t.Errorf("se perdió la rotación: %d", p.Rotacion)
	}
	frags, err := d3.TextoDePagina(0)
	if err != nil {
		t.Fatalf("TextoDePagina: %v", err)
	}
	// El pie tiene que caer DENTRO del papel sin rotar (ancho×alto invertidos
	// respecto a lo que devuelve la página ya rotada).
	anchoSinRotar, altoSinRotar := p.Alto, p.Ancho
	for _, f := range frags {
		if !strings.Contains(f.Texto, "pie") {
			continue
		}
		if f.X < 0 || f.X > anchoSinRotar || f.Y < 0 || f.Y > altoSinRotar {
			t.Errorf("el pie cae fuera del papel: (%.0f, %.0f) en %0.f×%0.f",
				f.X, f.Y, anchoSinRotar, altoSinRotar)
		}
	}
}

func TestSuperponerSobreDocumentoSinPaginasFalla(t *testing.T) {
	if _, err := SuperponerTextoPDF(&PDFDoc{}, CapaTexto{Texto: "x"}); err == nil {
		t.Error("superponer sobre un documento sin páginas debería fallar")
	}
}
