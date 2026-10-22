// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del render de PDF a SVG. Se comprueban las tres cosas que un visor
// puede romper sin que salte ningún error: que el contenido esté, que esté en el
// sitio correcto y que el color sea el que el documento pedía.
//
// Todos los casos construyen el PDF en el propio test —no hay binarios en el
// repo— y varios se han verificado por SABOTAJE: rompiendo a mano la línea que
// prueban y confirmando que el test se pone en rojo.
package domain

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// pdfConContenido arma un PDF de una página con el flujo de contenido dado.
// `extrasPagina` se pega dentro del diccionario de la página (por ejemplo
// /Resources o /Annots) y `objsExtra` se numeran a partir del 5.
func pdfConContenido(t *testing.T, contenido, extrasPagina string, objsExtra ...string) []byte {
	t.Helper()
	objs := make([]string, 0, 4+len(objsExtra))
	objs = append(objs,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 4 0 R `+extrasPagina+` >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(contenido), contenido),
	)
	objs = append(objs, objsExtra...)
	return pdfMinimo(t, fmt.Sprintf(`<< /Size %d /Root 1 0 R >>`, len(objs)+1), objs)
}

func renderizar(t *testing.T, datos []byte) string {
	t.Helper()
	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("RenderizarPaginaSVG: %v", err)
	}
	return svg
}

func TestRecursosSVGNoColisionanEntrePaginasODocumentos(t *testing.T) {
	crear := func(ancho int) *PDFDoc {
		contenido := []string{
			fmt.Sprintf("q 0 0 %d 40 re W n 0 0 100 100 re f Q", ancho),
			"q 20 20 60 60 re W n 0 0 100 100 re f Q",
		}
		objs := make([]string, 0, 4+len(contenido))
		objs = append(objs,
			`<< /Type /Catalog /Pages 2 0 R >>`,
			`<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>`,
			`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 5 0 R >>`,
			`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 6 0 R >>`,
		)
		for _, c := range contenido {
			objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c))
		}
		d, err := LeerPDF(pdfMinimo(t, `<< /Size 7 /Root 1 0 R >>`, objs))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	primero, segundo := crear(40), crear(60)
	ids := regexp.MustCompile(`\bid="([^"]+)"`)
	refs := regexp.MustCompile(`url\(#([^)]+)\)`)
	globales := map[string]bool{}
	for _, caso := range []struct {
		d *PDFDoc
		i int
	}{{primero, 0}, {primero, 1}, {segundo, 0}} {
		svg, err := caso.d.RenderizarPaginaSVG(caso.i, OpcionesRender{})
		if err != nil {
			t.Fatal(err)
		}
		locales := map[string]bool{}
		for _, id := range ids.FindAllStringSubmatch(svg, -1) {
			if globales[id[1]] {
				t.Errorf("recurso %q colisiona con otra página/documento inline", id[1])
			}
			globales[id[1]], locales[id[1]] = true, true
		}
		if len(locales) == 0 {
			t.Fatal("el caso no produjo recursos SVG")
		}
		for _, ref := range refs.FindAllStringSubmatch(svg, -1) {
			if !locales[ref[1]] {
				t.Errorf("referencia %q sin recurso propio", ref[1])
			}
		}
		otra, err := caso.d.RenderizarPaginaSVG(caso.i, OpcionesRender{})
		if err != nil || otra != svg {
			t.Fatal("renderizar la misma página debe conservar sus identidades")
		}
	}
}

// --- Estructura y encuadre ---------------------------------------------------

func TestRenderSVGDeclaraElTamanoDeLaPagina(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t, "", ""))
	if !strings.HasPrefix(svg, "<svg ") || !strings.HasSuffix(svg, "</svg>") {
		t.Fatalf("no parece un SVG completo: %.80s…", svg)
	}
	if !strings.Contains(svg, `viewBox="0 0 100 100"`) {
		t.Errorf("falta el viewBox de 100×100: %s", primerasLineas(svg))
	}
}

func TestRenderEscalaSoloCambiaElTamanoDibujado(t *testing.T) {
	d, err := LeerPDF(pdfConContenido(t, "", ""))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{Escala: 2})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, `width="200"`) || !strings.Contains(svg, `height="200"`) {
		t.Errorf("con escala 2 esperaba 200×200: %s", primerasLineas(svg))
	}
	// El viewBox NO cambia: el documento se sigue midiendo en puntos, que es lo
	// que permite superponer anotaciones sin recalcular nada al hacer zoom.
	if !strings.Contains(svg, `viewBox="0 0 100 100"`) {
		t.Errorf("la escala no debe tocar el viewBox: %s", primerasLineas(svg))
	}
}

// TestRenderVolteaElEjeY es el check que separa un render correcto de uno
// espejado. En PDF el origen está abajo; en SVG, arriba.
func TestRenderVolteaElEjeY(t *testing.T) {
	// Un rectángulo pegado al BORDE INFERIOR del papel (y de 0 a 10).
	svg := renderizar(t, pdfConContenido(t, "0 0 10 10 re f", ""))
	if !strings.Contains(svg, "matrix(1 0 0 -1 0 100)") {
		t.Fatalf("falta la matriz de volteo: %s", primerasLineas(svg))
	}
}

