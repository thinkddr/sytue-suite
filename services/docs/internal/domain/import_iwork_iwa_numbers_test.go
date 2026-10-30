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

// tablaIWAPrueba son los objetos de una tabla de 3×3 (modelo 600) con una celda de cada
// tipo, formatos de número/moneda/porcentaje/duración y estilos de celda; la segunda fila,
// en el formato anterior.
func tablaIWAPrueba() []objPrueba {
	// Formato anterior (v4): número con formato de millares (flags2 bit 0x1 en byte 10).
	viejo := []byte{4, 2, 0, 0, 0x20, 0, 0, 0, 0, 0, 0x01, 0}
	viejo = append(append(viejo, f64le(1234.5)...), u32le(2)...)
	return []objPrueba{
		{600, tipoIWAModelo, append(append(append(pbV(6, 3), pbV(7, 3)...), pbS(8, "Tabla 1")...),
			pbB(4,
				pbB(3, pbB(1, pbV(1, 0), pbR(2, 601))),
				pbR(4, 602),
				pbR(5, 607),
				pbR(6, 603),
				pbR(17, 604),
				pbR(22, 608),
			)...)},
		{601, tipoIWATesela, append(append(
			filaDeTesela(0, true,
				// (0,0) cadena con relleno (#FF8000, clave 1) y estilo de texto (color #0080FF + subrayado, clave 2)
				celdaV5(3, 0x8|0x20|0x40, u32le(1), u32le(1), u32le(2)),
				// (0,1) moneda (flags 0x4000, clave 1)
				celdaV5(2, 0x1|0x4000, dec128(125, -1), u32le(1)),
				// (0,2) fecha
				celdaV5(5, 0x4, f64le(276048000)),
			),
			filaDeTesela(1, false,
				viejo,
				celdaV5(9, 0x10, u32le(7)),
				celdaV5(6, 0x2, f64le(1)),
			)...),
			filaDeTesela(2, true,
				// (2,0) porcentaje (flags 0x2000, clave 3)
				celdaV5(2, 0x2|0x2000, f64le(0.5), u32le(3)),
				// (2,1) duración corta (flags 0x10000, clave 4): 8 días = 1w 1d
				celdaV5(7, 0x2|0x10000, f64le(8*86400), u32le(4)),
				// (2,2) duración con dos puntos (flags 0x10000, clave 5): 1h 1m 1s 1ms = 1:01:01.001
				celdaV5(7, 0x2|0x10000, f64le(3661.001), u32le(5)),
			)...)},
		{602, tipoIWALista, pbB(3, pbV(1, 1), pbS(3, "Hola & adiós"))},
		{603, tipoIWALista, pbB(3, pbV(1, 1))},
		{604, tipoIWALista, pbB(3, pbV(1, 7), pbR(9, 605))},
		{605, tipoIWARico, pbR(1, 606)},
		{606, tipoIWAAlmacen, pbS(3, "Rico")},
		{607, tipoIWALista, append(
			pbB(3, pbV(1, 1), pbR(4, 609)),
			pbB(3, pbV(1, 2), pbR(4, 610))...)},
		{608, tipoIWALista, append(append(append(append(
			pbB(3, pbV(1, 1), pbB(6, pbV(1, 257), pbV(2, 2))),
			pbB(3, pbV(1, 2), pbB(6, pbV(1, 256), pbV(2, 1), pbV(5, 1)))...),
			pbB(3, pbV(1, 3), pbB(6, pbV(1, 258), pbV(2, 0)))...),
			pbB(3, pbV(1, 4), pbB(6, pbV(1, 268), pbV(7, 1), pbV(15, 1), pbV(16, 2)))...),
			pbB(3, pbV(1, 5), pbB(6, pbV(1, 268), pbV(7, 0), pbV(15, 4), pbV(16, 32)))...)},
		{609, 6004, pbB(11, pbB(1, pbB(1, pbF(3, 1), pbF(4, 0.5), pbF(5, 0))))},
		{610, tipoIWAEstiloParr, pbB(11, pbV(11, 1), pbB(7, pbF(3, 0), pbF(4, 0.5), pbF(5, 1)))},
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
		"A2": "1234.5", "B2": "Rico", "C2": "TRUE",
		"A3": "0.5", "B3": "1w 1d", "C3": "1:01:01.001",
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
	if f := m.Sheets[1].Formats["B1"]; f.ID != "currency" || f.Decimals == nil || *f.Decimals != 2 {
		t.Errorf("B1 formato %+v, quiero currency con 2 decimales", f)
	}
	if f := m.Sheets[1].Formats["A2"]; f.ID != "thousands" || f.Decimals == nil || *f.Decimals != 1 {
		t.Errorf("A2 formato %+v, quiero thousands con 1 decimal", f)
	}
	if f := m.Sheets[1].Formats["A3"]; f.ID != "percent" || f.Decimals == nil || *f.Decimals != 0 {
		t.Errorf("A3 formato %+v, quiero percent con 0 decimales", f)
	}
	if st := m.Sheets[1].Styles["A1"]; st.Fill != "#FF8000" || st.Color != "#0080FF" || !st.U {
		t.Errorf("A1 estilo %+v, quiero Fill=#FF8000 Color=#0080FF U=true", st)
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
		{31, tipoIWAAlmacen, append(pbS(3, "Uno\nDos"), pbB(6, pbB(1, pbV(1, 0), pbV(2, 0)), pbB(1, pbV(1, 4), pbV(2, 1)))...)},
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
	if b := s.Boxes[1]; b.HTML != "<ul><li>Uno<ul><li>Dos</li></ul></li></ul>" || b.X != 25 || b.Y != 50 || b.W != 50 || b.H != 50 {
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

// El .key de Tika de 2018 (con Index.zip y sin vista previa) ya se convierte con sus
// niveles de viñeta.
func TestKeynoteIWADeTika(t *testing.T) {
	raw := leerIWork(t, "testKeynote2018.key")
	res, err := Import("charla.key", raw)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindPresentation || strings.Contains(res.Content, "Conversion not available") {
		t.Errorf("Keynote 2018: kind %q\n%.400s", res.Kind, res.Content)
	}
	// El almacén 9099 del propio fichero tiene los 5 niveles de viñeta anidados (0..4).
	bs := (&lectorIWA{doc: abrirPaqueteIWork(raw).iwa()}).bloques(9099)
	for i, quiero := range []string{"Level Badan Satu", "Level Badan Dua", "Level Badan Tiga", "Level Badan Empat", "Level Badan Lima"} {
		if i >= len(bs) || bs[i].para == nil || bs[i].para.level != i || bs[i].para.plain() != quiero {
			t.Fatalf("nivel %d en 9099: %+v", i, bs)
		}
	}
}

func TestKeynoteIWAConImagenesYNodoRaiz(t *testing.T) {
	objs := []objPrueba{
		{1, tipoIWADocumento, pbR(2, 2)},
		// Árbol con nodo raíz en el campo 1 e hijos en el campo 1 de cada nodo (Keynote 6).
		{2, tipoIWAPresentacion, append(pbB(3, pbR(1, 3)), pbB(4, pbF(1, 1024), pbF(2, 768))...)},
		{3, tipoIWANodo, pbR(1, 4)},
		{4, tipoIWANodo, pbR(2, 10)},
		{10, tipoIWADiapositiva, append(append(pbR(7, 20), pbR(7, 21)...), pbR(27, 24)...)},
		{20, 3005, append(formaIWA(128, 192, 512, 384), pbR(11, 801)...)},
		{21, 2014, nil}, // nota adhesiva en la diapositiva: no cuenta como pérdida
		{24, tipoIWANota, pbR(1, 32)},
		{32, tipoIWAAlmacen, pbS(3, "Hablar despacio")},
		{90, 11006, pbB(4, pbV(1, 801), pbS(3, "foto.jpg"), pbS(4, "foto-801.jpg"))},
	}
	paq := zipDe(t, map[string][]byte{
		"Index/Document.iwa":   iwaDe(1<<16, objs...),
		"Data/foto-801.jpg":    jpegDe(t, 64, 48),
		"Data/st-foto-801.jpg": jpegDe(t, 4, 4),
	})
	res, err := ImportLocalized("charla.key", paq, false, "es")
	if err != nil {
		t.Fatal(err)
	}
	var m slidesModel
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Slides) != 1 || len(m.Slides[0].Boxes) != 1 {
		t.Fatalf("diapositivas: %+v", m.Slides)
	}
	s := m.Slides[0]
	if b := s.Boxes[0]; b.Kind != "image" || !strings.HasPrefix(b.Src, "data:image/jpeg;base64,") || b.X != 12.5 || b.Y != 25 || b.W != 50 || b.H != 50 {
		t.Errorf("imagen en diapositiva: %+v", b)
	}
	if s.Notes != "Hablar despacio" {
		t.Errorf("notas: %q", s.Notes)
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

// TestNumbersIWATablaDinamicaV3CombinadasYGrafico comprueba:
//   - el modelo de resumen (6316) de una tabla dinámica añade la columna y la fila de totales;
//   - las celdas v3 (c[0]=3, c[1]=0, c[2]=tipo) y su formato en flags2 se convierten;
//   - el mapa de celdas combinadas (6144 en ds.ref(13)) pasa a Merges;
//   - un gráfico IWA (5000) con título, etiquetas y rejilla de valores se convierte en hoja
//     de .numbers, tabla en .pages y caja de tabla en .key sin aviso de conversión parcial.
func TestNumbersIWATablaDinamicaV3CombinadasYGrafico(t *testing.T) {
	// Celda v3 cadena (tipo=3 en c[2], bandera 0x10 en c[4]): clave 1 en offset 12.
	v3Cadena := append([]byte{3, 0, 3, 0, 0x10, 0, 0, 0, 0, 0, 0, 0}, u32le(1)...)
	// Celda v3 número (tipo=2 en c[2]) con estilo de celda (flags & 0x2 en offset 12), número (flags & 0x20 en offset 16) y formato en flags2 (byte 10 = 1, en offset 24).
	v3Numero := []byte{3, 0, 2, 0, 0x22, 0, 0, 0, 0, 0, 0x01, 0}
	v3Numero = append(append(append(v3Numero, u32le(99)...), f64le(42.5)...), u32le(1)...)

	// Rango combinado B1:C1 -> origen (fila=0, col=1) = 0x00010000, tamaño (filas=1, cols=2) = 0x00020001.
	rangoB1C1 := pbB(1,
		pbB(1, append([]byte{0x0d}, u32le(0x00010000)...)),
		pbB(2, append([]byte{0x0d}, u32le(0x00020001)...)),
	)

	// Gráfico 5000 con geometría, título "Ventas 2026", filas ["Norte","Sur"], columnas ["T1","T2"] y valores.
	rejillaGrafico := append(append(append(append(
		pbS(2, "Norte"),
		pbS(2, "Sur")...),
		pbS(3, "T1")...),
		pbS(3, "T2")...),
		append(
			pbB(4, pbB(1, pbD(1, 10)), pbB(1, pbD(1, 20))),
			pbB(4, pbB(1, pbD(1, 30)), pbB(1, pbD(1, 40)))...,
		)...,
	)
	objGrafico := append(append(formaIWA(102.4, 76.8, 512, 384), pbS(11, "Ventas 2026")...), pbB(4, rejillaGrafico)...)

	objs := []objPrueba{
		{1, tipoIWADocumento, pbR(1, 2)},
		{2, tipoIWAHoja, append(append(pbS(1, "Dinámica"), pbR(2, 3)...), pbR(2, 4)...)},
		// Tabla 3: modelo 600 (2×3) + modelo de resumen 630 (totales en col 3 y fila 2).
		{3, tipoIWATabla, append(pbR(2, 600), pbR(4, 630)...)},
		{4, 5000, objGrafico},
		{600, tipoIWAModelo, append(append(append(pbV(6, 2), pbV(7, 3)...), pbS(8, "Ventas Pivot")...),
			pbB(4,
				pbB(3, pbB(1, pbV(1, 0), pbR(2, 601))),
				pbR(4, 602),
				pbR(13, 613),
				pbR(22, 608),
			)...)},
		{601, tipoIWATesela, append(
			filaDeTesela(0, true, v3Cadena, celdaV5(3, 0x8, u32le(2)), nil),
			filaDeTesela(1, false, v3Numero, celdaV5(2, 0x2, f64le(7.5)), nil)...,
		)},
		{602, tipoIWALista, append(
			pbB(3, pbV(1, 1), pbS(3, "Región")),
			pbB(3, pbV(1, 2), pbS(3, "Combinada B1:C1"))...,
		)},
		{608, tipoIWALista, pbB(3, pbV(1, 1), pbB(6, pbV(1, 256), pbV(2, 1), pbV(5, 1)))},
		{613, 6144, rangoB1C1},
		// SummaryModel (6316): añade columna 3 ("Grand Total", 50) y fila 2 ("Grand Total", 50).
		{630, 6316, pbB(2,
			pbB(3, pbB(1, pbV(1, 0), pbR(2, 631))),
			pbR(4, 632),
		)},
		{631, tipoIWATesela, append(append(
			filaDeTesela(0, true, nil, nil, nil, celdaV5(3, 0x8, u32le(1))),
			filaDeTesela(1, true, nil, nil, nil, celdaV5(2, 0x2, f64le(50)))...),
			filaDeTesela(2, true, celdaV5(3, 0x8, u32le(1)), celdaV5(2, 0x2, f64le(50)))...,
		)},
		{632, tipoIWALista, pbB(3, pbV(1, 1), pbS(3, "Grand Total"))},
	}

	res, err := ImportLocalized("pivot.numbers", pagesIWA(t, iwaDe(64, objs...)), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	var m sheetModel
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Sheets) != 2 || m.Sheets[0].Name == "Aviso" {
		t.Fatalf("hojas (no debe haber Aviso): %+v", m.Sheets)
	}
	h0 := m.Sheets[0]
	if h0.Cells["A1"] != "Región" || h0.Cells["B1"] != "Combinada B1:C1" || h0.Cells["A2"] != "42.5" ||
		h0.Cells["D1"] != "Grand Total" || h0.Cells["D2"] != "50" || h0.Cells["A3"] != "Grand Total" || h0.Cells["B3"] != "50" {
		t.Errorf("celdas con resumen y v3: %v", h0.Cells)
	}
	if f := h0.Formats["A2"]; f.ID != "thousands" || f.Decimals == nil || *f.Decimals != 1 {
		t.Errorf("formato de celda v3 A2 = %+v, quiero thousands con 1 decimal", f)
	}
	if len(h0.Merges) != 1 || h0.Merges[0] != "B1:C1" {
		t.Errorf("combinadas = %v, quiero [B1:C1]", h0.Merges)
	}
	h1 := m.Sheets[1]
	if !strings.Contains(h1.Name, "Ventas 2026") || h1.Cells["B1"] != "T1" || h1.Cells["C1"] != "T2" ||
		h1.Cells["A2"] != "Norte" || h1.Cells["B2"] != "10" || h1.Cells["C3"] != "40" {
		t.Errorf("hoja del gráfico: %+v", h1)
	}

	// En .pages el mismo gráfico anclado (2003) sale como tabla en el documento sin aviso.
	objsPages := []objPrueba{
		{1, tipoIWAPages, pbR(4, 2)},
		{2, tipoIWAAlmacen, append(pbS(3, "Informe\n\uFFFC"), pbB(9, pbB(1, pbV(1, 8), pbR(2, 3)))...)},
		{3, 2003, pbR(1, 4)},
		{4, 5000, objGrafico},
	}
	resP, err := ImportLocalized("informe.pages", pagesIWA(t, iwaDe(64, objsPages...)), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resP.Content, "Conversión parcial") || !strings.Contains(resP.Content, "<table>") ||
		!strings.Contains(resP.Content, "Ventas 2026") || !strings.Contains(resP.Content, "<td>Norte</td>") || !strings.Contains(resP.Content, "<td>40</td>") {
		t.Errorf("gráfico en .pages:\n%s", resP.Content)
	}

	// En .key el gráfico sale como caja de tabla en la diapositiva sin diapositiva de aviso.
	objsKey := []objPrueba{
		{1, tipoIWADocumento, pbR(2, 2)},
		{2, tipoIWAPresentacion, append(pbB(3, pbR(2, 3)), pbB(4, pbF(1, 1024), pbF(2, 768))...)},
		{3, tipoIWANodo, pbR(2, 10)},
		{10, tipoIWADiapositiva, pbR(7, 4)},
		{4, 5000, objGrafico},
	}
	resK, err := ImportLocalized("charla.key", pagesIWA(t, iwaDe(64, objsKey...)), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	var mk slidesModel
	if err := json.Unmarshal([]byte(resK.Content), &mk); err != nil {
		t.Fatal(err)
	}
	if len(mk.Slides) != 1 || len(mk.Slides[0].Boxes) != 1 || !strings.Contains(mk.Slides[0].Boxes[0].HTML, "<table>") ||
		!strings.Contains(mk.Slides[0].Boxes[0].HTML, "Norte") || mk.Slides[0].Boxes[0].X != 10 || mk.Slides[0].Boxes[0].Y != 10 {
		t.Errorf("gráfico en .key: %+v", mk.Slides)
	}
}
