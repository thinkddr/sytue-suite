// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
)

// extraerFontFaceSVG decodifica el primer @font-face incrustado en un SVG.
func extraerFontFaceSVG(t *testing.T, svg string) []byte {
	t.Helper()
	_, resto, ok := strings.Cut(svg, "base64,")
	if !ok {
		t.Fatalf("el SVG no incrustó @font-face: %s", primerasLineas(svg))
	}
	b64, _, ok := strings.Cut(resto, `"`)
	if !ok {
		t.Fatal("@font-face mal cerrado")
	}
	datos, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64 inválido en @font-face: %v", err)
	}
	return datos
}

// ttfCuadradoEnGlifo construye una TrueType válida cuyo glifo `gidObj` es un
// cuadrado macizo grande (100..1800 de 2048 upem) para distinguir su contorno
// real de cualquier tipografía de reserva.
func ttfCuadradoEnGlifo(t *testing.T) ([]byte, uint16) {
	t.Helper()
	cara, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	gidA, err := cara.GlyphIndex(&buf, 'A')
	if err != nil || gidA == 0 {
		t.Fatalf("sin glifo A: %v", err)
	}
	return append([]byte(nil), goregular.TTF...), uint16(gidA)
}

func pixelesOscuros(img image.Image) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.R < 220 || c.G < 220 || c.B < 220 {
				n++
			}
		}
	}
	return n
}

