// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// indiceDePrueba codifica una INDEX de CFF con desplazamientos de dos bytes.
func indiceDePrueba(elems ...[]byte) []byte {
	if len(elems) == 0 {
		return []byte{0, 0}
	}
	out := binary.BigEndian.AppendUint16(nil, uint16(len(elems))) //nolint:gosec // G115: unas decenas
	out = append(out, 2)
	off := 1
	out = binary.BigEndian.AppendUint16(out, uint16(off)) //nolint:gosec // G115: unos cientos de bytes
	for _, e := range elems {
		off += len(e)
		out = binary.BigEndian.AppendUint16(out, uint16(off)) //nolint:gosec // G115: idem
	}
	for _, e := range elems {
		out = append(out, e...)
	}
	return out
}

// cffDePrueba es un CFF mínimo con el charset ISOAdobe (glifo i = SID i) en el
// que la «A» (SID 34) es un CUADRADO macizo de 100 a 600 milésimas: así se ve si
// se pinta con SU contorno o con la letra de reserva.
func cffDePrueba() []byte {
	vacio := []byte{14} // endchar
	// 100 100 rmoveto 500 hlineto 500 vlineto -500 hlineto endchar
	cuadrado := []byte{239, 239, 21, 248, 136, 6, 248, 136, 7, 252, 136, 6, 14}
	glifos := make([][]byte, 35)
	for i := range glifos {
		glifos[i] = vacio
	}
	glifos[34] = cuadrado
	return cffConGlifos(glifos)
}

// cffConGlifos arma un CFF con esos charstrings y el charset ISOAdobe.
func cffConGlifos(glifos [][]byte) []byte {
	cabecera := []byte{1, 0, 4, 2}
	nombres := indiceDePrueba([]byte("Prueba"))
	sinCadenas, sinSubrs := indiceDePrueba(), indiceDePrueba()
	// El Top DICT solo lleva CharStrings (17) con su desplazamiento en 4 bytes
	// (operador 29), así que su tamaño no depende de dónde caiga.
	tamTop := len(indiceDePrueba(make([]byte, 6)))
	offCS := len(cabecera) + len(nombres) + tamTop + len(sinCadenas) + len(sinSubrs)
	top := append([]byte{29}, binary.BigEndian.AppendUint32(nil, uint32(offCS))...) //nolint:gosec // G115: idem
	top = append(top, 17)
	var b bytes.Buffer
	for _, parte := range [][]byte{cabecera, nombres, indiceDePrueba(top), sinCadenas, sinSubrs, indiceDePrueba(glifos...)} {
		b.Write(parte)
	}
	return b.Bytes()
}

func TestLeerCFFCharsetYCodificacion(t *testing.T) {
	info, err := leerCFF(cffDePrueba())
	if err != nil {
		t.Fatal(err)
	}
	if info.numGlifos != 35 || info.cid || info.nombres[34] != "A" {
		t.Fatalf("info = %d glifos, cid=%v, glifo 34 = %q", info.numGlifos, info.cid, info.nombres[34])
	}
	if g := info.propia['A']; g != 34 {
		t.Errorf("la codificación estándar lleva la «A» al glifo %d, quería 34", g)
	}
}

// Un CFF suelto (/FontFile3 /Type1C) se pintaba con la letra de reserva: ahora
// se envuelve en OpenType y se pinta con SU contorno, en el PNG y en el SVG.
func TestFuenteCFFSePintaConSuContorno(t *testing.T) {
	cff := cffDePrueba()
	fuente := `<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Prueba /FirstChar 65 /LastChar 65 /Widths [700] ` +
		`/Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>`
	desc := `<< /Type /FontDescriptor /FontName /ABCDEF+Prueba /Flags 32 /FontBBox [0 0 700 700] /ItalicAngle 0 ` +
		`/Ascent 700 /Descent 0 /CapHeight 700 /StemV 80 /FontFile3 7 0 R >>`
	flujo := fmt.Sprintf("<< /Subtype /Type1C /Length %d >>\nstream\n%s\nendstream", len(cff), cff)
	pdf := pdfConContenido(t, `BT /F1 100 Tf 10 20 Td (A) Tj ET`, `/Resources << /Font << /F1 5 0 R >> >>`, fuente, desc, flujo)

	img, err := png.Decode(bytes.NewReader(rasterizar(t, pdf, OpcionesRender{Escala: 1})))
	if err != nil {
		t.Fatal(err)
	}
	// La esquina de arriba a la izquierda del cuadrado (página 22, 78): negra con
	// el contorno del CFF, blanca con una «A» de reserva, que ahí no tiene nada.
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
