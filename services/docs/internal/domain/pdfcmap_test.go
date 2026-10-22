// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
)

// compuesta devuelve los objetos 5, 6 y 7 (y 8 si hay /ToUnicode) de una Type0
// con esa /Encoding (un nombre, o el texto de una CMap que va en el objeto 7).
func compuesta(encoding, cmap, toUnicode string) []string {
	objs := []string{"", `<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Prueba /CIDToGIDMap /Identity ` +
		`/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 500 >>`}
	enc := encoding
	if cmap != "" {
		enc = "7 0 R"
		objs = append(objs, fmt.Sprintf("<< /Type /CMap /CMapName /Prueba-H /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap))
	}
	tu := ""
	switch {
	case strings.HasPrefix(toUnicode, "/"):
		tu = " /ToUnicode " + toUnicode
	case toUnicode != "":
		tu = fmt.Sprintf(" /ToUnicode %d 0 R", 5+len(objs))
		objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(toUnicode), toUnicode))
	}
	objs[0] = `<< /Type /Font /Subtype /Type0 /BaseFont /Prueba /Encoding ` + enc + ` /DescendantFonts [6 0 R]` + tu + ` >>`
	return objs
}

func textoCompuesta(t *testing.T, contenido string, objs []string) string {
	t.Helper()
	var b strings.Builder
	for _, f := range fragmentosDe(t, contenido, helvetica, objs...) {
		b.WriteString(f.Texto)
	}
	return b.String()
}

// Una CMap incrustada de UN byte (OneByteIdentityH de Word, Prince-*-H) se leía
// de dos en dos: «Hola» salía como dos códigos sin /ToUnicode, es decir, nada.
func TestCMapIncrustadaDeUnByte(t *testing.T) {
	cmap := `begincmap 1 begincodespacerange <00> <FF> endcodespacerange
1 begincidrange <00> <FF> 0 endcidrange endcmap`
	tu := `begincmap 1 begincodespacerange <00> <FF> endcodespacerange
1 beginbfrange <20> <7E> <0020> endbfrange endcmap`
	if got := textoCompuesta(t, `BT /F1 12 Tf 10 50 Td (Hola) Tj ET`, compuesta("", cmap, tu)); got != "Hola" {
		t.Fatalf("texto = %q, quería «Hola»", got)
	}
}

// Uni-Utf8-H incrustada: códigos de uno a cuatro bytes según el UTF-8, con su
// /ToUnicode indexada por esos mismos códigos.
func TestCMapIncrustadaUTF8(t *testing.T) {
	cmap := `begincmap 4 begincodespacerange <00> <7F> <C080> <DFBF> <E08080> <EFBFBF> <F0808080> <F7BFBFBF>
endcodespacerange 3 begincidchar <61> 1 <C3A9> 2 <E38081> 3 endcidchar endcmap`
	tu := `begincmap 4 begincodespacerange <00> <7F> <C080> <DFBF> <E08080> <EFBFBF> <F0808080> <F7BFBFBF>
endcodespacerange 3 beginbfchar <61> <0061> <C3A9> <00E9> <E38081> <3001> endbfchar endcmap`
	if got := textoCompuesta(t, `BT /F1 12 Tf 10 50 Td <61C3A9E3808161> Tj ET`, compuesta("", cmap, tu)); got != "aé、a" {
		t.Fatalf("texto = %q, quería «aé、a»", got)
	}
}

// Las predefinidas por Unicode no necesitan tablas: el código es el carácter.
func TestCMapPredefinidaUnicodeSinToUnicode(t *testing.T) {
	if got := textoCompuesta(t, `BT /F1 12 Tf 10 50 Td <30423044> Tj ET`, compuesta("/UniJIS-UCS2-H", "", "")); got != "あい" {
		t.Fatalf("UCS2: texto = %q, quería «あい»", got)
	}
	if got := textoCompuesta(t, `BT /F1 12 Tf 10 50 Td <61E38182> Tj ET`, compuesta("/UniJIS-UTF8-H", "", "")); got != "aあ" {
		t.Fatalf("UTF8: texto = %q, quería «aあ»", got)
	}
}

// Una /ToUnicode que dice «Identity» —por nombre o copiando la CMap de CIDs de
// Adobe— declara que el código es el carácter.
func TestToUnicodeIdentidad(t *testing.T) {
	if got := textoCompuesta(t, `BT /F1 12 Tf 10 50 Td <00480069> Tj ET`, compuesta("/Identity-H", "", "/Identity-H")); got != "Hi" {
		t.Fatalf("por nombre: texto = %q, quería «Hi»", got)
	}
	adobe := `begincmap 1 begincodespacerange <0000> <FFFF> endcodespacerange
1 begincidrange <0000> <FFFF> 0 endcidrange endcmap`
	if got := textoCompuesta(t, `BT /F1 12 Tf 10 50 Td <00480069> Tj ET`, compuesta("/Identity-H", "", adobe)); got != "Hi" {
		t.Fatalf("CMap de Adobe: texto = %q, quería «Hi»", got)
	}
}

// Sin /ToUnicode, la `cmap` de la TrueType incrustada dice qué letra es cada
// glifo; antes salía un espacio de anchura cero por letra.
func TestCompuestaSinToUnicodeUsaLaCmapDeLaTrueType(t *testing.T) {
	cara, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	var hex strings.Builder
	for _, r := range "Hola" {
		g, err := cara.GlyphIndex(&buf, r)
		if err != nil || g == 0 {
			t.Fatalf("sin glifo para %q", r)
		}
		fmt.Fprintf(&hex, "%04X", int(g))
	}
	objs := compuesta("/Identity-H", "", "")
	objs[1] = strings.Replace(objs[1], "/DW 500", "/DW 500 /FontDescriptor 7 0 R", 1)
	objs = append(objs, `<< /Type /FontDescriptor /FontName /Prueba /Flags 32 /FontFile2 8 0 R >>`,
		flujoPDF("", goregular.TTF))
	if got := textoCompuesta(t, `BT /F1 12 Tf 10 50 Td <`+hex.String()+`> Tj ET`, objs); got != "Hola" {
		t.Fatalf("texto = %q, quería «Hola»", got)
	}
}