func TestRenderRestaLaEsquinaDelMediaBox(t *testing.T) {
	// Un MediaBox que no empieza en el origen: el contenido está desplazado y hay
	// que restarlo, o la página sale corrida por el margen.
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [20 30 120 130] /Contents 4 0 R >>`,
		"<< /Length 14 >>\nstream\n0 0 10 10 re f\nendstream",
	}
	svg := renderizar(t, pdfMinimo(t, `<< /Size 5 /Root 1 0 R >>`, objs))
	if !strings.Contains(svg, "matrix(1 0 0 -1 -20 130)") {
		t.Errorf("la esquina del MediaBox no se resta: %s", primerasLineas(svg))
	}
}

func TestRenderPaginaRotadaIntercambiaElViewBox(t *testing.T) {
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 400] /Rotate 90 /Contents 4 0 R >>`,
		"<< /Length 0 >>\nstream\n\nendstream",
	}
	svg := renderizar(t, pdfMinimo(t, `<< /Size 5 /Root 1 0 R >>`, objs))
	if !strings.Contains(svg, `viewBox="0 0 400 200"`) {
		t.Errorf("una página rotada 90° se ve apaisada: %s", primerasLineas(svg))
	}
}

// --- Caminos y color ---------------------------------------------------------

func TestRenderRectanguloConColor(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t, "1 0 0 rg 10 20 30 40 re f", ""))
	if !strings.Contains(svg, `fill="#ff0000"`) {
		t.Errorf("el rojo de `1 0 0 rg` no llegó: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, "M10 20h30v40h-30Z") {
		t.Errorf("el operador `re` no se expandió bien: %s", primerasLineas(svg))
	}
}

func TestRenderCMYKSeConvierte(t *testing.T) {
	// Cian puro: (1-1)·(1-0)=0 en rojo, 1 en verde y azul.
	svg := renderizar(t, pdfConContenido(t, "1 0 0 0 k 0 0 10 10 re f", ""))
	if !strings.Contains(svg, `fill="#00ffff"`) {
		t.Errorf("el cian CMYK no se convirtió: %s", primerasLineas(svg))
	}
}

func TestRenderTrazoSinRellenoNoPintaRelleno(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t, "0 0 1 RG 2 w 10 10 m 90 90 l S", ""))
	if !strings.Contains(svg, `fill="none"`) {
		t.Errorf("`S` solo traza: el relleno tiene que ser none: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, `stroke="#0000ff"`) || !strings.Contains(svg, `stroke-width="2"`) {
		t.Errorf("el trazo azul de 2 puntos no llegó: %s", primerasLineas(svg))
	}
}

// TestRenderGrosorCeroSigueDibujando cubre el fallo silencioso más molesto: en
// PDF el grosor 0 es «lo más fino posible», en SVG es «no pintes nada». Sin
// traducirlo, las tablas se quedan sin líneas y nadie ve un error.
func TestRenderGrosorCeroSigueDibujando(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t, "0 w 10 10 m 90 10 l S", ""))
	if strings.Contains(svg, `stroke-width="0"`) {
		t.Fatalf("un grosor 0 literal deja la línea invisible: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, "non-scaling-stroke") {
		t.Errorf("falta el pelo de un píxel para grosor 0: %s", primerasLineas(svg))
	}
}

func TestRenderReglaParImpar(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t, "0 0 50 50 re 10 10 30 30 re f*", ""))
	if !strings.Contains(svg, `fill-rule="evenodd"`) {
		t.Errorf("`f*` es par-impar: sin esa regla el agujero se rellena: %s", primerasLineas(svg))
	}
}

func TestRenderCurvasBezier(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t, "10 10 m 20 30 40 30 50 10 c S", ""))
	if !strings.Contains(svg, "C20 30 40 30 50 10") {
		t.Errorf("la curva no se tradujo: %s", primerasLineas(svg))
	}
}

func TestRenderCurvaVUsaElPuntoActualComoControl(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t, "10 10 m 40 30 50 10 v S", ""))
	// `v` toma el punto actual (10,10) como primer control.
	if !strings.Contains(svg, "C10 10 40 30 50 10") {
		t.Errorf("`v` mal traducido: %s", primerasLineas(svg))
	}
}

func TestRenderTransformacionCM(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t, "q 2 0 0 2 5 5 cm 0 0 10 10 re f Q", ""))
	// La CTM del camino es [2 0 0 2 5 5] compuesta con el volteo de la página.
	if !strings.Contains(svg, "matrix(2 0 0 -2 5 95)") {
		t.Errorf("`cm` no se compuso con la matriz base: %s", primerasLineas(svg))
	}
}

func TestRenderRecorteAbreYCierraGrupo(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t, "q 10 10 50 50 re W n 0 0 100 100 re f Q", ""))
	if !strings.Contains(svg, "<clipPath") || !strings.Contains(svg, "clip-path=\"url(#") {
		t.Fatalf("el recorte `W n` no generó clipPath: %s", primerasLineas(svg))
	}
	if strings.Count(svg, "<g ") != strings.Count(svg, "</g>") {
		t.Errorf("grupos sin equilibrar: %d abren, %d cierran",
			strings.Count(svg, "<g "), strings.Count(svg, "</g>"))
	}
}

// TestRenderQRestauraElEstadoDeSuQ: `Q` vuelve al estado que guardó SU `q`, no
// al del `q` anterior. Con una matriz puesta ANTES del `q` —Google Slides abre
// así todas sus páginas— lo que venía tras el `Q` salía fuera de la hoja.
func TestRenderQRestauraElEstadoDeSuQ(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t, "1 0 0 1 10 0 cm q 2 0 0 2 0 0 cm Q 0 0 5 5 re f", ""))
	// MediaBox [0 0 100 100]: la base es matrix(1 0 0 -1 0 100) y el `cm` la
	// desplaza 10 puntos. Tras el `Q` el rectángulo tiene que seguir desplazado.
	if !strings.Contains(svg, `transform="matrix(1 0 0 -1 10 100)"`) {
		t.Fatalf("el `Q` no devolvió la matriz de antes de su `q`: %s", primerasLineas(svg))
	}
}

