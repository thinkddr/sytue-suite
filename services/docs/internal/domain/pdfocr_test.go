// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de la capa de texto de un escaneo. Lo que hay que comprobar es la
// promesa entera: el documento se puede BUSCAR y se ve EXACTAMENTE IGUAL. Si
// falla la segunda mitad, la herramienta estropea el documento.
package domain

import (
	"fmt"
	"strings"
	"testing"
)

func TestCapaDeTextoSeBuscaYNoSeVe(t *testing.T) {
	// Un «escaneo»: una página con una imagen y sin nada de texto.
	origen, err := ImagenesAPDF([]ImagenAPagina{{Datos: pngDePrueba(t, 200, 100, false), Nombre: "s.png"}})
	if err != nil {
		t.Fatalf("ImagenesAPDF: %v", err)
	}
	d, err := LeerPDF(origen)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if n := len(d.PaginasSinTexto()); n != 1 {
		t.Fatalf("páginas sin texto = %d, quería 1 (es un escaneo)", n)
	}

	salida, err := CapaDeTextoPDF(d, map[int][]TextoReconocido{
		0: {{Texto: "FACTURA 2026-114", Rect: [4]float64{10, 60, 150, 78}}},
	})
	if err != nil {
		t.Fatalf("CapaDeTextoPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF con capa no abre: %v", err)
	}

	// 1. Ahora se busca.
	texto, err := d2.TextoPlano(0)
	if err != nil {
		t.Fatalf("TextoPlano: %v", err)
	}
	if !strings.Contains(texto, "FACTURA 2026-114") {
		t.Errorf("la capa no se puede leer: %q", texto)
	}
	if len(d2.PaginasSinTexto()) != 0 {
		t.Error("la página sigue contando como sin texto")
	}

	// 2. Y no se ve: el render lo emite transparente, no con tinta.
	svg, err := d2.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "FACTURA 2026-114") {
		t.Fatalf("el texto no llegó al render: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, `fill-opacity="0"`) {
		t.Error("la capa se está pintando con tinta: cambiaría el aspecto del escaneo")
	}
	// La imagen del escaneo sigue ahí: la capa se AÑADE, no sustituye.
	if !strings.Contains(svg, "<image ") {
		t.Errorf("se perdió la imagen escaneada: %s", primerasLineas(svg))
	}
}

