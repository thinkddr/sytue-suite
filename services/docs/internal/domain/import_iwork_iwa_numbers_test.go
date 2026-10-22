// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// celdaV5 arma una celda del formato actual: versión, tipo, banderas en el byte 8 y los
// valores desde el 12.
func celdaV5(tipo byte, flags uint32, valores ...[]byte) []byte {
	c := []byte{5, tipo, 0, 0, 0, 0, 0, 0}
	c = binary.LittleEndian.AppendUint32(c, flags)
	for _, v := range valores {
		c = append(c, v...)
	}
	return c
}

func u32le(v uint32) []byte  { return binary.LittleEndian.AppendUint32(nil, v) }
func f64le(v float64) []byte { return binary.LittleEndian.AppendUint64(nil, math.Float64bits(v)) }

// dec128 es mantisa·10^exp en decimal128 little-endian.
func dec128(mantisa uint64, exp int) []byte {
	b := binary.LittleEndian.AppendUint64(nil, mantisa)
	b = append(b, make([]byte, 8)...)
	e := exp + 6176
	b[15] = byte(e >> 7 & 0x7F)
	b[14] = byte(e&0x7F) << 1
	return b
}

// filaDeTesela arma una fila de tesela: las celdas seguidas y sus posiciones (0xFFFF,
// vacía). `nuevo` usa los campos del formato actual (6 y 7); si no, los anteriores (3 y 4).
func filaDeTesela(fila uint64, nuevo bool, celdas ...[]byte) []byte {
	var buf, pos []byte
	for _, c := range celdas {
		if c == nil {
			pos = binary.LittleEndian.AppendUint16(pos, 0xFFFF)
			continue
		}
		pos = binary.LittleEndian.AppendUint16(pos, uint16(len(buf)))
		buf = append(buf, c...)
	}
	if nuevo {
		return pbB(5, pbV(1, fila), pbB(6, buf), pbB(7, pos))
	}
	return pbB(5, pbV(1, fila), pbB(3, buf), pbB(4, pos))
}

// tablaIWAPrueba son los objetos de una tabla de 2×3 (modelo 600) con una celda de cada
// tipo; la segunda fila, en el formato anterior.
func tablaIWAPrueba() []objPrueba {
	viejo := append([]byte{4, 2, 0, 0}, u32le(0x20)...)
	viejo = append(append(viejo, 0, 0, 0, 0), f64le(3.25)...)
	return []objPrueba{
		{600, tipoIWAModelo, append(append(append(pbV(6, 2), pbV(7, 3)...), pbS(8, "Tabla 1")...),
			pbB(4, pbB(3, pbB(1, pbV(1, 0), pbR(2, 601))), pbR(4, 602), pbR(6, 603), pbR(17, 604))...)},
		{601, tipoIWATesela, append(
			filaDeTesela(0, true,
				celdaV5(3, 0x8, u32le(1)),
				celdaV5(2, 0x1, dec128(125, -1)),
				celdaV5(5, 0x4, f64le(276048000)),
			),
			filaDeTesela(1, false,
				viejo,
				celdaV5(9, 0x10, u32le(7)),
				celdaV5(6, 0x2, f64le(1)),
			)...)},
		{602, tipoIWALista, pbB(3, pbV(1, 1), pbS(3, "Hola & adiós"))},
		{603, tipoIWALista, pbB(3, pbV(1, 1))},
		{604, tipoIWALista, pbB(3, pbV(1, 7), pbR(9, 605))},
		{605, tipoIWARico, pbR(1, 606)},
		{606, tipoIWAAlmacen, pbS(3, "Rico")},
	}
}

func TestNumbersIWAConvierteLasCeldas(t *testing.T) {
	objs := append([]objPrueba{
		{1, tipoIWADocumento, pbR(1, 2)},
		{2, tipoIWAHoja, append(append(pbS(1, "Cuentas"), pbR(2, 3)...), pbR(2, 4)...)},
		{3, tipoIWATabla, pbR(2, 600)},
		{4, 5000, nil}, // un gráfico: no se convierte
	}, tablaIWAPrueba()...)
	res, err := ImportLocalized("cuentas.numbers", pagesIWA(t, iwaDe(64, objs...)), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindSpreadsheet {
		t.Fatalf("kind %q:\n%.300s", res.Kind, res.Content)
	}
	var m sheetModel
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Sheets) != 2 || m.Sheets[0].Name != "Aviso" || m.Sheets[1].Name != "Cuentas" {
		t.Fatalf("hojas: %+v", m.Sheets)
	}
	if a := m.Sheets[0].Cells; !strings.Contains(a["A2"], "fórmulas") || !strings.Contains(a["A3"], "gráficos") {
		t.Errorf("aviso: %v", a)
	}
	quiero := map[string]string{
		"A1": "Hola &amp; adiós", "B1": "12.5", "C1": "40087",
		"A2": "3.25", "B2": "Rico", "C2": "TRUE",
	}
	celdas := m.Sheets[1].Cells
	for ref, v := range quiero {
		if celdas[ref] != v {
			t.Errorf("%s = %q, quiero %q", ref, celdas[ref], v)
		}
	}
	if len(celdas) != len(quiero) || m.Sheets[1].Formats["C1"].ID != "date" {
		t.Errorf("celdas %v, formatos %v", celdas, m.Sheets[1].Formats)
	}
}