// TestRenderCierraLosGruposQueElFlujoDejaAbiertos cubre un PDF con más `q` que
// `Q`, que es corriente. Sin cerrarlos, el SVG sale mal formado y el navegador
// pinta media página.
func TestRenderCierraLosGruposQueElFlujoDejaAbiertos(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t, "q 0 0 50 50 re W n q 1 0 0 rg 0 0 10 10 re f", ""))
	if strings.Count(svg, "<g ") != strings.Count(svg, "</g>") {
		t.Errorf("grupos sin equilibrar: %s", primerasLineas(svg))
	}
}

func TestRenderOpacidadDeExtGState(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t,
		"/GS0 gs 1 0 0 rg 0 0 50 50 re f",
		`/Resources << /ExtGState << /GS0 << /ca 0.5 >> >> >>`))
	if !strings.Contains(svg, `fill-opacity="0.5"`) {
		t.Errorf("la opacidad de /ExtGState no se aplicó: %s", primerasLineas(svg))
	}
}

func TestRenderModoDeFusion(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t,
		"/GS0 gs 0 0 50 50 re f",
		`/Resources << /ExtGState << /GS0 << /BM /Multiply >> >> >>`))
	if !strings.Contains(svg, "mix-blend-mode:multiply") {
		t.Errorf("el modo de fusión no llegó: %s", primerasLineas(svg))
	}
}

// --- Texto -------------------------------------------------------------------

func TestRenderTextoDelEscritorDeLaCasa(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("Hola mundo", "F1", 12)
	svg := renderizar(t, w.Bytes())
	if !strings.Contains(svg, "<text ") {
		t.Fatalf("no se emitió ningún <text>: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, "Hola mundo") {
		t.Errorf("el texto no aparece en el SVG: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, `font-size="12"`) {
		t.Errorf("el tamaño de fuente no llegó: %s", primerasLineas(svg))
	}
	// `scale(1,-1)` es lo que evita que el texto salga cabeza abajo dentro del
	// grupo volteado de la página.
	if !strings.Contains(svg, "scale(1,-1)") {
		t.Errorf("falta el contravolteo del texto: %s", primerasLineas(svg))
	}
}

func TestRenderTextoEscapaXML(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t,
		"BT /F1 12 Tf 10 50 Td (a < b & c) Tj ET",
		`/Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> >> >>`))
	if strings.Contains(svg, "a < b & c") {
		t.Fatalf("el texto entró sin escapar y rompe el XML: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, "a &lt; b &amp; c") {
		t.Errorf("no se escapó bien: %s", primerasLineas(svg))
	}
}

// TestRenderTextoInvisibleSeConservaTransparente es la capa de OCR de un PDF
// escaneado: si se omitiera, el escaneo se quedaría sin texto buscable.
func TestRenderTextoInvisibleSeConservaTransparente(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t,
		"BT 3 Tr /F1 12 Tf 10 50 Td (capa de ocr) Tj ET",
		`/Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> >> >>`))
	if !strings.Contains(svg, "capa de ocr") {
		t.Fatalf("el texto invisible se perdió: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, `fill-opacity="0"`) {
		t.Errorf("el modo 3 tiene que ser transparente, no visible: %s", primerasLineas(svg))
	}
}

// TestRenderTextoFijaLaAnchuraConLasMetricas comprueba que el avance sale de las
// métricas del documento y no de la fuente que acabe usando el navegador.
func TestRenderTextoFijaLaAnchuraConLasMetricas(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t,
		"BT /F1 10 Tf 10 50 Td (AAAA) Tj ET",
		`/Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica `+
			`/FirstChar 65 /LastChar 65 /Widths [500] >> >> >>`))
	// Cuatro glifos de 500 milésimas a 10 puntos = 20 puntos exactos.
	if !strings.Contains(svg, `textLength="20"`) {
		t.Errorf("la anchura no se fijó con /Widths: %s", primerasLineas(svg))
	}
}

func TestRenderTJAjustaLaSeparacion(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t,
		"BT /F1 10 Tf 10 50 Td [(A) -500 (B)] TJ ET",
		`/Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica `+
			`/FirstChar 65 /LastChar 66 /Widths [500 500] >> >> >>`))
	// A empieza en x=10; B en 10 + 5 (ancho de A) + 5 (el ajuste de −500) = 20.
	if !strings.Contains(svg, "matrix(1 0 0 -1 20 50)") {
		t.Errorf("el ajuste de TJ no movió el cursor: %s", primerasLineas(svg))
	}
}