func TestCapaDeTextoRespetaLasPaginasQueNoSeTocan(t *testing.T) {
	d := docDePrueba(t, "pagina uno", "pagina dos")
	salida, err := CapaDeTextoPDF(d, map[int][]TextoReconocido{
		1: {{Texto: "anexo", Rect: [4]float64{10, 10, 60, 24}}},
	})
	if err != nil {
		t.Fatalf("CapaDeTextoPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	uno, _ := d2.TextoPlano(0)
	if !strings.Contains(uno, "pagina uno") || strings.Contains(uno, "anexo") {
		t.Errorf("la primera página se tocó: %q", uno)
	}
	dos, _ := d2.TextoPlano(1)
	if !strings.Contains(dos, "anexo") {
		t.Errorf("la capa no llegó a la segunda página: %q", dos)
	}
}

func TestCapaDeTextoRechazaLoImposible(t *testing.T) {
	d := docDePrueba(t, "x")
	if _, err := CapaDeTextoPDF(d, nil); err == nil {
		t.Error("sin texto no hay capa")
	}
	if _, err := CapaDeTextoPDF(d, map[int][]TextoReconocido{
		9: {{Texto: "y", Rect: [4]float64{0, 0, 10, 10}}},
	}); err == nil {
		t.Error("una página que no existe debe fallar")
	}
}

func TestPaginasSinTextoDistingueEscaneoDeDocumento(t *testing.T) {
	if n := len(docDePrueba(t, "con texto").PaginasSinTexto()); n != 0 {
		t.Errorf("un documento con texto no necesita OCR, dio %d páginas", n)
	}
}

// --- La vuelta de coordenadas ------------------------------------------------

// imagenDePrueba coloca una imagen de `px`×`px` píxeles con la CTM dada.
func imagenDePrueba(px int, ctm matriz) ImagenColocada {
	return ImagenColocada{Ancho: px, Alto: px, ctm: ctm}
}

// TestRectDePDFVolteaElEje es EL check de esta pieza. Un OCR numera las filas de
// ARRIBA abajo y el PDF mide de ABAJO arriba: si el volteo falta o sobra, la capa
// de texto sale reflejada y la selección cae en la línea equivocada. No da ningún
// error — el documento se ve igual y solo falla al seleccionar.
func TestRectDePDFVolteaElEje(t *testing.T) {
	// Imagen de 100×100 píxeles ocupando el cuadrado [0,0]-[200,200] del PDF.
	im := imagenDePrueba(100, matriz{200, 0, 0, 200, 0, 0})

	// Una palabra en la esquina de ARRIBA a la izquierda de la imagen.
	arriba := im.RectDePDF(0, 0, 10, 10)
	// Y otra en la de ABAJO.
	abajo := im.RectDePDF(0, 90, 10, 10)

	if arriba[1] <= abajo[1] {
		t.Fatalf("la fila 0 de la imagen debe caer ARRIBA en el PDF: arriba=%v abajo=%v", arriba, abajo)
	}
	// La de arriba toca el borde superior del cuadrado.
	if arriba[3] < 199 {
		t.Errorf("arriba = %v, esperaba pegado a y=200", arriba)
	}
	if abajo[1] > 1 {
		t.Errorf("abajo = %v, esperaba pegado a y=0", abajo)
	}
}

func TestRectDePDFRespetaLaColocacion(t *testing.T) {
	// La misma imagen desplazada a (50,30) y de 100×100 puntos.
	im := imagenDePrueba(100, matriz{100, 0, 0, 100, 50, 30})
	r := im.RectDePDF(0, 0, 100, 100) // la imagen entera
	quiero := [4]float64{50, 30, 150, 130}
	for i := range quiero {
		if r[i] < quiero[i]-0.01 || r[i] > quiero[i]+0.01 {
			t.Fatalf("rect = %v, quería %v", r, quiero)
		}
	}
}

// TestRectDePDFConImagenGirada: una página escaneada de lado se coloca con una
// CTM que rota. Las cuatro esquinas hay que transformarlas de verdad, porque el
// envolvente NO sale de dos.
func TestRectDePDFConImagenGirada(t *testing.T) {
	// Giro de 90°: el eje X de la imagen va hacia arriba en el PDF.
	im := imagenDePrueba(100, matriz{0, 100, -100, 0, 100, 0})
	r := im.RectDePDF(0, 0, 100, 100)
	if r[2]-r[0] < 99 || r[3]-r[1] < 99 {
		t.Errorf("rect = %v: una imagen girada sigue ocupando 100×100", r)
	}
}

func TestRectDePDFSinTamanoNoRevienta(t *testing.T) {
	if r := (ImagenColocada{}).RectDePDF(0, 0, 10, 10); r != ([4]float64{}) {
		t.Errorf("una imagen sin tamaño debe dar el rectángulo vacío, dio %v", r)
	}
}

// TestImagenesParaOCRRescataElEscaneoPorBandas es el caso que `ImagenDeEscaneo`
// no puede resolver: un escaneo de gran formato llega partido en tiras y NINGUNA
// cubre el 60 % del papel, así que la página se contaba como «sin imagen» y no
// se reconocía nunca — sin error y sin forma de forzarlo.
func TestImagenesParaOCRRescataElEscaneoPorBandas(t *testing.T) {
	pixeles := "\x00\x40\x80\xff"
	imagen := func() string {
		return fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 2 /Height 2 "+
			"/ColorSpace /DeviceGray /BitsPerComponent 8 /Length %d >>\nstream\n%s\nendstream",
			len(pixeles), pixeles)
	}
	// Tres bandas de 100×30 sobre una página de 100×100: el 30 % cada una, así que
	// ninguna llega al 60 % que exige `ImagenDeEscaneo`.
	d, err := LeerPDF(pdfConContenido(t,
		"q 100 0 0 30 0 0 cm /Im0 Do Q q 100 0 0 30 0 35 cm /Im1 Do Q q 100 0 0 30 0 70 cm /Im2 Do Q",
		`/Resources << /XObject << /Im0 5 0 R /Im1 6 0 R /Im2 7 0 R >> >>`,
		imagen(), imagen(), imagen()))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if _, ok := d.ImagenDeEscaneo(0); ok {
		t.Fatal("ninguna banda cubre el 60 %: este es justo el caso que no resuelve")
	}
	trozos := d.ImagenesParaOCR(0)
	if len(trozos) != 3 {
		t.Fatalf("trozos = %d, quería las 3 bandas", len(trozos))
	}
	// Cada trozo lleva SU colocación, que es lo que permite devolver el texto a su
	// sitio en la página: si todos convirtieran igual, las tres bandas caerían una
	// encima de otra.
	alturas := make([]float64, 0, len(trozos))
	for _, tr := range trozos {
		r := tr.RectDePDF(0, 0, tr.Ancho, tr.Alto)
		alturas = append(alturas, r[1])
	}
	if alturas[0] == alturas[1] || alturas[1] == alturas[2] {
		t.Errorf("las bandas comparten posición vertical: %v", alturas)
	}
}

// TestImagenesParaOCRDejaFueraElLogotipo: mandar un logotipo al reconocedor
// gasta CPU para volver a leer el nombre de la empresa, y su texto —si lo
// tiene— ya está en el documento.
func TestImagenesParaOCRDejaFueraElLogotipo(t *testing.T) {
	pixeles := "\x00\x40\x80\xff"
	d, err := LeerPDF(pdfConContenido(t,
		"q 20 0 0 10 5 85 cm /Im0 Do Q",
		`/Resources << /XObject << /Im0 5 0 R >> >>`,
		fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 2 /Height 2 "+
			"/ColorSpace /DeviceGray /BitsPerComponent 8 /Length %d >>\nstream\n%s\nendstream",
			len(pixeles), pixeles)))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if n := len(d.ImagenesParaOCR(0)); n != 0 {
		t.Errorf("un logotipo de 20×10 en una página de 100×100 no va al OCR, dio %d", n)
	}
}

