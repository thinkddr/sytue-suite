// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"
)

func TestParidadGraficosPDF(t *testing.T) {
	t.Run("recorte_texto_tr_4_a_7", func(t *testing.T) {
		base := append([]byte(nil), goregular.TTF...)
		fuente := `<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+GoReg /FirstChar 32 /LastChar 122 /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>`
		desc := `<< /Type /FontDescriptor /FontName /ABCDEF+GoReg /Flags 32 /FontBBox [0 -200 1000 900] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>`
		flujo := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(base), base)
		// Tr 7 recorta por el contorno del texto sin pintarlo; el rectángulo rojo de
		// página completa solo debe aparecer dentro de las letras (issue3584.pdf).
		contenido := "q BT /F1 72 Tf 7 Tr 10 20 Td (HI) Tj ET 1 0 0 rg 0 0 200 100 re f Q"
		pdf := pdfConContenido(t, contenido, `/MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >>`, fuente, desc, flujo)

		svg := renderizar(t, pdf)
		if !strings.Contains(svg, "<clipPath") || !strings.Contains(svg, `clip-path="url(#`) {
			t.Fatalf("Tr 7 en SVG no abrió un <clipPath> con el contorno del texto: %s", primerasLineas(svg))
		}

		img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		// Esquina (190, 5) fuera del texto debe quedar blanca, no roja.
		esquina := color.NRGBAModel.Convert(img.At(190, 5)).(color.NRGBA)
		if esquina.R > 240 && esquina.G < 30 && esquina.B < 30 {
			t.Fatalf("Tr 7 en PNG no recortó el rectángulo exterior: esquina=%+v", esquina)
		}
		if n := pixelesRojos(img); n < 80 || n > 6000 {
			t.Fatalf("Tr 7 en PNG debía pintar rojo solo dentro de «HI», pintó %d píxeles rojos", n)
		}
	})

	t.Run("mosaico_coloreado_no_coloreado_solapado_y_texto", func(t *testing.T) {
		// 1) Mosaico solapado (tiling-pattern-fold-order.pdf): BBox 20x20, paso 10x10.
		celdaSol := "1 0 0 rg 0 0 10 10 re f 0 0 1 rg 10 10 10 10 re f"
		patSol := fmt.Sprintf("<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 20 20] /XStep 10 /YStep 10 /Length %d >>\nstream\n%s\nendstream", len(celdaSol), celdaSol)
		// 2) Mosaico no coloreado PaintType 2 en [/Pattern /DeviceRGB] con color interior que debe ignorarse.
		celdaNoCol := "1 0 0 rg 0 0 10 10 re f"
		patNoCol := fmt.Sprintf("<< /Type /Pattern /PatternType 1 /PaintType 2 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Length %d >>\nstream\n%s\nendstream", len(celdaNoCol), celdaNoCol)

		base := append([]byte(nil), goregular.TTF...)
		fuente := `<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+GoReg /FirstChar 32 /LastChar 122 /Encoding /WinAnsiEncoding /FontDescriptor 8 0 R >>`
		desc := `<< /Type /FontDescriptor /FontName /ABCDEF+GoReg /Flags 32 /FontBBox [0 -200 1000 900] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile2 9 0 R >>`
		flujo := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(base), base)

		contenido := "q /Pattern cs /P1 scn 0 0 20 20 re f Q " +
			"q /CSp cs 0 0.8 0 /P2 scn 30 0 20 20 re f Q " +
			"q /Pattern cs BT /F1 60 Tf /P1 scn 60 10 Td (H) Tj ET Q"
		recursos := `/MediaBox [0 0 200 100] /Resources << /ColorSpace << /CSp [/Pattern /DeviceRGB] >> /Pattern << /P1 5 0 R /P2 6 0 R >> /Font << /F1 7 0 R >> >>`
		pdf := pdfConContenido(t, contenido, recursos, patSol, patNoCol, fuente, desc, flujo)

		svg := renderizar(t, pdf)
		if !strings.Contains(svg, "#00cc00") {
			t.Fatalf("PaintType 2 en SVG no aplicó el color base verde #00cc00: %s", primerasLineas(svg))
		}
		if strings.Count(svg, `fill="url(#`) < 3 {
			t.Fatalf("el texto con relleno de patrón en SVG no usó url(#...): %s", primerasLineas(svg))
		}

		img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		// Los 4 cuadrantes de [0,20]x[0,20] (en Y SVG: 80..100) deben quedar rojos por orden de solape.
		for _, pt := range [][2]int{{5, 95}, {15, 95}, {5, 85}, {15, 85}} {
			c := color.NRGBAModel.Convert(img.At(pt[0], pt[1])).(color.NRGBA)
			if c.R < 220 || c.G > 30 || c.B > 30 {
				t.Fatalf("mosaico solapado en PNG (%d,%d) debía ser rojo, salió %+v", pt[0], pt[1], c)
			}
		}
		// El rectángulo PaintType 2 en x=30..50, y=80..100 debe ser verde, no blanco ni rojo.
		cv := color.NRGBAModel.Convert(img.At(40, 90)).(color.NRGBA)
		if cv.G < 170 || cv.R > 40 || cv.B > 40 {
			t.Fatalf("PaintType 2 en PNG debía ser verde, salió %+v", cv)
		}
		// La letra «H» rellena con patrón en x=60..110 debe tener píxeles coloreados en PNG.
		coloreadosTexto := 0
		for y := 40; y < 95; y++ {
			for x := 60; x < 120; x++ {
				c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				if c.R < 250 || c.G < 250 || c.B < 250 {
					coloreadosTexto++
				}
			}
		}
		if coloreadosTexto < 50 {
			t.Fatalf("texto con patrón PatternType 1 en PNG quedó en blanco (%d píxeles)", coloreadosTexto)
		}
	})

	t.Run("mosaico_denso_rapido", func(t *testing.T) {
		celda := "0 0 1 rg 0 0 1 1 re f"
		pat := fmt.Sprintf("<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 2 2] /XStep 2 /YStep 2 /Length %d >>\nstream\n%s\nendstream", len(celda), celda)
		pdf := pdfConContenido(t, "/Pattern cs /P1 scn 0 0 200 100 re f", `/Resources << /Pattern << /P1 5 0 R >> >>`, pat)
		t0 := time.Now()
		img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		if d := time.Since(t0); d > 500*time.Millisecond {
			t.Fatalf("mosaico denso tardó demasiado: %v", d)
		}
		if pixelesOscuros(img) < 1000 {
			t.Fatalf("mosaico denso no pintó celdas en PNG (%d píxeles)", pixelesOscuros(img))
		}
	})

	t.Run("par_impar_relleno_y_recorte", func(t *testing.T) {
		// Dos cuadrados concéntricos con el MISMO sentido de giro: con f* y W* el
		// hueco interior debe quedar blanco, mientras que con regla no-cero se tapa.
		anillo := "10 10 80 80 re 30 30 40 40 re"
		contenido := "q 0 0 1 rg " + anillo + " f* Q " +
			"q 110 10 80 80 re 130 30 40 40 re W* n 1 0 0 rg 100 0 100 100 re f Q"
		pdf := pdfConContenido(t, contenido, `/MediaBox [0 0 200 100]`)

		img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		// Hueco de f* en (50, 50) debe ser blanco; borde en (20, 50) azul.
		huecoF := color.NRGBAModel.Convert(img.At(50, 50)).(color.NRGBA)
		bordeF := color.NRGBAModel.Convert(img.At(20, 50)).(color.NRGBA)
		if huecoF.B > 240 && huecoF.R < 30 {
			t.Fatalf("f* rellenó el hueco concéntrico como non-zero: %+v", huecoF)
		}
		if huecoF.R < 240 || huecoF.G < 240 || huecoF.B < 240 || bordeF.B < 220 || bordeF.R > 30 {
			t.Fatalf("f* inesperado: hueco=%+v borde=%+v", huecoF, bordeF)
		}
		// Hueco de W* en (150, 50) debe ser blanco; borde en (120, 50) rojo.
		huecoW := color.NRGBAModel.Convert(img.At(150, 50)).(color.NRGBA)
		bordeW := color.NRGBAModel.Convert(img.At(120, 50)).(color.NRGBA)
		if huecoW.R < 240 || huecoW.G < 240 || huecoW.B < 240 || bordeW.R < 220 || bordeW.B > 30 {
			t.Fatalf("W* inesperado: hueco=%+v borde=%+v", huecoW, bordeW)
		}
	})

	t.Run("puntos_longitud_cero_fase_guiones_y_franjas_finas", func(t *testing.T) {
		// 1) Línea punteada [0 12] 0 d con 1 J (tapa redonda).
		// 2) Línea discontinua [10 10] 10 d: empieza en hueco [10..20] y traza [20..30].
		// 3) Franja fina rellena -0.12 re f* (euskadi-gauzatu2.pdf).
		contenido := "q 0 0 0 RG 6 w 1 J [0 12] 0 d 10 80 m 70 80 l S Q " +
			"q 1 0 0 RG 6 w 0 J [10 10] 10 d 10 50 m 50 50 l S Q " +
			"q 0 0.5 1 rg 10 20 80 -0.12 re f* Q"
		pdf := pdfConContenido(t, contenido, "")

		svg := renderizar(t, pdf)
		if !strings.Contains(svg, `stroke-dashoffset="10"`) {
			t.Fatalf("SVG ignoró la fase del patrón de guiones: %s", primerasLineas(svg))
		}

		img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
		if err != nil {
			t.Fatal(err)
		}
		// Punto redondo en (10, 20) (y=80 en PDF -> y=20 en imagen de alto 100).
		if c := color.NRGBAModel.Convert(img.At(10, 20)).(color.NRGBA); c.R > 60 {
			t.Fatalf("guion de longitud 0 con 1 J no pintó punto redondo en PNG: %+v", c)
		}
		// Con fase 10 en [10 10], x=15 (5 pt desde x=10) cae en hueco blanco y x=25 (15 pt) en trazo rojo.
		cHueco := color.NRGBAModel.Convert(img.At(15, 50)).(color.NRGBA)
		cTrazo := color.NRGBAModel.Convert(img.At(25, 50)).(color.NRGBA)
		if cHueco.G < 220 || cTrazo.R < 220 || cTrazo.G > 40 {
			t.Fatalf("fase de guiones en PNG mal aplicada: hueco(15,50)=%+v trazo(25,50)=%+v", cHueco, cTrazo)
		}
		// Franja fina en x=50, y=80 (y=20 en PDF) debe quedar azul visible, no casi blanca.
		cFranja := color.NRGBAModel.Convert(img.At(50, 80)).(color.NRGBA)
		if cFranja.R > 80 || cFranja.B < 200 {
			t.Fatalf("franja fina rellena -0.12 re f* quedó desvanecida en PNG: %+v", cFranja)
		}
	})
}

func pixelesRojos(img image.Image) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.R > 200 && c.G < 60 && c.B < 60 {
				n++
			}
		}
	}
	return n
}