func TestRenderTextoUsaToUnicode(t *testing.T) {
	// Fuente compuesta: los códigos son índices de glifo y solo /ToUnicode dice
	// qué letra es cada uno.
	cmap := "/CIDInit /ProcSet findresource begin 1 begincmap\n" +
		"1 beginbfchar <0003> <0041> endbfchar\nendcmap end"
	svg := renderizar(t, pdfConContenido(t,
		"BT /F1 12 Tf 10 50 Td <0003> Tj ET",
		`/Resources << /Font << /F1 5 0 R >> >>`,
		`<< /Type /Font /Subtype /Type0 /BaseFont /Sub+Test /Encoding /Identity-H `+
			`/DescendantFonts [6 0 R] /ToUnicode 7 0 R >>`,
		`<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Sub+Test /DW 600 >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap)))
	if !strings.Contains(svg, ">A</text>") {
		t.Errorf("el código 0x0003 tenía que leerse como «A»: %s", primerasLineas(svg))
	}
}

// --- Imágenes ----------------------------------------------------------------

func TestRenderImagenEnGrisesDaUnPNG(t *testing.T) {
	// Cuatro píxeles de 8 bits en escala de grises, sin comprimir.
	pixeles := "\x00\x40\x80\xff"
	svg := renderizar(t, pdfConContenido(t,
		"q 50 0 0 50 10 10 cm /Im0 Do Q",
		`/Resources << /XObject << /Im0 5 0 R >> >>`,
		fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 2 /Height 2 "+
			"/ColorSpace /DeviceGray /BitsPerComponent 8 /Length %d >>\nstream\n%s\nendstream",
			len(pixeles), pixeles)))
	if !strings.Contains(svg, "<image ") || !strings.Contains(svg, "data:image/png;base64,") {
		t.Fatalf("la imagen no se emitió: %s", primerasLineas(svg))
	}
	// El cuadrado unidad se voltea otra vez para que la foto no salga del revés:
	// [1 0 0 -1 0 1] · [50 0 0 50 10 10] · base = [50 0 0 50 10 40] en SVG.
	if !strings.Contains(svg, "matrix(50 0 0 50 10 40)") {
		t.Errorf("la imagen no está donde el `cm` la coloca: %s", primerasLineas(svg))
	}
}

func TestRenderImagenMascaraUsaElColorDeRelleno(t *testing.T) {
	// Una imagen-máscara no lleva color: se pinta con el relleno vigente.
	svg := renderizar(t, pdfConContenido(t,
		"q 1 0 0 rg 50 0 0 50 10 10 cm /Im0 Do Q",
		`/Resources << /XObject << /Im0 5 0 R >> >>`,
		"<< /Type /XObject /Subtype /Image /Width 8 /Height 1 /ImageMask true "+
			"/BitsPerComponent 1 /Length 1 >>\nstream\n\x00\nendstream"))
	if !strings.Contains(svg, "<image ") {
		t.Errorf("el estarcido no se pintó: %s", primerasLineas(svg))
	}
}

func TestRenderImagenIlegibleDejaUnHuecoVisible(t *testing.T) {
	// JPEG 2000: ningún navegador lo abre y aquí no hay decodificador. Lo que no
	// puede pasar es que la página salga como si ahí no hubiera nada.
	svg := renderizar(t, pdfConContenido(t,
		"q 50 0 0 50 10 10 cm /Im0 Do Q",
		`/Resources << /XObject << /Im0 5 0 R >> >>`,
		"<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /Filter /JPXDecode "+
			"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Length 4 >>\nstream\nabcd\nendstream"))
	if !strings.Contains(svg, "#e5e5e5") {
		t.Errorf("una imagen que no se puede decodificar debe dejar marca: %s", primerasLineas(svg))
	}
}

// --- XObjects de formulario y anotaciones ------------------------------------

func TestRenderXObjectDeFormularioSeEjecuta(t *testing.T) {
	interno := "1 0 0 rg 0 0 10 10 re f"
	svg := renderizar(t, pdfConContenido(t,
		"q 1 0 0 1 20 20 cm /Fx0 Do Q",
		`/Resources << /XObject << /Fx0 5 0 R >> >>`,
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length %d >>\nstream\n%s\nendstream",
			len(interno), interno)))
	if !strings.Contains(svg, `fill="#ff0000"`) {
		t.Errorf("el contenido del formulario no se ejecutó: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, "<clipPath") {
		t.Errorf("la /BBox del formulario tiene que recortar: %s", primerasLineas(svg))
	}
}

// TestRenderAnotacionSePinta cubre el agujero clásico: un resaltado, un sello o
// una firma NO están en el flujo de la página, sino en su flujo de apariencia.
// Sin esto, el visor enseña el documento sin las anotaciones y parece que se han
// perdido.
func TestRenderAnotacionSePinta(t *testing.T) {
	ap := "0 1 0 rg 0 0 20 20 re f"
	svg := renderizar(t, pdfConContenido(t,
		"",
		`/Annots [5 0 R]`,
		`<< /Type /Annot /Subtype /Square /Rect [10 10 30 30] /AP << /N 6 0 R >> >>`,
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 20 20] /Length %d >>\nstream\n%s\nendstream",
			len(ap), ap)))
	if !strings.Contains(svg, `fill="#00ff00"`) {
		t.Errorf("la apariencia de la anotación no se pintó: %s", primerasLineas(svg))
	}
}

func TestRenderAnotacionOcultaNoSePinta(t *testing.T) {
	ap := "0 1 0 rg 0 0 20 20 re f"
	svg := renderizar(t, pdfConContenido(t,
		"",
		`/Annots [5 0 R]`,
		`<< /Type /Annot /Subtype /Square /Rect [10 10 30 30] /F 2 /AP << /N 6 0 R >> >>`,
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 20 20] /Length %d >>\nstream\n%s\nendstream",
			len(ap), ap)))
	if strings.Contains(svg, `fill="#00ff00"`) {
		t.Errorf("el bit «oculta» de /F no se respetó: %s", primerasLineas(svg))
	}
}

func TestRenderSinAnotacionesLasOmite(t *testing.T) {
	ap := "0 1 0 rg 0 0 20 20 re f"
	datos := pdfConContenido(t,
		"",
		`/Annots [5 0 R]`,
		`<< /Type /Annot /Subtype /Square /Rect [10 10 30 30] /AP << /N 6 0 R >> >>`,
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 20 20] /Length %d >>\nstream\n%s\nendstream",
			len(ap), ap))
	d, _ := LeerPDF(datos)
	svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{SinAnotaciones: true})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(svg, `fill="#00ff00"`) {
		t.Errorf("SinAnotaciones no las omitió: %s", primerasLineas(svg))
	}
}

// --- Degradados --------------------------------------------------------------

func TestRenderSombreadoAxialDaUnGradiente(t *testing.T) {
	svg := renderizar(t, pdfConContenido(t,
		"q 0 0 100 100 re W n /Sh0 sh Q",
		`/Resources << /Shading << /Sh0 << /ShadingType 2 /ColorSpace /DeviceRGB `+
			`/Coords [0 0 100 0] /Function << /FunctionType 2 /Domain [0 1] `+
			`/C0 [1 0 0] /C1 [0 0 1] /N 1 >> >> >> >>`))
	if !strings.Contains(svg, "<linearGradient") {
		t.Fatalf("el sombreado axial no dio gradiente: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, `stop-color="#ff0000"`) || !strings.Contains(svg, `stop-color="#0000ff"`) {
		t.Errorf("las paradas no van de rojo a azul: %s", primerasLineas(svg))
	}
}

func TestRenderPatronDeMosaicoSeRepite(t *testing.T) {
	celda := "0 0 1 rg 0 0 5 5 re f"
	svg := renderizar(t, pdfConContenido(t,
		"/Pattern cs /P0 scn 0 0 100 100 re f",
		`/Resources << /Pattern << /P0 5 0 R >> >>`,
		fmt.Sprintf("<< /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] "+
			"/XStep 10 /YStep 10 /Length %d >>\nstream\n%s\nendstream", len(celda), celda)))
	if !strings.Contains(svg, "<pattern ") {
		t.Fatalf("el patrón de mosaico no se emitió: %s", primerasLineas(svg))
	}
	id := regexp.MustCompile(`<pattern id="([^"]+)"`).FindStringSubmatch(svg)
	if len(id) != 2 || !strings.Contains(svg, `fill="url(#`+id[1]+`)"`) {
		t.Errorf("el relleno no apunta al patrón: %s", primerasLineas(svg))
	}
}