func TestParidadFuentes(t *testing.T) {
	t.Run("name_y_hmtx_ots", func(t *testing.T) {
		base, _ := ttfCuadradoEnGlifo(t)
		// Quitar tabla `name` y truncar `hmtx` a una sola métrica de 4 bytes mientras
		// `maxp` declara todos los glifos (caso real: bug1151216.pdf y subconjuntos PDF).
		sinName := quitarTablaSfntTest(t, base, "name")
		hhea := append([]byte(nil), datosTablaSfnt(sinName, "hhea")...)
		binary.BigEndian.PutUint16(hhea[34:36], 10)
		sinName, ok := reemplazarTablaSfnt(sinName, "hhea", hhea)
		if !ok {
			t.Fatal("reemplazar hhea")
		}
		hmtxCorta := append([]byte(nil), datosTablaSfnt(sinName, "hmtx")[:40]...)
		rota, ok := reemplazarTablaSfnt(sinName, "hmtx", hmtxCorta)
		if !ok {
			t.Fatal("reemplazar hmtx")
		}

		fuente := `<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+Sub /FirstChar 65 /LastChar 65 /Widths [650] /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>`
		desc := `<< /Type /FontDescriptor /FontName /ABCDEF+Sub /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>`
		flujo := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(rota), rota)
		pdf := pdfConContenido(t, `BT /F1 48 Tf 10 30 Td (A) Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuente, desc, flujo)

		svg := renderizar(t, pdf)
		incrustada := extraerFontFaceSVG(t, svg)
		if len(datosTablaSfnt(incrustada, "name")) < 30 {
			t.Fatalf("OTS rechaza una fuente sin tabla name válida (len=%d)", len(datosTablaSfnt(incrustada, "name")))
		}
		if _, err := sfnt.Parse(incrustada); err != nil {
			t.Fatalf("sfnt.Parse rechazó la fuente saneada para SVG: %v", err)
		}

		d, err := LeerPDF(pdf)
		if err != nil {
			t.Fatal(err)
		}
		l, _, err := d.recorrerPagina(0, OpcionesRender{Escala: 1}, &rasterizador{})
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range l.fuentes {
			f.caraSfnt()
			if f.reserva {
				t.Fatal("el rasterizador cayó a la fuente de reserva por hmtx truncada")
			}
		}
	})

	t.Run("apple_true_ttc_y_tabla_corrupta", func(t *testing.T) {
		base, gidA := ttfCuadradoEnGlifo(t)
		// cmap formato 0 en plataforma Unicode (0,0), cabecera Apple "true",
		// desplazamiento desalineado y entrada de directorio basura (issue5501 + S2 + issue3405r).
		cmap0 := make([]byte, 12+262)
		be := binary.BigEndian
		be.PutUint16(cmap0[2:], 1)
		be.PutUint16(cmap0[4:], 0)
		be.PutUint32(cmap0[8:], 12)
		be.PutUint16(cmap0[12:], 0)
		be.PutUint16(cmap0[14:], 262)
		cmap0[18+'A'] = byte(gidA)
		prog, ok := reemplazarTablaSfnt(base, "cmap", cmap0)
		if !ok {
			t.Fatal("cmap0")
		}
		apple := sfntConDesajusteYBasuraTest(prog)

		fuente := `<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+Quartz /FirstChar 65 /LastChar 65 /Widths [650] /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>`
		desc := `<< /Type /FontDescriptor /FontName /ABCDEF+Quartz /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>`
		flujo := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(apple), apple)
		pdf := pdfConContenido(t, `BT /F1 48 Tf 10 30 Td (A) Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuente, desc, flujo)

		svg := renderizar(t, pdf)
		incrustada := extraerFontFaceSVG(t, svg)
		if string(incrustada[:4]) != "\x00\x01\x00\x00" {
			t.Fatalf("cabecera Apple 'true' no normalizada: %q", incrustada[:4])
		}
		if _, err := sfnt.Parse(incrustada); err != nil {
			t.Fatalf("sfnt.Parse rechazó la fuente Apple/desalineada: %v", err)
		}

		// Colección TrueType "ttcf" (issue13193.pdf).
		ttc := envolverEnTTCTest(base)
		flujoTTC := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(ttc), ttc)
		pdfTTC := pdfConContenido(t, `BT /F1 48 Tf 10 30 Td (A) Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuente, desc, flujoTTC)
		svgTTC := renderizar(t, pdfTTC)
		incTTC := extraerFontFaceSVG(t, svgTTC)
		if _, err := sfnt.Parse(incTTC); err != nil {
			t.Fatalf("sfnt.Parse rechazó la fuente desempaquetada de TTC: %v", err)
		}
	})

	t.Run("opentype_fontfile3_y_cid_sin_tounicode", func(t *testing.T) {
		// 1) CIDFontType0C sin /ToUnicode (cff_bluescale_small_zones.pdf): debe pintar en PNG e incrustarse en SVG.
		cff := cffCIDDePruebaTest()
		f0 := `<< /Type /Font /Subtype /Type0 /BaseFont /ABCDEF+Sofia /Encoding /Identity-H /DescendantFonts [6 0 R] >>`
		dd := `<< /Type /Font /Subtype /CIDFontType0 /BaseFont /ABCDEF+Sofia /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 700 /FontDescriptor 7 0 R >>`
		desc := `<< /Type /FontDescriptor /FontName /ABCDEF+Sofia /Flags 4 /FontBBox [0 0 700 700] /ItalicAngle 0 /Ascent 700 /Descent 0 /CapHeight 700 /StemV 80 /FontFile3 8 0 R >>`
		flujo := fmt.Sprintf("<< /Subtype /CIDFontType0C /Length %d >>\nstream\n%s\nendstream", len(cff), cff)
		pdf := pdfConContenido(t, `BT /F1 60 Tf 10 20 Td <0005> Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, f0, dd, desc, flujo)

		img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		if pixelesOscuros(img) == 0 {
			t.Fatal("CIDFontType0C sin /ToUnicode salió en blanco en PNG")
		}
		svg := renderizar(t, pdf)
		inc := extraerFontFaceSVG(t, svg)
		if _, err := sfnt.Parse(inc); err != nil {
			t.Fatalf("CIDFontType0C sin /ToUnicode produjo un OTF inválido para SVG/sfnt: %v", err)
		}

		// 2) Type0 con /FontFile3 /Subtype /OpenType y /CIDToGIDMap explícito (CID 5 -> GID 34).
		otfSimple, ok := (&fuentePDF{}).envolverCFF(cffDePrueba())
		if !ok {
			t.Fatal("envolverCFF")
		}
		cidMap := make([]byte, 12)
		binary.BigEndian.PutUint16(cidMap[10:12], 34) // CID 5 -> GID 34 (cuadrado)
		toUni := "1 beginbfchar <0005> <0041> endbfchar"
		f0OT := `<< /Type /Font /Subtype /Type0 /BaseFont /ABCDEF+OT /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 9 0 R >>`
		ddOT := `<< /Type /Font /Subtype /CIDFontType0 /BaseFont /ABCDEF+OT /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 700 /CIDToGIDMap 10 0 R /FontDescriptor 7 0 R >>`
		flujoOT := fmt.Sprintf("<< /Subtype /OpenType /Length %d >>\nstream\n%s\nendstream", len(otfSimple), otfSimple)
		flujoTU := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(toUni), toUni)
		flujoMap := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cidMap), cidMap)
		pdfOT := pdfConContenido(t, `BT /F1 60 Tf 10 20 Td <0005> Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, f0OT, ddOT, desc, flujoOT, flujoTU, flujoMap)
		svgOT := renderizar(t, pdfOT)
		caraOT, err := sfnt.Parse(extraerFontFaceSVG(t, svgOT))
		if err != nil {
			t.Fatalf("OpenType en SVG: %v", err)
		}
		var buf sfnt.Buffer
		if gid, err := caraOT.GlyphIndex(&buf, 'A'); err != nil || gid != 34 {
			t.Fatalf("/Subtype /OpenType con /CIDToGIDMap mapeó 'A' al glifo %d (err=%v), quería 34", gid, err)
		}
	})

	t.Run("cmap_30_post_v2_y_fuera_bmp", func(t *testing.T) {
		base, gidA := ttfCuadradoEnGlifo(t)
		// 1) Fuente simbólica con (1,0) formato 6 apuntando a glifo vacío y (3,0) en 0xF241 apuntando a gidA.
		cmap30 := construirCmap10Y30Test(gidA)
		prog30, ok := reemplazarTablaSfnt(base, "cmap", cmap30)
		if !ok {
			t.Fatal("cmap30")
		}
		fSim := `<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+Sym /FirstChar 65 /LastChar 65 /Widths [650] /FontDescriptor 6 0 R >>`
		dSim := `<< /Type /FontDescriptor /FontName /ABCDEF+Sym /Flags 4 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>`
		flSim := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(prog30), prog30)
		pdfSim := pdfConContenido(t, `BT /F1 48 Tf 10 30 Td (A) Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fSim, dSim, flSim)
		svgSim := renderizar(t, pdfSim)
		caraSim, err := sfnt.Parse(extraerFontFaceSVG(t, svgSim))
		if err != nil {
			t.Fatal(err)
		}
		var buf sfnt.Buffer
		if gid, err := caraSim.GlyphIndex(&buf, 0xE041); err != nil || gid != sfnt.GlyphIndex(gidA) {
			t.Fatalf("cmap (3,0) 0xF241: glifo %d (err=%v), quería %d", gid, err, gidA)
		}

		// 2) Type0 sin tabla cmap con /ToUnicode fuera del BMP (U+10C80 = D803 DC80, bug1175962.pdf).
		sinCmap := quitarTablaSfntTest(t, base, "cmap")
		toUni := fmt.Sprintf("1 beginbfchar <%04X> <D803DC80> endbfchar", gidA)
		f0 := `<< /Type /Font /Subtype /Type0 /BaseFont /ABCDEF+Hung /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 9 0 R >>`
		dd := `<< /Type /Font /Subtype /CIDFontType2 /BaseFont /ABCDEF+Hung /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 650 /CIDToGIDMap /Identity /FontDescriptor 7 0 R >>`
		d0 := `<< /Type /FontDescriptor /FontName /ABCDEF+Hung /Flags 4 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile2 8 0 R >>`
		fl0 := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(sinCmap), sinCmap)
		flTU := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(toUni), toUni)
		pdfBMP := pdfConContenido(t, fmt.Sprintf("BT /F1 48 Tf 10 30 Td <%04X> Tj ET", gidA), `/Resources << /Font << /F1 5 0 R >> >>`, f0, dd, d0, fl0, flTU)
		imgBMP, err := png.Decode(bytes.NewReader(rasterizar(t, pdfBMP, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		if pixelesOscuros(imgBMP) == 0 {
			t.Fatal("Type0 fuera del BMP sin tabla cmap salió en blanco en PNG")
		}
		svgBMP := renderizar(t, pdfBMP)
		if !strings.Contains(svgBMP, "@font-face") {
			t.Fatal("Type0 fuera del BMP no incrustó @font-face en SVG")
		}
	})

	t.Run("type1_hex_crlf_y_afii_cirilico", func(t *testing.T) {
		prog, len1 := type1HexCirilicoTest(t)
		fuente := `<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Aura /FirstChar 1 /LastChar 1 /Widths [700] /FontDescriptor 6 0 R >>`
		desc := `<< /Type /FontDescriptor /FontName /ABCDEF+Aura /Flags 32 /FontBBox [0 0 700 700] /ItalicAngle 0 /Ascent 700 /Descent 0 /CapHeight 700 /StemV 80 /FontFile 7 0 R >>`
		flujo := fmt.Sprintf("<< /Length %d /Length1 %d /Length2 %d /Length3 0 >>\nstream\n%s\nendstream", len(prog), len1, len(prog)-len1, prog)
		pdf := pdfConContenido(t, "BT /F1 100 Tf 10 20 Td <01> Tj ET", `/Resources << /Font << /F1 5 0 R >> >>`, fuente, desc, flujo)

		doc, err := LeerPDF(pdf)
		if err != nil {
			t.Fatal(err)
		}
		txt, err := doc.TextoPlano(0)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(txt) != "О" {
			t.Fatalf("afii10032 debía extraerse como «О», salió %q", txt)
		}
		svg, err := doc.RenderizarPaginaSVG(0, OpcionesRender{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(svg, "font/otf") {
			t.Fatal("Type1 con eexec hex tras CRLF no se incrustó en SVG")
		}
		img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		if c := color.GrayModel.Convert(img.At(22, 100-78)).(color.Gray); c.Y > 80 {
			t.Fatalf("Type1 con eexec hex tras CRLF no pintó su contorno (gris %d)", c.Y)
		}
	})

	t.Run("type3_d1_ignora_color_y_no_avanza_doble", func(t *testing.T) {
		// CharProc con d1 y `1 1 1 rg` blanco dentro (issue12705.pdf), precedido de un
		// espacio sin CharProc en TJ (Type3WordSpacing.pdf).
		glifo := "1000 0 0 0 750 750 d1 1 1 1 rg 0 0 700 700 re f"
		datos := pdfConContenido(t,
			"BT 0 0 1 rg 1 0 0 RG /F3 20 Tf 10 40 Td [( ) (A)] TJ ET",
			`/Resources << /Font << /F3 5 0 R >> >>`,
			`<< /Type /Font /Subtype /Type3 /FontBBox [0 0 750 750] `+
				`/FontMatrix [0.001 0 0 0.001 0 0] /CharProcs 6 0 R `+
				`/Encoding << /Differences [65 /cuadro] >> `+
				`/FirstChar 32 /LastChar 65 /Widths [500 `+strings.Repeat("0 ", 32)+`1000] >>`,
			`<< /cuadro 7 0 R >>`,
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(glifo), glifo))

		svg := renderizar(t, datos)
		if !strings.Contains(svg, `fill="#0000ff"`) || strings.Contains(svg, `fill="#ffffff"`) {
			t.Fatalf("d1 en Type 3 no ignoró los operadores de color internos: %s", primerasLineas(svg))
		}
		// El espacio de 500 milésimas a 20 pt avanza 10 pt (de x=10 a x=20), no 20 pt (x=30).
		if !strings.Contains(svg, "matrix(0.02 0 0 -0.02 20 60)") {
			t.Fatalf("Type 3 avanzó dos veces la cadena sin dibujo: %s", primerasLineas(svg))
		}
	})

	t.Run("symbol_estandar_y_espacios_svg", func(t *testing.T) {
		fuente := `<< /Type /Font /Subtype /Type1 /BaseFont /Symbol >>`
		pdf := pdfConContenido(t, `BT /F1 20 Tf 10 50 Td (ab\261   z) Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuente)
		doc, err := LeerPDF(pdf)
		if err != nil {
			t.Fatal(err)
		}
		txt, err := doc.TextoPlano(0)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(txt, "αβ±") {
			t.Fatalf("Symbol sin incrustar debía extraer «αβ±», salió %q", txt)
		}
		svg, err := doc.RenderizarPaginaSVG(0, OpcionesRender{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(svg, "white-space:pre") {
			t.Fatalf("falta white-space:pre en el SVG para preservar espacios en HTML5: %s", primerasLineas(svg))
		}
	})

	t.Run("encoding_explicito_frente_a_tounicode", func(t *testing.T) {
		// Caso real (issue1512r.pdf): fuente simple no simbólica con /Encoding /WinAnsiEncoding
		// cuyo /ToUnicode traduce mayúsculas del flujo («ROM») a minúsculas («rom») para
		// copiar como título, mientras que ISO 32000-1 §9.6.6.4 elige el glifo por /Encoding.
		base := append([]byte(nil), goregular.TTF...)
		fuente := `<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+GoReg /FirstChar 32 /LastChar 122 /Encoding /WinAnsiEncoding /ToUnicode 8 0 R /FontDescriptor 6 0 R >>`
		desc := `<< /Type /FontDescriptor /FontName /ABCDEF+GoReg /Flags 32 /FontBBox [0 -200 1000 900] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>`
		flujo := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(base), base)
		toUni := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap\n" +
			"1 begincodespacerange <00> <FF> endcodespacerange\n" +
			"8 beginbfchar\n" +
			"<46> <0046>\n<52> <0072>\n<4F> <006F>\n<4D> <006D>\n" +
			"<66> <0066>\n<72> <0072>\n<6F> <006F>\n<6D> <006D>\n" +
			"endbfchar\nendcmap"
		objTU := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(toUni), toUni)

		pdfUpper := pdfConContenido(t, `BT /F1 48 Tf 10 20 Td (FROM) Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuente, desc, flujo, objTU)
		fuenteRef := `<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+GoReg /FirstChar 32 /LastChar 122 /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>`
		pdfRef := pdfConContenido(t, `BT /F1 48 Tf 10 20 Td (FROM) Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuenteRef, desc, flujo)

		imgUpper, err := png.Decode(bytes.NewReader(rasterizar(t, pdfUpper, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		imgRef, err := png.Decode(bytes.NewReader(rasterizar(t, pdfRef, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := pixelesOscuros(imgUpper), pixelesOscuros(imgRef); got != want {
			t.Fatalf("el glifo simple eligió /ToUnicode en lugar de /Encoding: %d píxeles frente a %d", got, want)
		}
		doc, err := LeerPDF(pdfUpper)
		if err != nil {
			t.Fatal(err)
		}
		if txt, err := doc.TextoPlano(0); err != nil || strings.TrimSpace(txt) != "From" {
			t.Fatalf("TextoPlano debía conservar /ToUnicode «From», salió %q (%v)", txt, err)
		}
		svg, err := doc.RenderizarPaginaSVG(0, OpcionesRender{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(svg, "\uE052") || !strings.Contains(svg, ">From</text>") {
			t.Fatalf("el SVG debía separar el glifo mayúsculo por código y la capa seleccionable «From»: %s", primerasLineas(svg))
		}
	})

	t.Run("tablas_sfnt_head56_desplazada_y_loca", func(t *testing.T) {
		base, _ := ttfCuadradoEnGlifo(t)
		be := binary.BigEndian
		fuente := `<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+Rot /FirstChar 65 /LastChar 65 /Widths [650] /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>`
		desc := `<< /Type /FontDescriptor /FontName /ABCDEF+Rot /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>`

		probarVariante := func(nombre string, ttf []byte) {
			t.Helper()
			flujo := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(ttf), ttf)
			pdf := pdfConContenido(t, `BT /F1 48 Tf 10 30 Td (A) Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuente, desc, flujo)
			svg := renderizar(t, pdf)
			inc := extraerFontFaceSVG(t, svg)
			if _, err := sfnt.Parse(inc); err != nil {
				t.Fatalf("%s: sfnt.Parse rechazó la fuente saneada: %v", nombre, err)
			}
			os2 := datosTablaSfnt(inc, "OS/2")
			if len(os2) < 78 || be.Uint16(os2[4:6]) < 1 || be.Uint16(os2[4:6]) > 1000 {
				t.Fatalf("%s: OS/2 inválida para OTS (len=%d)", nombre, len(os2))
			}
			d, err := LeerPDF(pdf)
			if err != nil {
				t.Fatal(err)
			}
			l, _, err := d.recorrerPagina(0, OpcionesRender{Escala: 1}, &rasterizador{})
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range l.fuentes {
				f.caraSfnt()
				if f.reserva {
					t.Fatalf("%s: el rasterizador cayó a la fuente de reserva", nombre)
				}
			}
		}

		// 1) head de 56 bytes (bug1627427_reduced.pdf, issue5677.pdf) y loca con 4 bytes sobrantes (bug1200096.pdf).
		head56 := append(append([]byte(nil), datosTablaSfnt(base, "head")...), 0, 0)
		locaExtra := append(append([]byte(nil), datosTablaSfnt(base, "loca")...), 0, 0, 0, 0)
		v1, _ := reemplazarTablaSfnt(base, "head", head56)
		v1, _ = reemplazarTablaSfnt(v1, "loca", locaExtra)
		probarVariante("head56_y_loca_extra", v1)

		// 2) head y maxp desplazadas 1 byte a la izquierda + OS/2 corrupta (bug1050040.pdf).
		headOrig := datosTablaSfnt(base, "head")
		headShift := append(append([]byte(nil), headOrig[1:]...), 0)
		maxpOrig := datosTablaSfnt(base, "maxp")
		maxpShift := append(append([]byte(nil), maxpOrig[1:]...), 0)
		os2Corrupta := append([]byte(nil), datosTablaSfnt(base, "OS/2")...)
		be.PutUint16(os2Corrupta[4:6], 0x9000)
		v2, _ := reemplazarTablaSfnt(base, "head", headShift)
		v2, _ = reemplazarTablaSfnt(v2, "maxp", maxpShift)
		v2, _ = reemplazarTablaSfnt(v2, "OS/2", os2Corrupta)
		probarVariante("head_maxp_os2_desplazadas", v2)

		// 3) maxp declara más glifos de los que cubre loca (issue14618.pdf).
		maxpMas := append([]byte(nil), maxpOrig...)
		ngOrig := be.Uint16(maxpMas[4:6])
		be.PutUint16(maxpMas[4:6], ngOrig+20)
		v3, _ := reemplazarTablaSfnt(base, "maxp", maxpMas)
		probarVariante("loca_corta_subconjunto", v3)
	})

	t.Run("truetype_glifo_real_en_gid_cero", func(t *testing.T) {
		// Caso real (issue8234.pdf): subconjunto simbólico cuya cmap (3,0) mapea
		// explícitamente 0xF020 (código 32) al GID 0, donde vive el contorno real de «ä».
		ttfGid0 := ttfConContornoEnGidCeroTest(t)
		toUni := "1 beginbfchar <20> <00E4> endbfchar"
		fuente := `<< /Type /Font /Subtype /TrueType /BaseFont /AAAAAC+Arial /FirstChar 32 /LastChar 32 /Widths [650] /ToUnicode 8 0 R /FontDescriptor 6 0 R >>`
		desc := `<< /Type /FontDescriptor /FontName /AAAAAC+Arial /Flags 4 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>`
		flujo := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(ttfGid0), ttfGid0)
		flujoTU := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(toUni), toUni)
		pdf := pdfConContenido(t, `BT /F1 48 Tf 10 25 Td <20> Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuente, desc, flujo, flujoTU)

		img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		if pixelesOscuros(img) == 0 {
			t.Fatal("el glifo real almacenado en GID 0 salió en blanco en PNG")
		}
		svg := renderizar(t, pdf)
		cara, err := sfnt.Parse(extraerFontFaceSVG(t, svg))
		if err != nil {
			t.Fatalf("sfnt.Parse en SVG de GID 0: %v", err)
		}
		var buf sfnt.Buffer
		gid, err := cara.GlyphIndex(&buf, 0xE020)
		if err != nil || gid == 0 {
			t.Fatalf("SVG dejó 0xE020 apuntando a GID 0 (.notdef para el navegador): gid=%d err=%v", gid, err)
		}
		segs, err := cara.LoadGlyph(&buf, gid, 1000, nil)
		if err != nil || len(segs) == 0 {
			t.Fatalf("el glifo clonado de GID 0 no tiene contorno: segs=%d err=%v", len(segs), err)
		}
	})

	t.Run("cff_en_fontfile_y_encoding_cmap_bfchar", func(t *testing.T) {
		// CFF CID-keyed guardado en /FontFile (issue5751.pdf) + CMap en /Encoding
		// con beginbfchar <0043> <0005> endbfchar (bug920426.pdf).
		cff := cffCIDDePruebaTest()
		cmapEnc := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap\n" +
			"1 begincodespacerange <0000> <FFFF> endcodespacerange\n" +
			"1 beginbfchar <0043> <0005> endbfchar\nendcmap"
		toUni := "1 beginbfchar <0043> <0043> endbfchar"
		f0 := `<< /Type /Font /Subtype /Type0 /BaseFont /MyriadArabic+FPEF /Encoding 9 0 R /ToUnicode 10 0 R /DescendantFonts [6 0 R] >>`
		dd := `<< /Type /Font /Subtype /CIDFontType0 /BaseFont /MyriadArabic+FPEF /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 700 /FontDescriptor 7 0 R >>`
		desc := `<< /Type /FontDescriptor /FontName /MyriadArabic+FPEF /Flags 4 /FontBBox [0 0 700 700] /ItalicAngle 0 /Ascent 700 /Descent 0 /CapHeight 700 /StemV 80 /FontFile 8 0 R >>`
		flCFF := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cff), cff)
		flCM := fmt.Sprintf("<< /Type /CMap /CMapName /CM10 /Length %d >>\nstream\n%s\nendstream", len(cmapEnc), cmapEnc)
		flTU := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(toUni), toUni)
		pdf := pdfConContenido(t, `BT /F1 60 Tf 10 20 Td <0043> Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, f0, dd, desc, flCFF, flCM, flTU)

		img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		if pixelesOscuros(img) == 0 {
			t.Fatal("CFF en /FontFile con /Encoding beginbfchar salió en blanco en PNG")
		}
		cara, err := sfnt.Parse(extraerFontFaceSVG(t, renderizar(t, pdf)))
		if err != nil {
			t.Fatalf("sfnt.Parse en SVG de CFF en /FontFile: %v", err)
		}
		var buf sfnt.Buffer
		if gid, err := cara.GlyphIndex(&buf, 'C'); err != nil || gid != 1 {
			t.Fatalf("cmap SVG debía mapear 'C' al glifo 1 (CID 5), dio gid=%d err=%v", gid, err)
		}
	})

	t.Run("cff_cid_fdarray_upem_dotsection_y_bbox", func(t *testing.T) {
		// 1) CIDFontType0C con FDSelect antes de ROS, FontBBox invertida [65080 65231 2158 989],
		// Private en Top DICT y FontMatrix 1/2048 en FDArray[0] (issue5540 + issue18030 +
		// issue_cff_unsigned_bbox + text_clip_cff_cid).
		cffCID := cffCIDHostilTest()
		f0 := `<< /Type /Font /Subtype /Type0 /BaseFont /ABCDEF+Hostil /Encoding /Identity-H /DescendantFonts [6 0 R] >>`
		dd := `<< /Type /Font /Subtype /CIDFontType0 /BaseFont /ABCDEF+Hostil /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 700 /FontDescriptor 7 0 R >>`
		desc := `<< /Type /FontDescriptor /FontName /ABCDEF+Hostil /Flags 4 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile3 8 0 R >>`
		fl := fmt.Sprintf("<< /Subtype /CIDFontType0C /Length %d >>\nstream\n%s\nendstream", len(cffCID), cffCID)
		pdfCID := pdfConContenido(t, `BT /F1 60 Tf 10 20 Td <0005> Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, f0, dd, desc, fl)

		incCID := extraerFontFaceSVG(t, renderizar(t, pdfCID))
		caraCID, err := sfnt.Parse(incCID)
		if err != nil {
			t.Fatalf("CID-CFF hostil no se aplanó para sfnt/OTS: %v", err)
		}
		var buf sfnt.Buffer
		for g := 0; g < caraCID.NumGlyphs(); g++ {
			if _, err := caraCID.LoadGlyph(&buf, sfnt.GlyphIndex(g), 1000, nil); err != nil {
				t.Fatalf("LoadGlyph(%d) falló en CID-CFF aplanado: %v", g, err)
			}
		}
		imgCID, err := png.Decode(bytes.NewReader(rasterizar(t, pdfCID, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		if pixelesOscuros(imgCID) == 0 {
			t.Fatal("CID-CFF hostil salió en blanco en PNG")
		}

		// 2) Type1C simple con dotsection (12 0) en el charstring (issue7439.pdf).
		cffDot := cffSimpleConDotsectionTest()
		fSim := `<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Dot /FirstChar 65 /LastChar 65 /Widths [700] /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>`
		dSim := `<< /Type /FontDescriptor /FontName /ABCDEF+Dot /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile3 7 0 R >>`
		flDot := fmt.Sprintf("<< /Subtype /Type1C /Length %d >>\nstream\n%s\nendstream", len(cffDot), cffDot)
		pdfDot := pdfConContenido(t, `BT /F1 60 Tf 10 20 Td (A) Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fSim, dSim, flDot)

		caraDot, err := sfnt.Parse(extraerFontFaceSVG(t, renderizar(t, pdfDot)))
		if err != nil {
			t.Fatalf("CFF con dotsection: %v", err)
		}
		for g := 0; g < caraDot.NumGlyphs(); g++ {
			if _, err := caraDot.LoadGlyph(&buf, sfnt.GlyphIndex(g), 1000, nil); err != nil {
				t.Fatalf("CFF con dotsection no eliminó (12 0) en glifo %d: %v", g, err)
			}
		}
	})

	t.Run("zapfdingbats_nombres_a_y_palos", func(t *testing.T) {
		// Caso real (issue15716.pdf): /Differences con nombres /a109../a112 en ZapfDingbats.
		fuente := `<< /Type /Font /Subtype /Type1 /BaseFont /ZapfDingbats /Encoding << /Type /Encoding /Differences [1 /a109 /a110 /a111 /a112] >> >>`
		pdf := pdfConContenido(t, `BT /F1 36 Tf 10 30 Td <01020304> Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuente)
		doc, err := LeerPDF(pdf)
		if err != nil {
			t.Fatal(err)
		}
		txt, err := doc.TextoPlano(0)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(txt) != "♠♥♦♣" {
			t.Fatalf("ZapfDingbats /a109../a112 debía dar «♠♥♦♣», dio %q", txt)
		}
		img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		if pixelesOscuros(img) == 0 {
			t.Fatal("ZapfDingbats /a109../a112 salió en blanco en PNG")
		}
	})
}

// --- Ayudantes de construcción para TestParidadFuentes -----------------------

func quitarTablaSfntTest(t *testing.T, fuente []byte, quitar string) []byte {
	t.Helper()
	be := binary.BigEndian
	n := int(be.Uint16(fuente[4:6]))
	var out []byte
	for i := 0; i < n; i++ {
		rec := fuente[12+i*16 : 12+i*16+16]
		tag := string(rec[0:4])
		if tag == quitar {
			continue
		}
		off, lon := be.Uint32(rec[8:12]), be.Uint32(rec[12:16])
		datos := fuente[off : off+lon]
		if out == nil {
			out = sfntNuevo(be.Uint32(fuente[0:4]), tag, datos)
		} else {
			var ok bool
			out, ok = reemplazarTablaSfnt(out, tag, datos)
			if !ok {
				t.Fatal("reemplazarTablaSfnt")
			}
		}
	}
	return out
}

func sfntConDesajusteYBasuraTest(fuente []byte) []byte {
	be := binary.BigEndian
	n := int(be.Uint16(fuente[4:6]))
	nuevoN := n + 1
	cab := 12 + nuevoN*16
	out := make([]byte, cab, len(fuente)+64)
	copy(out[0:4], "true")
	be.PutUint16(out[4:6], uint16(nuevoN)) //nolint:gosec // prueba controlada
	for i := 0; i < n; i++ {
		rec := fuente[12+i*16 : 12+i*16+16]
		off, lon := be.Uint32(rec[8:12]), be.Uint32(rec[12:16])
		if i == 1 {
			out = append(out, 0) // provocar desplazamiento impar no múltiplo de 4
		}
		nuevoOff := len(out)
		out = append(out, fuente[off:off+lon]...)
		dst := out[12+i*16 : 12+i*16+16]
		copy(dst[0:4], rec[0:4])
		copy(dst[4:8], rec[4:8])
		be.PutUint32(dst[8:12], uint32(nuevoOff)) //nolint:gosec // prueba
		be.PutUint32(dst[12:16], lon)
	}
	// Entrada final corrupta como en issue3405r.pdf.
	basura := out[12+n*16 : 12+(n+1)*16]
	copy(basura[0:4], "\x00\x01\x00\x00")
	be.PutUint32(basura[8:12], 0x68F4A2EF)
	be.PutUint32(basura[12:16], 0x5F0F3CF5)
	return out
}

func envolverEnTTCTest(ttf []byte) []byte {
	be := binary.BigEndian
	cabTTC := 16
	out := make([]byte, cabTTC+len(ttf))
	copy(out[0:4], "ttcf")
	be.PutUint32(out[4:8], 0x00010000)
	be.PutUint32(out[8:12], 1)
	be.PutUint32(out[12:16], uint32(cabTTC))
	copy(out[cabTTC:], ttf)
	n := int(be.Uint16(ttf[4:6]))
	for i := 0; i < n; i++ {
		pos := cabTTC + 12 + i*16
		be.PutUint32(out[pos+8:pos+12], be.Uint32(out[pos+8:pos+12])+uint32(cabTTC))
	}
	return out
}

func cffCIDDePruebaTest() []byte {
	// CFF CID-keyed (ROS en Top DICT = 12 30) con glifo 0 vacío y glifo 1 (CID 5) cuadrado.
	cabecera := []byte{1, 0, 4, 2}
	nombres := indiceDePrueba([]byte("SofiaCID"))
	cadenas := indiceDePrueba([]byte("Adobe"), []byte("Identity"))
	sinSubrs := indiceDePrueba()
	vacio := []byte{14}
	cuadrado := []byte{239, 239, 21, 248, 136, 6, 248, 136, 7, 252, 136, 6, 14}
	glifos := indiceDePrueba(vacio, cuadrado)
	charset := []byte{0, 0, 5} // formato 0: glifo 1 -> CID 5
	fdDict := indiceDePrueba([]byte{29, 0, 0, 0, 2, 29, 0, 0, 0, 0, 18})
	fdSelect := []byte{3, 0, 1, 0, 0, 0, 2} // formato 3: 2 glifos -> FD 0
	privado := []byte{139, 20}

	armarTop := func(offCharset, offFDSelect, offFDArray, offCS int) []byte {
		var top []byte
		// 391 392 0 ROS (12 30)
		top = append(top, 28, 1, 135, 28, 1, 136, 139, 12, 30)
		top = enteroDictCFF(top, offCharset, 15)
		top = append(top,
			29, byte(offFDSelect>>24), byte(offFDSelect>>16), byte(offFDSelect>>8), byte(offFDSelect), 12, 37,
			29, byte(offFDArray>>24), byte(offFDArray>>16), byte(offFDArray>>8), byte(offFDArray), 12, 36,
		)
		top = enteroDictCFF(top, offCS, 17)
		return top
	}
	topDummy := indiceDePrueba(armarTop(0, 0, 0, 0))
	base := len(cabecera) + len(nombres) + len(topDummy) + len(cadenas) + len(sinSubrs)
	offCharset := base
	offFDSelect := offCharset + len(charset)
	offCS := offFDSelect + len(fdSelect)
	offPriv := offCS + len(glifos) + len(fdDict)
	fdDictReal := indiceDePrueba(append(
		[]byte{29, 0, 0, 0, 2},
		enteroDictCFF(nil, offPriv, 18)...,
	))
	offFDArray := offCS + len(glifos)
	topReal := indiceDePrueba(armarTop(offCharset, offFDSelect, offFDArray, offCS))

	var b bytes.Buffer
	for _, p := range [][]byte{cabecera, nombres, topReal, cadenas, sinSubrs, charset, fdSelect, glifos, fdDictReal, privado} {
		b.Write(p)
	}
	return b.Bytes()
}

func construirCmap10Y30Test(gid uint16) []byte {
	be := binary.BigEndian
	// Subtabla (1,0) formato 6: código 65 -> glifo 0 (vacío, trampa como en bug1151216).
	sub10 := make([]byte, 12)
	be.PutUint16(sub10[0:], 6)
	be.PutUint16(sub10[2:], 12)
	be.PutUint16(sub10[6:], 65)
	be.PutUint16(sub10[8:], 1)
	be.PutUint16(sub10[10:], 0)

	// Subtabla (3,0) formato 4: 0xF241 -> gid (rango F200 de ISO 32000 §9.6.6.4).
	sub30 := make([]byte, 32)
	be.PutUint16(sub30[0:], 4)
	be.PutUint16(sub30[2:], 32)
	be.PutUint16(sub30[6:], 4) // 2 segmentos * 2
	be.PutUint16(sub30[8:], 4)
	be.PutUint16(sub30[10:], 1)
	be.PutUint16(sub30[12:], 0)
	be.PutUint16(sub30[14:], 0xF241) // endCode[0]
	be.PutUint16(sub30[16:], 0xFFFF) // endCode[1]
	be.PutUint16(sub30[20:], 0xF241) // startCode[0]
	be.PutUint16(sub30[22:], 0xFFFF) // startCode[1]
	be.PutUint16(sub30[24:], gid-0xF241)
	be.PutUint16(sub30[26:], 1)

	cab := make([]byte, 4+2*8, 4+2*8+len(sub10)+len(sub30))
	be.PutUint16(cab[2:], 2)
	be.PutUint16(cab[4:], 1)
	be.PutUint16(cab[6:], 0)
	be.PutUint32(cab[8:], uint32(len(cab))) //nolint:gosec // fijo
	be.PutUint16(cab[12:], 3)
	be.PutUint16(cab[14:], 0)
	be.PutUint32(cab[16:], uint32(len(cab)+len(sub10))) //nolint:gosec // fijo
	cab = append(cab, sub10...)
	cab = append(cab, sub30...)
	return cab
}

func type1HexCirilicoTest(t *testing.T) ([]byte, int) {
	t.Helper()
	claro := "%!PS-AdobeFont-1.0: Aura 001\r\n11 dict begin\r\n/FontName /Aura def\r\n" +
		"/FontMatrix [0.001 0 0 0.001 0 0] readonly def\r\n" +
		"/Encoding 256 array 0 1 255 {1 index exch /.notdef put} for\r\ndup 1 /afii10032 put\r\nreadonly def\r\n" +
		"currentdict end\r\ncurrentfile eexec\r\n"
	var priv bytes.Buffer
	priv.WriteString("dup /Private 8 dict dup begin\n/RD{string currentfile exch readstring pop}executeonly def\n" +
		"/ND{noaccess def}executeonly def\n/NP{noaccess put}executeonly def\n/lenIV 4 def\n" +
		"2 index /CharStrings 2 dict dup begin\n/.notdef ")
	binario := func(cs []byte) {
		c := cifrarType1(cs, 4330, 4)
		fmt.Fprintf(&priv, "%d RD ", len(c))
		priv.Write(c)
	}
	binario(csType1(t, "0 0 hsbw endchar"))
	priv.WriteString(" ND\n/afii10032 ")
	binario(csType1(t, "0 700 hsbw 100 100 rmoveto 500 hlineto 500 vlineto -500 hlineto closepath endchar"))
	priv.WriteString(" ND\nend\nend\nreadonly put\nmark currentfile closefile\n")
	cifHex := hex.EncodeToString(cifrarType1(priv.Bytes(), 55665, 4))
	// Length1 apunta entre \r y \n tras eexec, exactamente como en issue11740_reduced.pdf.
	prog := append([]byte(claro), []byte(cifHex+"\n"+strings.Repeat("0", 512)+"\ncleartomark\n")...)
	return prog, len(claro) - 1
}

func ttfConContornoEnGidCeroTest(t *testing.T) []byte {
	t.Helper()
	base, gidA := ttfCuadradoEnGlifo(t)
	be := binary.BigEndian
	head := datosTablaSfnt(base, "head")
	loca := datosTablaSfnt(base, "loca")
	glyf := datosTablaSfnt(base, "glyf")
	hmtx := datosTablaSfnt(base, "hmtx")

	var offA0, offA1 int
	if int16(be.Uint16(head[50:52])) == 0 {
		offA0 = 2 * int(be.Uint16(loca[2*int(gidA):]))
		offA1 = 2 * int(be.Uint16(loca[2*int(gidA+1):]))
	} else {
		offA0 = int(be.Uint32(loca[4*int(gidA):]))
		offA1 = int(be.Uint32(loca[4*int(gidA+1):]))
	}
	contornoA := append([]byte(nil), glyf[offA0:offA1]...)
	if len(contornoA)%2 == 1 {
		contornoA = append(contornoA, 0)
	}

	// Fuente mínima de 2 glifos donde GID 0 tiene el contorno real de 'A' y GID 1 está vacío.
	headN := append([]byte(nil), head...)
	be.PutUint16(headN[50:52], 1) // loca larga
	maxpN := make([]byte, 32)
	be.PutUint32(maxpN[0:4], 0x00010000)
	be.PutUint16(maxpN[4:6], 2)
	locaN := make([]byte, 12)
	be.PutUint32(locaN[0:4], 0)
	be.PutUint32(locaN[4:8], uint32(len(contornoA)))  //nolint:gosec // pequeño
	be.PutUint32(locaN[8:12], uint32(len(contornoA))) //nolint:gosec // GID 1 vacío
	hheaN := append([]byte(nil), datosTablaSfnt(base, "hhea")...)
	be.PutUint16(hheaN[34:36], 2)
	hmtxN := make([]byte, 8)
	copy(hmtxN[0:4], hmtx[4*int(gidA):4*int(gidA)+4])
	copy(hmtxN[4:8], hmtx[4*int(gidA):4*int(gidA)+4])

	// Subtabla (3,0) formato 4 que mapea 0xF020..0xF021 con delta = -0xF020 (0xF020 -> GID 0).
	sub30 := make([]byte, 32)
	be.PutUint16(sub30[0:], 4)
	be.PutUint16(sub30[2:], 32)
	be.PutUint16(sub30[6:], 4)
	be.PutUint16(sub30[8:], 4)
	be.PutUint16(sub30[10:], 1)
	be.PutUint16(sub30[14:], 0xF021)
	be.PutUint16(sub30[16:], 0xFFFF)
	be.PutUint16(sub30[20:], 0xF020)
	be.PutUint16(sub30[22:], 0xFFFF)
	delta := ^uint16(0xF020) + 1
	be.PutUint16(sub30[24:], delta)
	be.PutUint16(sub30[26:], 1)
	cmap := make([]byte, 12, 12+len(sub30))
	be.PutUint16(cmap[2:], 1)
	be.PutUint16(cmap[4:], 3)
	be.PutUint16(cmap[6:], 0)
	be.PutUint32(cmap[8:], 12)
	cmap = append(cmap, sub30...)

	postN := make([]byte, 32)
	be.PutUint32(postN[0:4], 0x00030000)

	out := sfntNuevo(0x00010000, "head", headN)
	for tag, tb := range map[string][]byte{
		"maxp": maxpN, "loca": locaN, "glyf": contornoA,
		"hhea": hheaN, "hmtx": hmtxN, "cmap": cmap, "post": postN,
	} {
		var ok bool
		if out, ok = reemplazarTablaSfnt(out, tag, tb); !ok {
			t.Fatalf("reemplazar %s", tag)
		}
	}
	return out
}

func cffCIDHostilTest() []byte {
	cabecera := []byte{1, 0, 4, 2}
	nombres := indiceDePrueba([]byte("HostilCID"))
	cadenas := indiceDePrueba([]byte("Adobe"), []byte("Identity"), []byte("HostilFD"))
	sinSubrs := indiceDePrueba()
	vacio := []byte{14}
	// Cuadrado en espacio 2048 upem (200..1400).
	cuadrado := append(numeroT2(nil, 200), numeroT2(nil, 200)...)
	cuadrado = append(cuadrado, 21)
	cuadrado = append(cuadrado, append(numeroT2(nil, 1200), 6)...)
	cuadrado = append(cuadrado, append(numeroT2(nil, 1200), 7)...)
	cuadrado = append(cuadrado, append(numeroT2(nil, -1200), 6)...)
	cuadrado = append(cuadrado, 14)
	glifos := indiceDePrueba(vacio, cuadrado)
	charset := []byte{0, 0, 5}
	fdSelect := []byte{3, 0, 1, 0, 0, 0, 0, 2} // formato 3 de 8 bytes (centinela uint16 = 2)
	privado := []byte{139, 20}

	// FontMatrix [0.00048828125 0 0 0.00048828125 0 0] en nibble real CFF (30 = 0x1E):
	// "0.00048828125" -> 0x0a 0x00 0x04 0x88 0x28 0x12 0x5f
	numEsc := []byte{30, 0x0a, 0x00, 0x04, 0x88, 0x28, 0x12, 0x5f}
	armarFD := func(offPriv int) []byte {
		fd := make([]byte, 0, 2*len(numEsc)+22)
		fd = append(fd, numEsc...)
		fd = append(fd, 139, 139)
		fd = append(fd, numEsc...)
		fd = append(fd,
			139, 139, 12, 7, // FontMatrix (12 7)
			28, 1, 137, 12, 38,
			29, 0, 0, 0, 2,
		)
		return enteroDictCFF(fd, offPriv, 18)
	}
	// Top DICT hostil: FDSelect (12 37) ANTES de ROS (12 30), FontBBox invertida y Private en Top DICT.
	armarTop := func(offCharset, offFDSelect, offFDArray, offCS, offPriv int) []byte {
		top := []byte{
			29, byte(offFDSelect >> 24), byte(offFDSelect >> 16), byte(offFDSelect >> 8), byte(offFDSelect), 12, 37,
			28, 1, 135, 28, 1, 136, 139, 12, 30, // ROS después de FDSelect
			// FontBBox [65080 65231 2158 989] (xmin > xmax)
			29, 0, 0, 0xFE, 0x38, 29, 0, 0, 0xFE, 0xCF, 28, 0x08, 0x6E, 28, 0x03, 0xDD, 5,
		}
		top = enteroDictCFF(top, offCharset, 15)
		top = append(top, 29, byte(offFDArray>>24), byte(offFDArray>>16), byte(offFDArray>>8), byte(offFDArray), 12, 36)
		top = enteroDictCFF(top, offCS, 17)
		top = append(top, 29, 0, 0, 0, 2)
		return enteroDictCFF(top, offPriv, 18)
	}

	fdDummy := indiceDePrueba(armarFD(0))
	topDummy := indiceDePrueba(armarTop(0, 0, 0, 0, 0))
	base := len(cabecera) + len(nombres) + len(topDummy) + len(cadenas) + len(sinSubrs)
	offCharset := base
	offFDSelect := offCharset + len(charset)
	offCS := offFDSelect + len(fdSelect)
	offFDArray := offCS + len(glifos)
	offPriv := offFDArray + len(fdDummy)
	fdReal := indiceDePrueba(armarFD(offPriv))
	topReal := indiceDePrueba(armarTop(offCharset, offFDSelect, offFDArray, offCS, offPriv))

	var b bytes.Buffer
	for _, p := range [][]byte{cabecera, nombres, topReal, cadenas, sinSubrs, charset, fdSelect, glifos, fdReal, privado} {
		b.Write(p)
	}
	return b.Bytes()
}

func cffSimpleConDotsectionTest() []byte {
	vacio := []byte{139, 14}
	// Glifo con dotsection (12 0) en mitad del trazado (issue7439.pdf).
	conDot := []byte{139, 239, 239, 21, 12, 0, 248, 136, 6, 248, 136, 7, 252, 136, 6, 14}
	return armarCFFSimple("DotFont", []string{".notdef", "A"}, [][]byte{vacio, conDot}, []byte{65})
}
