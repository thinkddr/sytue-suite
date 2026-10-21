// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/color"
	"image/png"
	"strconv"
	"strings"
	"testing"
)

// csType1 ensambla un charstring Type 1 sin cifrar: números y nombres de
// operador separados por blancos.
func csType1(t *testing.T, fuente string) []byte {
	t.Helper()
	ops := map[string][]byte{
		"hstem": {1}, "vstem": {3}, "vmoveto": {4}, "rlineto": {5}, "hlineto": {6}, "vlineto": {7},
		"rrcurveto": {8}, "closepath": {9}, "callsubr": {10}, "return": {11}, "hsbw": {13},
		"endchar": {14}, "rmoveto": {21}, "hmoveto": {22}, "vhcurveto": {30}, "hvcurveto": {31},
		"seac": {12, 6}, "sbw": {12, 7}, "div": {12, 12}, "callothersubr": {12, 16}, "pop": {12, 17},
		"setcurrentpoint": {12, 33},
	}
	var out []byte
	for _, tok := range strings.Fields(fuente) {
		if op, ok := ops[tok]; ok {
			out = append(out, op...)
			continue
		}
		v, err := strconv.Atoi(tok)
		if err != nil {
			t.Fatalf("símbolo %q", tok)
		}
		switch {
		case v >= -107 && v <= 107:
			out = append(out, byte(v+139))
		case v >= 108 && v <= 1131:
			out = append(out, byte((v-108)/256+247), byte((v-108)%256))
		case v >= -1131 && v <= -108:
			out = append(out, byte((-v-108)/256+251), byte((-v-108)%256))
		default:
			out = binary.BigEndian.AppendUint32(append(out, 255), uint32(int32(v))) //nolint:gosec // G115: prueba
		}
	}
	return out
}

// cifrarType1 es la inversa de descifrarType1, con n bytes de relleno delante.
func cifrarType1(b []byte, r uint16, n int) []byte {
	claro := append(make([]byte, n), b...)
	out := make([]byte, len(claro))
	for i, p := range claro {
		c := p ^ byte(r>>8)
		out[i] = c
		r = (uint16(c)+r)*52845 + 22719
	}
	return out
}

// type1DePrueba es una Type 1 en la que la «A» es el mismo CUADRADO macizo de
// 100 a 600 milésimas que en cffDePrueba, con su último lado en una subrutina.
func type1DePrueba(t *testing.T) (programa []byte, len1 int) {
	t.Helper()
	claro := "%!PS-AdobeFont-1.0: Prueba 001\n11 dict begin\n/FontName /Prueba def\n" +
		"/FontMatrix [0.001 0 0 0.001 0 0] readonly def\n" +
		"/Encoding 256 array 0 1 255 {1 index exch /.notdef put} for\ndup 65 /A put\nreadonly def\n" +
		"currentdict end\ncurrentfile eexec\n"
	var priv bytes.Buffer
	priv.WriteString("dup /Private 8 dict dup begin\n/RD{string currentfile exch readstring pop}executeonly def\n" +
		"/ND{noaccess def}executeonly def\n/NP{noaccess put}executeonly def\n/lenIV 4 def\n/Subrs 1 array\n")
	binario := func(cs []byte) {
		c := cifrarType1(cs, 4330, 4)
		fmt.Fprintf(&priv, "%d RD ", len(c))
		priv.Write(c)
	}
	priv.WriteString("dup 0 ")
	binario(csType1(t, "-500 hlineto closepath return"))
	priv.WriteString(" NP\nND\n2 index /CharStrings 2 dict dup begin\n/.notdef ")
	binario(csType1(t, "0 0 hsbw endchar"))
	priv.WriteString(" ND\n/A ")
	binario(csType1(t, "0 700 hsbw 100 100 rmoveto 500 hlineto 500 vlineto 0 callsubr endchar"))
	priv.WriteString(" ND\nend\nend\nreadonly put\nnoaccess put\ndup /FontName get exch definefont pop\nmark currentfile closefile\n")
	programa = append([]byte(claro), cifrarType1(priv.Bytes(), 55665, 4)...)
	return append(programa, []byte("\n"+strings.Repeat("0", 512)+"\ncleartomark\n")...), len(claro)
}