func TestRecursosDeCeldasDeMosaicoConservanNamespace(t *testing.T) {
	celda := "0 0 5 5 re W n 0 0 10 10 re f"
	svg := renderizar(t, pdfConContenido(t,
		"/Pattern cs /P0 scn 0 0 50 50 re f /P0 scn 50 50 50 50 re f",
		`/Resources << /Pattern << /P0 5 0 R >> >>`,
		fmt.Sprintf("<< /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] "+
			"/XStep 10 /YStep 10 /Length %d >>\nstream\n%s\nendstream", len(celda), celda)))
	ids := regexp.MustCompile(`\bid="([^"]+)"`).FindAllStringSubmatch(svg, -1)
	if len(ids) != 4 {
		t.Fatalf("esperaba dos patrones con sus recortes, hay %d recursos", len(ids))
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !strings.HasPrefix(id[1], "sypdf-") || seen[id[1]] {
			t.Errorf("la celda perdió el namespace o reutilizó un recurso: %q", id[1])
		}
		seen[id[1]] = true
		if !strings.Contains(svg, `url(#`+id[1]+`)`) {
			t.Errorf("recurso de celda %q sin referencia propia", id[1])
		}
	}
}

// --- Funciones ---------------------------------------------------------------

func TestFuncionExponencialInterpola(t *testing.T) {
	f := &funcionPDF{tipo: 2, dominio: []float64{0, 1}, c0: []float64{0}, c1: []float64{1}, exp: 1, nSalida: 1}
	if v := f.evaluar(0.5); len(v) != 1 || v[0] < 0.49 || v[0] > 0.51 {
		t.Errorf("f(0.5) = %v, quería 0.5", v)
	}
}

func TestFuncionMuestreadaLeeBitsSueltos(t *testing.T) {
	// Cuatro muestras de 4 bits: 0, 5, 10, 15.
	f := &funcionPDF{
		tipo: 0, dominio: []float64{0, 1}, rango: []float64{0, 1},
		muestras: []byte{0x05, 0xAF}, tamano: []int{4}, bps: 4, nSalida: 1,
	}
	if v := f.evaluar(1); len(v) != 1 || v[0] < 0.99 {
		t.Errorf("f(1) = %v, quería 1 (la última muestra es 15/15)", v)
	}
	if v := f.evaluar(0); len(v) != 1 || v[0] > 0.01 {
		t.Errorf("f(0) = %v, quería 0", v)
	}
}

// --- Filtros nuevos ----------------------------------------------------------

func TestDecodificarASCII85(t *testing.T) {
	if got := string(decodificarASCII85([]byte(`87cURD]i,"Ebo80~>`))); got != "Hello World!" {
		t.Errorf("ASCII85 = %q, quería «Hello World!»", got)
	}
	if got := string(decodificarASCII85([]byte("z~>"))); got != "\x00\x00\x00\x00" {
		t.Errorf("la 'z' tiene que valer cuatro ceros, dio %q", got)
	}
}

func TestDecodificarRunLength(t *testing.T) {
	// 2 → copia 3 bytes; 254 → repite el siguiente 3 veces; 128 → fin.
	entrada := []byte{2, 'a', 'b', 'c', 254, 'x', 128, 'z'}
	if got := string(decodificarRunLength(entrada)); got != "abcxxx" {
		t.Errorf("RunLength = %q, quería «abcxxx»", got)
	}
}

func TestDescomprimirLZW(t *testing.T) {
	// El ejemplo de la especificación (§7.4.4): la secuencia de códigos
	// 256 45 258 258 65 259 257 codifica «-----A---». Cubre además el caso
	// KwKwK —un código que TODAVÍA no está en la tabla y hay que deducir del
	// anterior—, que es donde se rompen las implementaciones a medias.
	//nolint:gomnd // los códigos son literales de la especificación
	bits := []int{256, 45, 258, 258, 65, 259, 257}
	var buf []byte
	acumulado, n := uint32(0), 0
	for _, c := range bits {
		acumulado = acumulado<<9 | uint32(c)
		n += 9
		for n >= 8 {
			buf = append(buf, byte(acumulado>>uint(n-8)))
			n -= 8
		}
	}
	if n > 0 {
		buf = append(buf, byte(acumulado<<uint(8-n)))
	}
	if got := string(descomprimirLZW(buf, true)); got != "-----A---" {
		t.Errorf("LZW = %q, quería «-----A---»", got)
	}
}

