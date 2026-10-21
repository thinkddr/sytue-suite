// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"slices"
	"testing"
)

// cffConAcentuado es un CFF con la «A» cuadrada de cffDePrueba, un «acute» que
// es una raya de 50 a 150 milésimas a 600 de altura, y el «Aacute» escrito como
// en Type 2: endchar con adx ady base acento (TN 5177, apéndice C).
func cffConAcentuado(t *testing.T) []byte {
	t.Helper()
	sid := func(n string) int {
		i := slices.Index(cadenasEstandarCFF, n)
		if i < 0 {
			t.Fatalf("sin SID para %s", n)
		}
		return i
	}
	glifos := make([][]byte, sid("Aacute")+1)
	for i := range glifos {
		glifos[i] = []byte{14}
	}
	glifos[sid("A")] = []byte{239, 239, 21, 248, 136, 6, 248, 136, 7, 252, 136, 6, 14}
	// 50 600 rmoveto 100 hlineto 50 vlineto -100 hlineto endchar
	glifos[sid("acute")] = []byte{189, 248, 236, 21, 239, 6, 189, 7, 39, 6, 14}
	// 200 0 65 194 endchar: la base es el código 65 (A) y el acento el 194 (acute)
	glifos[sid("Aacute")] = []byte{247, 92, 139, 204, 247, 86, 14}
	return cffConGlifos(glifos)
}

func TestAplanarSeacCFFComponeElAcentuado(t *testing.T) {
	cff := aplanarSeacCFF(cffConAcentuado(t))
	info, err := leerCFF(cff)
	if err != nil {
		t.Fatal(err)
	}
	prog, err := leerProgramasCFF(cff)
	if err != nil {
		t.Fatal(err)
	}
	g := slices.Index(info.nombres, "Aacute")
	if g < 0 {
		t.Fatalf("sin Aacute: %q", info.nombres)
	}
	in := &interpreteType2{prog: prog}
	if err := in.correr(prog.glifos[g], 0); err != nil {
		t.Fatal(err)
	}
	if in.hayAcento {
		t.Error("el Aacute sigue siendo un endchar compuesto")
	}
	var movs []puntoT1
	for _, s := range in.segs {
		if s.op == 'm' {
			movs = append(movs, s.p[0])
		}
	}
	if fmt.Sprint(movs) != fmt.Sprint([]puntoT1{{100, 100}, {250, 600}}) {
		t.Errorf("subtrazados en %v, quería la base en (100,100) y el acento en (250,600)", movs)
	}
	// Sin acentuados, el CFF no se toca.
	if sin := cffDePrueba(); !bytes.Equal(aplanarSeacCFF(sin), sin) {
		t.Error("se reescribió un CFF que no tenía acentuados")
	}
}

// El «Á» de un CFF que lo compone con endchar salía VACÍO: sfnt no hace el seac.
func TestCFFConAcentuadoPintaElAcento(t *testing.T) {
	cff := cffConAcentuado(t)
	fuente := `<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Prueba /FirstChar 193 /LastChar 193 /Widths [700] ` +
		`/Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>`
	desc := `<< /Type /FontDescriptor /FontName /ABCDEF+Prueba /Flags 32 /FontBBox [0 0 700 700] /ItalicAngle 0 ` +
		`/Ascent 700 /Descent 0 /CapHeight 700 /StemV 80 /FontFile3 7 0 R >>`
	flujo := fmt.Sprintf("<< /Subtype /Type1C /Length %d >>\nstream\n%s\nendstream", len(cff), cff)
	pdf := pdfConContenido(t, `BT /F1 100 Tf 10 20 Td <C1> Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuente, desc, flujo)
	img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
	if err != nil {
		t.Fatal(err)
	}
	// Base: el cuadrado de (20, 30) a (70, 80). Acento: de (35, 80) a (45, 85).
	for _, p := range [][2]int{{22, 78}, {40, 82}} {
		if c := color.GrayModel.Convert(img.At(p[0], 100-p[1])).(color.Gray); c.Y > 80 {
			t.Errorf("el punto %v del «Á» está en blanco (gris %d)", p, c.Y)
		}
	}
}

// Los operadores 29 a 31 (callgsubr, vhcurveto, hvcurveto) se tomaban por
// números cortados y el CFF entero se quedaba sin aplanar (issue4801 de pdf.js).
func TestInterpreteType2Curvas(t *testing.T) {
	// 0 0 rmoveto 10 20 30 40 hvcurveto endchar
	in := &interpreteType2{prog: &programasCFF{}}
	if err := in.correr([]byte{139, 139, 21, 149, 159, 169, 179, 31, 14}, 0); err != nil {
		t.Fatal(err)
	}
	quiero := []segmentoT1{
		{op: 'm', p: [3]puntoT1{{0, 0}}},
		{op: 'c', p: [3]puntoT1{{10, 0}, {30, 30}, {30, 70}}},
	}
	if fmt.Sprint(in.segs) != fmt.Sprint(quiero) {
		t.Errorf("hvcurveto:\n got %v\nwant %v", in.segs, quiero)
	}
}