func TestType1ACFFLeeGlifosYCodificacion(t *testing.T) {
	prog, len1 := type1DePrueba(t)
	cff, err := type1ACFF(prog, len1)
	if err != nil {
		t.Fatal(err)
	}
	info, err := leerCFF(cff)
	if err != nil {
		t.Fatal(err)
	}
	if info.numGlifos != 2 || info.nombres[0] != ".notdef" || info.nombres[1] != "A" {
		t.Fatalf("glifos = %d %q", info.numGlifos, info.nombres)
	}
	if g := info.propia[65]; g != 1 {
		t.Errorf("la codificación propia lleva el 65 al glifo %d, quería 1", g)
	}
	// Truncado por cualquier sitio no revienta: o error o un CFF que se lee.
	for n := 0; n < len(prog); n += 7 {
		if c, err := type1ACFF(prog[:n], len1); err == nil {
			if _, err := leerCFF(c); err != nil {
				t.Fatalf("truncado a %d: CFF ilegible: %v", n, err)
			}
		}
	}
}

func TestInterpreteType1FlexYSeac(t *testing.T) {
	flex := csType1(t, "0 500 hsbw 0 0 rmoveto 0 1 callothersubr "+
		"50 0 rmoveto 0 2 callothersubr -40 10 rmoveto 0 2 callothersubr 30 10 rmoveto 0 2 callothersubr "+
		"10 0 rmoveto 0 2 callothersubr 10 0 rmoveto 0 2 callothersubr 30 -10 rmoveto 0 2 callothersubr "+
		"10 -10 rmoveto 0 2 callothersubr 50 100 0 3 0 callothersubr pop pop setcurrentpoint "+
		"10 20 div 0 rlineto closepath endchar")
	in := &interpreteType1{}
	if err := in.ejecutar(flex); err != nil {
		t.Fatal(err)
	}
	quiero := []segmentoT1{
		{op: 'm', p: [3]puntoT1{{0, 0}}},
		{op: 'c', p: [3]puntoT1{{10, 10}, {40, 20}, {50, 20}}},
		{op: 'c', p: [3]puntoT1{{60, 20}, {90, 10}, {100, 0}}},
		{op: 'l', p: [3]puntoT1{{100.5, 0}}},
	}
	if fmt.Sprint(in.segs) != fmt.Sprint(quiero) {
		t.Errorf("flex:\n got %v\nwant %v", in.segs, quiero)
	}

	glifos := map[string][]byte{
		"A":     csType1(t, "0 700 hsbw 100 100 rmoveto 500 hlineto endchar"),
		"acute": csType1(t, "93 333 hsbw 0 500 rmoveto 100 hlineto endchar"),
	}
	in = &interpreteType1{glifos: glifos}
	if err := in.ejecutar(csType1(t, "0 700 hsbw 93 222 194 65 194 seac")); err != nil {
		t.Fatal(err)
	}
	quiero = []segmentoT1{
		{op: 'm', p: [3]puntoT1{{100, 100}}},
		{op: 'l', p: [3]puntoT1{{600, 100}}},
		{op: 'm', p: [3]puntoT1{{222, 694}}},
		{op: 'l', p: [3]puntoT1{{322, 694}}},
	}
	if fmt.Sprint(in.segs) != fmt.Sprint(quiero) || in.ancho != 700 {
		t.Errorf("seac: ancho %v\n got %v\nwant %v", in.ancho, in.segs, quiero)
	}
}

// Una Type 1 incrustada (/FontFile) se pintaba con la letra de reserva: ahora se
// convierte a CFF y se pinta con SU contorno, en el PNG y en el SVG.
func TestFuenteType1SePintaConSuContorno(t *testing.T) {
	prog, len1 := type1DePrueba(t)
	fuente := `<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Prueba /FirstChar 65 /LastChar 65 /Widths [700] ` +
		`/Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>`
	desc := `<< /Type /FontDescriptor /FontName /ABCDEF+Prueba /Flags 32 /FontBBox [0 0 700 700] /ItalicAngle 0 ` +
		`/Ascent 700 /Descent 0 /CapHeight 700 /StemV 80 /FontFile 7 0 R >>`
	flujo := fmt.Sprintf("<< /Length %d /Length1 %d /Length2 %d /Length3 0 >>\nstream\n%s\nendstream",
		len(prog), len1, len(prog)-len1, prog)
	pdf := pdfConContenido(t, `BT /F1 100 Tf 10 20 Td (A) Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuente, desc, flujo)

	img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
	if err != nil {
		t.Fatal(err)
	}
	if c := color.GrayModel.Convert(img.At(22, 100-78)).(color.Gray); c.Y > 80 {
		t.Errorf("la esquina del cuadrado no está pintada (gris %d): se usó la letra de reserva", c.Y)
	}
	d, err := LeerPDF(pdf)
	if err != nil {
		t.Fatal(err)
	}
	svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(svg, "font/otf") {
		t.Error("el SVG no incrusta la fuente: sigue sustituyéndola")
	}
}