func TestFlujoConLZWSeLee(t *testing.T) {
	// Comprueba el cableado del filtro, no el algoritmo: un flujo declarado LZW
	// tiene que llegar al descompresor en vez de tumbar la lectura.
	d := &PDFDoc{objs: map[int]pdfObj{}, xref: map[int]xrefEntrada{}}
	st := &pdfStream{Dict: pdfDict{"Filter": pdfName("LZWDecode")}, Raw: []byte{0x80, 0x0B, 0x60, 0x50, 0x22, 0x0C, 0x0C, 0x85, 0x01}}
	if _, err := d.datosDeFlujo(st); err != nil {
		t.Errorf("LZWDecode debería estar soportado: %v", err)
	}
}

func TestDecodeParmsSeAlineaConSuFiltro(t *testing.T) {
	// Dos filtros: el predictor va con el SEGUNDO. Cogiendo siempre el primer
	// /DecodeParms se aplicaría un predictor que no toca y saldría basura.
	var comprimido bytes.Buffer
	zw := zlib.NewWriter(&comprimido)
	_, _ = zw.Write([]byte("hola"))
	_ = zw.Close()

	d := &PDFDoc{objs: map[int]pdfObj{}, xref: map[int]xrefEntrada{}}
	st := &pdfStream{
		Dict: pdfDict{
			"Filter":      pdfArray{pdfName("ASCIIHexDecode"), pdfName("FlateDecode")},
			"DecodeParms": pdfArray{pdfNull{}, pdfDict{"Predictor": int64(1)}},
		},
		Raw: []byte(hexDe(comprimido.Bytes()) + ">"),
	}
	got, err := d.datosDeFlujo(st)
	if err != nil {
		t.Fatalf("datosDeFlujo: %v", err)
	}
	if string(got) != "hola" {
		t.Errorf("cadena de filtros = %q, quería «hola»", got)
	}
}

func hexDe(b []byte) string {
	const d = "0123456789abcdef"
	var sb strings.Builder
	for _, x := range b {
		sb.WriteByte(d[x>>4])
		sb.WriteByte(d[x&0xf])
	}
	return sb.String()
}

// --- Reconstrucción de la cmap ----------------------------------------------

// TestReescribirCmapTTF comprueba la pieza que hace que una fuente Identity-H
// incrustada se vea con SU tipografía: sin la tabla `cmap` reconstruida, el
// navegador no encuentra el glifo y pinta cuadritos.
func TestReescribirCmapTTF(t *testing.T) {
	ttf := sfntDePrueba(t)
	salida, ok := reescribirCmapTTF(ttf, map[uint32]string{3: "A", 4: "B", 5: "C"})
	if !ok {
		t.Fatal("reescribirCmapTTF falló con un ToUnicode válido")
	}
	tabla, ok := tablaSfnt(salida, "cmap")
	if !ok {
		t.Fatal("la fuente resultante no tiene tabla cmap")
	}
	be := binary.BigEndian
	if be.Uint16(tabla[0:2]) != 0 || be.Uint16(tabla[2:4]) != 1 {
		t.Fatalf("cabecera de cmap inesperada: %v", tabla[:4])
	}
	if be.Uint16(tabla[4:6]) != 3 || be.Uint16(tabla[6:8]) != 1 {
		t.Errorf("se esperaba la plataforma (3,1), hay (%d,%d)", be.Uint16(tabla[4:6]), be.Uint16(tabla[6:8]))
	}
	sub := tabla[12:]
	if be.Uint16(sub[0:2]) != 4 {
		t.Errorf("subtabla de formato %d, quería 4", be.Uint16(sub[0:2]))
	}
	// 'A'..'C' → glifos 3..5 es un solo segmento con delta constante, más el
	// centinela 0xFFFF: dos segmentos.
	if segs := be.Uint16(sub[6:8]) / 2; segs != 2 {
		t.Errorf("segCount = %d, quería 2 (el rango contiguo más el centinela)", segs)
	}
	// El directorio tiene que seguir ordenado o el navegador rechaza la fuente.
	n := int(be.Uint16(salida[4:6]))
	for i := 1; i < n; i++ {
		ant := string(salida[12+(i-1)*16 : 12+(i-1)*16+4])
		act := string(salida[12+i*16 : 12+i*16+4])
		if ant >= act {
			t.Fatalf("directorio desordenado: %q antes de %q", ant, act)
		}
	}
}

func TestReescribirCmapTTFSinToUnicodeFalla(t *testing.T) {
	if _, ok := reescribirCmapTTF(sfntDePrueba(t), nil); ok {
		t.Error("sin /ToUnicode no hay forma de construir la cmap: debe fallar y sustituir")
	}
}

