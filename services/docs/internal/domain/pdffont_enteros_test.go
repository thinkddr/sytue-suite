// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"math"
	"testing"
)

func TestAnchosSimplesNoTruncanPrimerCodigo(t *testing.T) {
	l := &lienzo{d: &PDFDoc{}}
	for _, primero := range []int64{-1, 256, 1<<32 + 65, math.MaxInt64} {
		f := &fuentePDF{anchos: map[uint32]float64{65: 900}}
		l.anchosSimples(pdfDict{"FirstChar": primero, "Widths": pdfArray{int64(123)}}, f)
		if f.anchos[65] != 900 {
			t.Errorf("FirstChar %d overwrote the width of a different glyph", primero)
		}
		for codigo := range f.anchos {
			if codigo > 255 {
				t.Errorf("FirstChar %d produced an invalid simple-font code %d", primero, codigo)
			}
		}
	}
}

func TestAnchosSimplesTerminanEnUltimoCodigo(t *testing.T) {
	d := &PDFDoc{}
	o, err := d.parsearObjeto(&pdfLexer{buf: []byte("<< /FirstChar 255 /Widths [123 456] >>")})
	if err != nil {
		t.Fatal(err)
	}
	f := &fuentePDF{anchos: map[uint32]float64{}}
	(&lienzo{d: d}).anchosSimples(o.(pdfDict), f)
	if len(f.anchos) != 1 || f.anchos[255] != 123 {
		t.Fatalf("widths at the simple-font boundary = %v; want only code 255 with width 123", f.anchos)
	}
}

func TestMetricasCIDNoTruncanCodigos(t *testing.T) {
	l := &lienzo{d: &PDFDoc{}}
	for _, codigo := range []int64{-1, maxCID + 1, 1<<32 + 65, math.MaxInt64} {
		f := &fuentePDF{anchos: map[uint32]float64{65: 900}}
		l.anchosCID(pdfDict{"W": pdfArray{codigo, pdfArray{int64(123)}}}, f)
		if len(f.anchos) != 1 || f.anchos[65] != 900 {
			t.Errorf("invalid CID %d changed horizontal metrics: %v", codigo, f.anchos)
		}
		l.altosCID(pdfDict{"W2": pdfArray{codigo, pdfArray{int64(-123), int64(40), int64(50)}}}, f)
		if len(f.altos) != 0 || len(f.vx) != 0 || len(f.vy) != 0 {
			t.Errorf("invalid CID %d created vertical metrics", codigo)
		}
	}
	f := &fuentePDF{anchos: map[uint32]float64{}}
	l.anchosCID(pdfDict{"W": pdfArray{int64(maxCID), pdfArray{int64(123), int64(456)}}}, f)
	if len(f.anchos) != 1 || f.anchos[maxCID] != 123 {
		t.Fatalf("widths at the CID boundary = %v; want only CID %d", f.anchos, maxCID)
	}
}

func TestDecodificarSeparaOperandosNumericosDeCadenas(t *testing.T) {
	d := &PDFDoc{}
	o, err := d.parsearObjeto(&pdfLexer{buf: []byte(fmt.Sprintf("[%d <00007FFF8000FFFFFF>]", math.MaxInt))})
	if err != nil {
		t.Fatal(err)
	}
	ops := o.(pdfArray)
	if n, ok := ops[0].(int64); !ok || n != int64(math.MaxInt) {
		t.Fatalf("the numeric operand was not preserved as an integer: %v", ops[0])
	}
	f := &fuentePDF{
		dosBytes: true,
		anchos:   map[uint32]float64{0: 100, 0x7FFF: 200, 0x8000: 300, 0xFFFF: 400, 0xFF: 500},
		aUnicode: map[uint32]string{0: "A", 0x7FFF: "B", 0x8000: "C", 0xFFFF: "D", 0xFF: "E"},
	}
	glifos := f.decodificar(ultimaCadena(ops))
	if len(glifos) != 5 {
		t.Fatalf("decoded %d glyphs; want 5 including the trailing single byte", len(glifos))
	}
	for i, codigo := range []uint32{0, 0x7FFF, 0x8000, 0xFFFF, 0xFF} {
		if g := glifos[i]; g.codigo != codigo || g.ancho != f.anchos[codigo] || g.texto != f.aUnicode[codigo] {
			t.Errorf("glyph %d = %+v; want code %#x with its own text and width", i, g, codigo)
		}
	}
	if s := ultimaCadena(append(ops, ops[0])); s != nil {
		t.Fatalf("a numeric final operand was treated as a PDF string: %x", s)
	}
}