// Una celda que dice traer más de lo que mide, o una posición fuera del búfer, no rompe
// la lectura: esa celda no sale y las demás sí.
func TestNumbersIWACeldasRotas(t *testing.T) {
	objs := []objPrueba{
		{1, tipoIWADocumento, pbR(1, 2)},
		{2, tipoIWAHoja, append(pbS(1, "H"), pbR(2, 3)...)},
		{3, tipoIWATabla, pbR(2, 600)},
		{600, tipoIWAModelo, append(append(pbV(6, 1), pbV(7, 3)...), pbB(4, pbB(3, pbB(1, pbV(1, 0), pbR(2, 601))), pbR(4, 602))...)},
		{601, tipoIWATesela, pbB(5, pbV(1, 0),
			pbB(6, append(celdaV5(3, 0x8, u32le(1)), celdaV5(2, 0x1, []byte{1, 2})...)),
			pbB(7, []byte{0, 0, 16, 0, 0xF0, 0x0F}))},
		{602, tipoIWALista, pbB(3, pbV(1, 1), pbS(3, "bien"))},
	}
	res, err := Import("rota.numbers", pagesIWA(t, iwaDe(1<<16, objs...)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, `"A1":"bien"`) || strings.Contains(res.Content, `"B1"`) || strings.Contains(res.Content, `"C1"`) {
		t.Errorf("celdas rotas:\n%s", res.Content)
	}
}

// formaIWA arma la forma de un objeto con su geometría (en puntos): forma {1 dibujo {1
// geometría {1 posición, 2 tamaño}}}.
func formaIWA(x, y, w, h float32) []byte {
	return pbB(1, pbB(1, pbB(1, pbB(1, pbF(1, x), pbF(2, y)), pbB(2, pbF(1, w), pbF(2, h)))))
}

func TestKeynoteIWAConvierteLasDiapositivas(t *testing.T) {
	objs := append([]objPrueba{
		{1, tipoIWADocumento, pbR(2, 2)},
		{2, tipoIWAPresentacion, append(pbB(3, pbR(2, 3), pbR(2, 4)), pbB(4, pbF(1, 1024), pbF(2, 768))...)},
		{3, tipoIWANodo, pbR(2, 10)},
		{4, tipoIWANodo, pbR(2, 11)},
		{10, tipoIWADiapositiva, append(append(append(append(append(append(
			pbR(5, 20), pbR(6, 21)...), pbR(7, 20)...), pbR(7, 21)...), pbR(7, 22)...), pbR(7, 23)...), pbR(27, 24)...)},
		// La segunda tiene marcador de cuerpo, pero oculto: no está entre sus objetos.
		{11, tipoIWADiapositiva, pbR(6, 25)},
		{20, tipoIWAMarcador, pbB(1, formaIWA(0, 0, 1024, 76.8), pbR(2, 30))},
		{21, tipoIWAMarcador, pbB(1, formaIWA(256, 384, 512, 384), pbR(2, 31))},
		{22, tipoIWATabla, append(pbB(1, pbB(1, pbB(1, pbF(1, 0), pbF(2, 0)), pbB(2, pbF(1, 512), pbF(2, 76.8)))), pbR(2, 600)...)},
		{23, 3005, nil}, // una imagen: no se convierte
		{24, tipoIWANota, pbR(1, 32)},
		{25, tipoIWAMarcador, pbB(1, formaIWA(0, 0, 10, 10), pbR(2, 33))},
		{30, tipoIWAAlmacen, pbS(3, "Título")},
		{31, tipoIWAAlmacen, pbS(3, "Uno\nDos")},
		{32, tipoIWAAlmacen, pbS(3, "Nota del orador")},
		{33, tipoIWAAlmacen, pbS(3, "OCULTO")},
	}, tablaIWAPrueba()...)
	res, err := ImportLocalized("charla.key", pagesIWA(t, iwaDe(1<<16, objs...)), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindPresentation {
		t.Fatalf("kind %q:\n%.300s", res.Kind, res.Content)
	}
	var m slidesModel
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatal(err)
	}
	if m.Size == nil || m.Size.Preset != "4:3" || len(m.Slides) != 3 {
		t.Fatalf("tamaño %+v, %d diapositivas:\n%s", m.Size, len(m.Slides), res.Content)
	}
	s := m.Slides[0]
	if len(s.Boxes) != 3 {
		t.Fatalf("cajas: %+v", s.Boxes)
	}
	if b := s.Boxes[0]; b.HTML != "<p>Título</p>" || b.X != 0 || b.W != 100 || b.H != 10 {
		t.Errorf("título: %+v", b)
	}
	if b := s.Boxes[1]; b.HTML != "<ul><li>Uno</li><li>Dos</li></ul>" || b.X != 25 || b.Y != 50 || b.W != 50 || b.H != 50 {
		t.Errorf("cuerpo: %+v", b)
	}
	if b := s.Boxes[2]; !strings.Contains(b.HTML, "<td>Hola &amp; adiós</td>") || !strings.Contains(b.HTML, "<td>2009-10-01</td>") || b.W != 50 {
		t.Errorf("tabla: %+v", b)
	}
	if s.Notes != "Nota del orador" {
		t.Errorf("notas: %q", s.Notes)
	}
	if len(m.Slides[1].Boxes) != 0 || strings.Contains(res.Content, "OCULTO") {
		t.Errorf("el marcador oculto no debería salir: %+v", m.Slides[1].Boxes)
	}
	if u := m.Slides[2]; len(u.Boxes) != 2 || !strings.Contains(u.Boxes[0].HTML, "Conversión parcial · presentación .key") {
		t.Errorf("falta la diapositiva del aviso: %+v", u.Boxes)
	}
}

// El .key de Tika de 2018 (con Index.zip y sin vista previa) ya se convierte.
func TestKeynoteIWADeTika(t *testing.T) {
	res, err := Import("charla.key", leerIWork(t, "testKeynote2018.key"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindPresentation || strings.Contains(res.Content, "Conversion not available") {
		t.Errorf("Keynote 2018: kind %q\n%.400s", res.Kind, res.Content)
	}
}

// Una hoja que, convertida, no cabe en un documento no es un error: se abre como lo que no
// se convierte, con el aviso y la vista previa.
func TestNumbersIWAQueNoCabeAbreLaVistaPrevia(t *testing.T) {
	// 1024 filas × 10 columnas que apuntan todas a la misma celda de 100 letras.
	celda := celdaV5(3, 0x8, u32le(1))
	pos := make([]byte, 0, 20)
	for range 10 {
		pos = binary.LittleEndian.AppendUint16(pos, 0)
	}
	objs := make([]objPrueba, 0, 9)
	objs = append(objs,
		objPrueba{1, tipoIWADocumento, pbR(1, 2)},
		objPrueba{2, tipoIWAHoja, append(pbS(1, "H"), pbR(2, 3)...)},
		objPrueba{3, tipoIWATabla, pbR(2, 600)},
		objPrueba{602, tipoIWALista, pbB(3, pbV(1, 1), pbS(3, strings.Repeat("x", 100)))},
	)
	teselas := make([]byte, 0, 64)
	for n := range uint64(4) {
		filas := make([]byte, 0, filasPorTesela*64)
		for f := range uint64(filasPorTesela) {
			filas = append(filas, pbB(5, pbV(1, f), pbB(6, celda), pbB(7, pos))...)
		}
		objs = append(objs, objPrueba{700 + n, tipoIWATesela, filas})
		teselas = append(teselas, pbB(1, pbV(1, n), pbR(2, 700+n))...)
	}
	objs = append(objs, objPrueba{600, tipoIWAModelo, append(append(pbV(6, 1024), pbV(7, 10)...), pbB(4, pbB(3, teselas), pbR(4, 602))...)})
	res, err := Import("enorme.numbers", pagesIWA(t, iwaDe(1<<16, objs...)))
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindDocument || !strings.Contains(res.Content, "Preview · .numbers spreadsheet") {
		t.Errorf("kind %q:\n%.300s", res.Kind, res.Content)
	}
}