// sfntDePrueba arma un contenedor sfnt mínimo con dos tablas.
func sfntDePrueba(t *testing.T) []byte {
	t.Helper()
	be := binary.BigEndian
	tablas := []struct {
		tag   string
		datos []byte
	}{
		{"glyf", bytes.Repeat([]byte{1}, 40)},
		{"head", bytes.Repeat([]byte{2}, 54)},
	}
	out := make([]byte, 12+len(tablas)*16, 12+len(tablas)*16+256)
	be.PutUint32(out[0:4], 0x00010000)
	be.PutUint16(out[4:6], uint16(len(tablas)))
	for i, tb := range tablas {
		off := len(out)
		out = append(out, tb.datos...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		rec := out[12+i*16:]
		copy(rec[0:4], tb.tag)
		be.PutUint32(rec[8:12], uint32(off))
		be.PutUint32(rec[12:16], uint32(len(tb.datos)))
	}
	return out
}

func tablaSfnt(f []byte, etiqueta string) ([]byte, bool) {
	be := binary.BigEndian
	n := int(be.Uint16(f[4:6]))
	for i := 0; i < n; i++ {
		rec := f[12+i*16 : 12+i*16+16]
		if string(rec[0:4]) != etiqueta {
			continue
		}
		off, lon := int(be.Uint32(rec[8:12])), int(be.Uint32(rec[12:16]))
		if off+lon > len(f) {
			return nil, false
		}
		return f[off : off+lon], true
	}
	return nil, false
}

// --- Robustez ----------------------------------------------------------------

func TestRenderNoSeAtascaConContenidoRaro(t *testing.T) {
	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		for _, basura := range []string{
			"q q q q q q q q", "Q Q Q Q", "BT BT BT (", "((((((((", "1 2 3 4 5 6 7 8 9",
			"/Nada Do", "0 0 0 0 0 0 cm 0 0 1 1 re f", "sh", "gs", ") ) )",
		} {
			svg := renderizar(t, pdfConContenido(t, basura, ""))
			if !strings.HasSuffix(svg, "</svg>") {
				t.Errorf("con %q el SVG salió mal cerrado", basura)
			}
		}
	}()
	select {
	case <-hecho:
	case <-timeoutCorto():
		t.Fatal("el render se colgó con contenido raro")
	}
}

func primerasLineas(s string) string {
	if len(s) > 700 {
		return s[:700] + "…"
	}
	return s
}

// --- Extracción de texto: casos del corpus -----------------------------------