// TestImagenDeEscaneoDistingueLaPaginaDeUnLogotipo: mandar un logotipo al
// reconocedor gasta un segundo de CPU para leer el nombre de la empresa por
// décima vez. Solo cuenta la imagen que ES la página.
func TestImagenDeEscaneoDistingueLaPaginaDeUnLogotipo(t *testing.T) {
	pixeles := "\x00\x40\x80\xff"
	imagen := func() string {
		return fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 2 /Height 2 "+
			"/ColorSpace /DeviceGray /BitsPerComponent 8 /Length %d >>\nstream\n%s\nendstream",
			len(pixeles), pixeles)
	}

	// Un logotipo pequeño en la esquina: NO es un escaneo.
	d, err := LeerPDF(pdfConContenido(t,
		"q 20 0 0 10 5 85 cm /Im0 Do Q",
		`/Resources << /XObject << /Im0 5 0 R >> >>`, imagen()))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if _, ok := d.ImagenDeEscaneo(0); ok {
		t.Error("un logotipo de 20×10 en una página de 100×100 no es un escaneo")
	}

	// La misma imagen cubriendo la página: SÍ lo es.
	d, err = LeerPDF(pdfConContenido(t,
		"q 100 0 0 100 0 0 cm /Im0 Do Q",
		`/Resources << /XObject << /Im0 5 0 R >> >>`, imagen()))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	im, ok := d.ImagenDeEscaneo(0)
	if !ok {
		t.Fatal("una imagen que cubre la página entera SÍ es un escaneo")
	}
	if len(im.Datos) == 0 || im.Tipo == "" {
		t.Errorf("la imagen salió sin datos que mandar al reconocedor (%d bytes, tipo %q)",
			len(im.Datos), im.Tipo)
	}
}