func TestExtraccionTextoType3SinAnchoPorDefecto(t *testing.T) {
	// Type 3 donde 'a' y 'b' se llaman /square y /triangle en /Differences y el
	// espacio (32) está fuera de /FirstChar..LastChar: sin Tw el código 32 tiene
	// anchura 0 (no 500) y los nombres geométricos no sustituyen al ASCII.
	charProc := "<< /Length 16 >>\nstream\n10 0 d0 0 0 m S\nendstream"
	fuente := `<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.1 0 0 0.1 0 0] ` +
		`/FirstChar 97 /LastChar 98 /Widths [10 10] ` +
		`/Encoding << /Type /Encoding /Differences [97 /square /triangle] >> ` +
		`/CharProcs << /square 6 0 R /triangle 6 0 R >> >>`
	d, err := LeerPDF(pdfConContenido(t,
		"BT /F1 10 Tf 12 TL 10 50 Td 10 Tw (a b) Tj T* 0 Tw ( a b) Tj ET",
		`/Resources << /Font << /F1 5 0 R >> >>`,
		fuente, charProc))
	if err != nil {
		t.Fatal(err)
	}
	plano, err := d.TextoPlano(0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(plano) != "a b\nab" {
		t.Errorf("Type 3 con Tw: obtuvo %q, quería %q", strings.TrimSpace(plano), "a b\nab")
	}
}

func TestExtraccionTextoMarcasCombinantesYAvanceNegativo(t *testing.T) {
	// 1) Marca combinante de anchura 0 emitida antes de su base (como hace FOP:
	//    [-600 <0002> 600 <0001>]).
	// 2) Cadena con Tc negativo (avance hacia la izquierda) y ajustes de TJ entre
	//    letras contiguas.
	cmap := "/CIDInit /ProcSet findresource begin 1 begincmap\n" +
		"3 beginbfchar <0001> <0E01> <0002> <0E39> <0003> <0E19> endbfchar\nendcmap end"
	d, err := LeerPDF(pdfConContenido(t,
		"BT /F1 12 Tf 10 50 Td [-600 <0002> 600 <00010003>] TJ ET",
		`/Resources << /Font << /F1 5 0 R >> >>`,
		`<< /Type /Font /Subtype /Type0 /BaseFont /Sub+Thai /Encoding /Identity-H `+
			`/DescendantFonts [6 0 R] /ToUnicode 7 0 R >>`,
		`<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Sub+Thai /DW 600 /W [2 [0]] >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap)))
	if err != nil {
		t.Fatal(err)
	}
	plano, err := d.TextoPlano(0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(plano) != "กูน" {
		t.Errorf("marca antes de base: obtuvo %q, quería %q", strings.TrimSpace(plano), "กูน")
	}
}

func TestExtraccionToUnicodeComprimidoSinFiltroYGlifoIdot(t *testing.T) {
	// Un /ToUnicode comprimido con zlib cuyo diccionario olvida /Filter /FlateDecode,
	// y una fuente simple con /Idot en /Differences.
	cmap := "/CIDInit /ProcSet findresource begin 1 begincmap\n" +
		"1 beginbfchar <01> <00C0> endbfchar\nendcmap end"
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	_, _ = zw.Write([]byte(cmap))
	_ = zw.Close()

	d, err := LeerPDF(pdfConContenido(t,
		"BT /F1 12 Tf 10 50 Td <01> Tj /F2 12 Tf (A) Tj ET",
		`/Resources << /Font << /F1 5 0 R /F2 7 0 R >> >>`,
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode 6 0 R >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", zbuf.Len(), zbuf.String()),
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding << /Differences [65 /Idot] >> >>`))
	if err != nil {
		t.Fatal(err)
	}
	plano, err := d.TextoPlano(0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(plano) != "Àİ" {
		t.Errorf("obtuvo %q, quería %q", strings.TrimSpace(plano), "Àİ")
	}
}

func TestExtraccionCMapGrandeConEntradasNulas(t *testing.T) {
	// Un bloque beginbfchar con más de 200 pares no pierde los primeros por el
	// tope de la pila, y una entrada <0000> no machaca el respaldo de la fuente.
	var sb strings.Builder
	sb.WriteString("/CIDInit /ProcSet findresource begin 1 begincmap\n250 beginbfchar\n")
	sb.WriteString("<0001> <00C0>\n<0002> <0000>\n")
	for i := 3; i <= 250; i++ {
		fmt.Fprintf(&sb, "<%04X> <0041>\n", i)
	}
	sb.WriteString("endbfchar\nendcmap end")
	cmap := sb.String()
	d, err := LeerPDF(pdfConContenido(t,
		"BT /F1 12 Tf 10 50 Td <0001> Tj ET",
		`/Resources << /Font << /F1 5 0 R >> >>`,
		`<< /Type /Font /Subtype /Type0 /BaseFont /Sub /Encoding /Identity-H `+
			`/DescendantFonts [6 0 R] /ToUnicode 7 0 R >>`,
		`<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Sub /DW 600 >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap)))
	if err != nil {
		t.Fatal(err)
	}
	plano, err := d.TextoPlano(0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(plano) != "À" {
		t.Errorf("CMap grande: obtuvo %q, quería %q", strings.TrimSpace(plano), "À")
	}
}

func TestRenderPNGSombreadosDeMallaYFuncion(t *testing.T) {
	// 1) Malla de tipo 4 con dos triángulos independientes (flag=0 en ambos): el
	//    segundo cubre (50..90, 10..90) en verde puro y antes reutilizaba los
	//    vértices del primero.
	// 2) Sombreado de función (ShadingType 1) con calculadora PostScript
	//    (FunctionType 4: `{ pop }`) en rojo.
	vert := func(flag, x, y, r, g, b byte) []byte {
		return []byte{flag, x, y, r, g, b}
	}
	var malla bytes.Buffer
	malla.Write(vert(0, 10, 10, 255, 0, 0))
	malla.Write(vert(0, 40, 10, 255, 0, 0))
	malla.Write(vert(0, 40, 90, 255, 0, 0))
	malla.Write(vert(0, 50, 10, 0, 255, 0))
	malla.Write(vert(0, 90, 10, 0, 255, 0))
	malla.Write(vert(0, 90, 90, 0, 255, 0))

	ps := "{ pop 0 1 }"
	d, err := LeerPDF(pdfConContenido(t,
		"/Pattern cs /P1 scn 0 0 100 100 re f q 10 10 30 30 re W n /ShF sh Q",
		`/Resources << /Pattern << /P1 5 0 R >> /Shading << /ShF 7 0 R >> >>`,
		`<< /Type /Pattern /PatternType 2 /Shading 6 0 R >>`,
		fmt.Sprintf("<< /ShadingType 4 /ColorSpace /DeviceRGB /BitsPerCoordinate 8 "+
			"/BitsPerComponent 8 /BitsPerFlag 8 /Decode [0 255 0 255 0 1 0 1 0 1] /Length %d >>\nstream\n%s\nendstream",
			malla.Len(), malla.String()),
		`<< /ShadingType 1 /ColorSpace /DeviceRGB /Domain [0 1 0 1] /Matrix [30 0 0 30 10 10] /Function 8 0 R >>`,
		fmt.Sprintf("<< /FunctionType 4 /Domain [0 1 0 1] /Range [0 1 0 1 0 1] /Length %d >>\nstream\n%s\nendstream",
			len(ps), ps)))
	if err != nil {
		t.Fatal(err)
	}
	img, err := d.RenderizarPaginaRGBA(0, OpcionesRender{Escala: 1})
	if err != nil {
		t.Fatal(err)
	}
	// En (80, 100-30=70) cae el segundo triángulo (verde puro).
	c := img.RGBAAt(80, 70)
	if c.G < 200 || c.R > 30 {
		t.Errorf("segundo triángulo de malla tipo 4 en (80,70): %+v, quería verde", c)
	}
	// En (25, 100-25=75) cae el sombreado de función con PS `{ pop 0 1 }` (azul > 200).
	cf := img.RGBAAt(25, 75)
	if cf.B < 200 || cf.G > 30 {
		t.Errorf("sombreado tipo 1 con PS en (25,75): %+v, quería azul", cf)
	}
}

func TestRenderPNGCapasOCGApagadasYTextoVertical(t *testing.T) {
	// Una capa OCG listada en /OCProperties /D /OFF no debe tapar el fondo cuando
	// se abre con `/OC /MC1 BDC ... EMC`.
	raw := []byte("%PDF-1.4\n" +
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [6 0 R] /D << /OFF [6 0 R] >> >> >>\nendobj\n" +
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n" +
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 4 0 R " +
		"/Resources << /Properties << /MC1 6 0 R >> >> >>\nendobj\n" +
		"4 0 obj\n<< /Length 62 >>\nstream\n0 1 0 rg 0 0 100 100 re f /OC /MC1 BDC 1 0 0 rg 0 0 100 100 re f EMC\nendstream\nendobj\n" +
		"6 0 obj\n<< /Type /OCG /Name (Oculta) >>\nendobj\n" +
		"trailer\n<< /Root 1 0 R >>\n%%EOF")
	d, err := LeerPDF(raw)
	if err != nil {
		t.Fatal(err)
	}
	img, err := d.RenderizarPaginaRGBA(0, OpcionesRender{Escala: 1})
	if err != nil {
		t.Fatal(err)
	}
	c := img.RGBAAt(50, 50)
	if c.G < 200 || c.R > 30 {
		t.Errorf("la capa OCG en /OFF tapó el lienzo: %+v, quería verde", c)
	}
}
