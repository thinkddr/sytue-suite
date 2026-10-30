// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/klauspost/compress/snappy"
)

// Un protobuf mínimo para armar ficheros de prueba: campo varint, delimitado, referencia
// (mensaje con el id en el 1) y float32.
func pbV(num int, v uint64) []byte {
	return binary.AppendUvarint(binary.AppendUvarint(nil, uint64(num)<<3), v)
}

func pbB(num int, partes ...[]byte) []byte {
	var b []byte
	for _, p := range partes {
		b = append(b, p...)
	}
	return append(binary.AppendUvarint(binary.AppendUvarint(nil, uint64(num)<<3|2), uint64(len(b))), b...)
}

func pbR(num int, id uint64) []byte { return pbB(num, pbV(1, id)) }

func pbF(num int, f float32) []byte {
	return binary.LittleEndian.AppendUint32(binary.AppendUvarint(nil, uint64(num)<<3|5), math.Float32bits(f))
}

func pbD(num int, d float64) []byte {
	return binary.LittleEndian.AppendUint64(binary.AppendUvarint(nil, uint64(num)<<3|1), math.Float64bits(d))
}

func pbS(num int, s string) []byte { return pbB(num, []byte(s)) }

type objPrueba struct {
	id, tipo uint64
	datos    []byte
}

// iwaDe arma un .iwa: los objetos con su cabecera, partidos en trozos Snappy de `trozo`
// bytes para que la prueba cruce los límites entre trozos.
func iwaDe(trozo int, objs ...objPrueba) []byte {
	var plano []byte
	for _, o := range objs {
		cab := append(pbV(1, o.id), pbB(2, pbV(1, o.tipo), pbV(3, uint64(len(o.datos))))...)
		plano = binary.AppendUvarint(plano, uint64(len(cab)))
		plano = append(append(plano, cab...), o.datos...)
	}
	var out []byte
	for len(plano) > 0 {
		n := min(trozo, len(plano))
		c := snappy.Encode(nil, plano[:n])
		out = append(out, 0, byte(len(c)), byte(len(c)>>8), byte(len(c)>>16))
		out = append(out, c...)
		plano = plano[n:]
	}
	return out
}

// pagesIWA es un .pages de 2013 con este Document.iwa.
func pagesIWA(t *testing.T, iwa []byte) []byte {
	t.Helper()
	return zipDe(t, map[string][]byte{"Index/Document.iwa": iwa, "preview.jpg": jpegDe(t, 8, 8)})
}

func TestPagesIWAConvierteElTexto(t *testing.T) {
	texto := "Título\nHola 😀 negrita y cursiva otra línea\n￼\nfin\x05"
	// Posiciones en unidades UTF-16: el emoji ocupa dos.
	pos := func(sub string) uint64 {
		i := strings.Index(texto, sub)
		return uint64(len([]rune(texto[:i])) + strings.Count(texto[:i], "😀"))
	}
	iwa := iwaDe(40,
		objPrueba{1, tipoIWAPages, append(pbR(4, 10), pbR(3, 20)...)},
		objPrueba{10, tipoIWAAlmacen, append(append(
			// El texto puede venir en dos trozos.
			append(pbS(3, texto[:9]), pbS(3, texto[9:])...),
			pbB(5,
				pbB(1, pbV(1, 0), pbR(2, 100)),
				pbB(1, pbV(1, pos("Hola")), pbR(2, 101)),
				pbB(1, pbV(1, pos("￼"))), // sin estilo: sigue con el de antes
				pbB(1, pbV(1, pos("fin"))),
			)...),
			pbB(8,
				pbB(1, pbV(1, pos("negrita")), pbR(2, 200)),
				pbB(1, pbV(1, pos(" y "))),
				pbB(1, pbV(1, pos("cursiva")), pbR(2, 201)),
				pbB(1, pbV(1, pos(" "))),
			)...)},
		objPrueba{100, tipoIWAEstiloParr, pbB(1, pbS(1, "Heading 2"))},
		// Un estilo anónimo que hereda la alineación (centro) de su padre con nombre.
		objPrueba{101, tipoIWAEstiloParr, pbB(1, pbR(3, 102))},
		objPrueba{102, tipoIWAEstiloParr, append(pbB(1, pbS(1, "Body")), pbB(12, pbV(1, 2))...)},
		objPrueba{200, tipoIWAEstiloCar, pbB(11, pbV(1, 1), pbV(11, 1), pbV(12, 1), pbB(7, pbF(3, 1), pbF(4, 0), pbF(5, 0)))},
		// Cursiva, y la negrita definida como «no» aunque el padre la ponga.
		objPrueba{201, tipoIWAEstiloCar, append(pbB(1, pbR(3, 200)), pbB(11, pbV(1, 0), pbV(2, 1))...)},
		// Un cuadro de texto flotante y una imagen (otro tipo), que no se convierte.
		objPrueba{20, 10010, pbB(1, pbV(1, 0), pbB(4, pbR(1, 30)), pbB(4, pbR(1, 31)))},
		objPrueba{30, tipoIWACuadro, pbR(2, 40)},
		objPrueba{31, 3005, nil},
		objPrueba{40, tipoIWAAlmacen, pbS(3, "En un cuadro")},
	)
	res, err := ImportLocalized("informe.pages", pagesIWA(t, iwa), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	for _, quiero := range []string{
		"Conversión parcial · documento .pages",
		`<h2>Título</h2>`,
		`<p style="text-align:center">Hola 😀 <strong><u><s><span style="color:#FF0000">negrita</span></s></u></strong> y <em><u><s><span style="color:#FF0000">cursiva</span></s></u></em><br>otra línea</p>`,
		`<p style="text-align:center"></p>`,
		`<p style="text-align:center">fin</p>`,
		`<p>En un cuadro</p>`,
		`<img src="data:image/jpeg;base64,`,
	} {
		if !strings.Contains(res.Content, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, res.Content)
		}
	}
}

// El fichero de Tika: el mismo documento que testPages.pages, guardado en el formato de
// 2013. Entra su texto y su tabla anclada, sin aviso de conversión parcial.
func TestPagesIWADeTika(t *testing.T) {
	res, err := ImportLocalized("nuevo.pages", leerIWork(t, "testPages2013.pages"), false, "en")
	if err != nil {
		t.Fatal(err)
	}
	for _, quiero := range []string{"Sample pages document", "Some plain text to parse.", "A text box with text.", "<td>Column one</td>", "<td>Cell nine</td>"} {
		if !strings.Contains(res.Content, quiero) {
			t.Errorf("falta %q en:\n%.800s", quiero, res.Content)
		}
	}
	if strings.Contains(res.Content, "Partial conversion") {
		t.Errorf("no debería llevar el aviso de conversión parcial:\n%.400s", res.Content)
	}
}

func TestPagesIWAConvierteListasYTablas(t *testing.T) {
	texto := "Punto 1\nSubpunto 1.1\nPaso A\n\uFFFC\nDespues"
	pos := func(sub string) uint64 { return uint64(strings.Index(texto, sub)) }
	objs := append([]objPrueba{
		{1, tipoIWAPages, pbR(4, 10)},
		{10, tipoIWAAlmacen, append(append(append(
			pbS(3, texto),
			pbB(6,
				pbB(1, pbV(1, 0), pbV(2, 0)),
				pbB(1, pbV(1, pos("Subpunto")), pbV(2, 1)),
				pbB(1, pbV(1, pos("Paso A")), pbV(2, 0)),
			)...),
			pbB(7,
				pbB(1, pbV(1, 0), pbR(2, 300)),
				pbB(1, pbV(1, pos("Paso A")), pbR(2, 301)),
				pbB(1, pbV(1, pos("\uFFFC")), pbR(2, 302)),
			)...),
			pbB(9, pbB(1, pbV(1, pos("\uFFFC")), pbR(2, 400)))...)},
		{300, 2023, pbV(11, 2)},
		{301, 2023, pbV(11, 3)},
		{302, 2023, pbV(11, 0)},
		{400, 2003, pbR(1, 401)},
		{401, tipoIWATabla, pbR(2, 600)},
	}, tablaIWAPrueba()...)
	res, err := Import("listas.pages", pagesIWA(t, iwaDe(1<<16, objs...)))
	if err != nil {
		t.Fatal(err)
	}
	for _, quiero := range []string{
		`<ul><li>Punto 1<ul><li>Subpunto 1.1</li></ul></li></ul>`,
		`<ol><li>Paso A</li></ol>`,
		`<table><tbody><tr><td>Hola &amp; adiós</td>`,
		`<p>Despues</p>`,
	} {
		if !strings.Contains(res.Content, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, res.Content)
		}
	}
}

// El marco roto o fuera de los topes no es un error ni un documento vacío: es el aviso.
func TestMarcoIWARotoAbreElAviso(t *testing.T) {
	bueno := iwaDe(1<<16, objPrueba{1, tipoIWAPages, pbR(4, 10)}, objPrueba{10, tipoIWAAlmacen, pbS(3, "hola")})
	if res, err := Import("a.pages", pagesIWA(t, bueno)); err != nil || !strings.HasPrefix(res.Content, "<p>hola</p>") {
		t.Fatalf("el bueno no se convierte: %v %.200s", err, res.Content)
	}
	tipo := append([]byte{}, bueno...)
	tipo[0] = 1
	largo := append([]byte{}, bueno...)
	largo[3] = 0x7f // un trozo que dice medir más de lo que hay
	// Un bloque que descomprime más de lo que se admite: la cabecera Snappy lo dice antes
	// de descomprimir.
	enorme := binary.AppendUvarint(nil, maxIndice09+1)
	bomba := append([]byte{0, byte(len(enorme)), 0, 0}, enorme...)
	for nombre, iwa := range map[string][]byte{"tipo": tipo, "largo": largo, "bomba": bomba} {
		res, err := Import(nombre+".pages", pagesIWA(t, iwa))
		if err != nil {
			t.Errorf("%s: %v", nombre, err)
			continue
		}
		if !strings.Contains(res.Content, "Preview · .pages document") {
			t.Errorf("%s: quiero el aviso con la vista previa:\n%.300s", nombre, res.Content)
		}
	}
}

// En el formato de 2018 los .iwa van en un Index.zip dentro del paquete.
func TestPagesIWAEnIndexZip(t *testing.T) {
	iwa := iwaDe(1<<16, objPrueba{1, tipoIWAPages, pbR(4, 10)}, objPrueba{10, tipoIWAAlmacen, pbS(3, "dentro")})
	paquete := zipDe(t, map[string][]byte{
		"Informe.pages/Index.zip": zipDe(t, map[string][]byte{"Index/Document.iwa": iwa}),
	})
	res, err := Import("informe.pages", paquete)
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "<p>dentro</p>" {
		t.Errorf("Index.zip: %q", res.Content)
	}
}

// El tope de lo descomprimido se mira en la cabecera de cada bloque, antes de
// descomprimirlo, y cuenta lo que ya llevan los bloques anteriores.
func TestDescomprimirIWARespetaElTope(t *testing.T) {
	iwa := iwaDe(600, objPrueba{1, tipoIWAPages, make([]byte, 1000)})
	plano, err := descomprimirIWA(iwa, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := descomprimirIWA(iwa, len(plano)-1); err == nil {
		t.Errorf("con un tope de %d bytes no debería descomprimir %d", len(plano)-1, len(plano))
	}
}

func TestPagesIWAConvierteImagenesYComentarios(t *testing.T) {
	texto := "Antes texto marcado después\n\uFFFC\nFin"
	pos := func(sub string) uint64 {
		i := strings.Index(texto, sub)
		return uint64(len([]rune(texto[:i])))
	}
	ini := pos("texto marcado")
	fin := ini + uint64(len([]rune("texto marcado")))
	posImg := pos("\uFFFC")

	iwa := iwaDe(1<<16,
		objPrueba{1, tipoIWAPages, append(pbR(4, 10), pbR(3, 20)...)},
		objPrueba{10, tipoIWAAlmacen, append(append(
			pbS(3, texto),
			pbB(9, pbB(1, pbV(1, posImg), pbR(2, 400)))...),
			pbB(23, pbB(1, pbV(1, ini), pbR(2, 500)), pbB(1, pbV(1, fin)))...)},
		objPrueba{20, 10010, pbB(1, pbV(1, 0), pbB(4, pbR(1, 31)), pbB(4, pbR(1, 32)))},
		objPrueba{31, 3005, append(pbB(4, pbF(1, 1200), pbF(2, 300)), pbR(11, 702)...)},
		objPrueba{32, 2014, pbR(2, 503)},
		objPrueba{400, 2003, pbR(1, 401)},
		objPrueba{401, 3005, append(pbB(4, pbF(1, 240), pbF(2, 120)), pbR(11, 701)...)},
		objPrueba{500, 2013, pbR(1, 501)},
		objPrueba{501, 3056, append(append(append(pbS(1, "Cambiar verbo"), pbB(2, pbD(1, 276048000))...), pbR(3, 600)...), pbR(4, 502)...)},
		objPrueba{502, 3056, append(pbS(1, "Hecho"), pbR(3, 600)...)},
		objPrueba{503, 3056, pbS(1, "Nota al margen")},
		objPrueba{600, 212, pbS(1, "Luis")},
	)
	meta := iwaDe(1<<16, objPrueba{900, 11006, append(
		pbB(4, pbV(1, 701), pbS(3, "foto.jpg"), pbS(4, "foto-701.jpg")),
		pbB(4, pbV(1, 702), pbS(3, "ancha.jpg"), pbS(4, "ancha-702.jpg"))...,
	)})
	paq := zipDe(t, map[string][]byte{
		"Index/Document.iwa":    iwa,
		"Index/Metadata.iwa":    meta,
		"Data/foto-701.jpg":     jpegDe(t, 24, 12),
		"Data/mt-foto-701.jpg":  jpegDe(t, 4, 4),
		"Data/ancha-702.jpg":    jpegDe(t, 120, 30),
		"QuickLook/Preview.jpg": jpegDe(t, 8, 8),
	})

	res, err := ImportLocalized("informe.pages", paq, true, "es")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Comments) != 3 {
		t.Fatalf("comentarios = %+v, quiero 3", res.Comments)
	}
	for _, quiero := range []string{
		`<mark data-comment-id="` + res.Comments[0].ID + `">texto marcado</mark>`,
		`data-w="240" data-h="120"`,
		`data-w="600" data-h="150"`,
	} {
		if !strings.Contains(res.Content, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, res.Content)
		}
	}
	if strings.Contains(res.Content, "Conversión parcial") {
		t.Errorf("no debería llevar aviso de conversión parcial:\n%s", res.Content)
	}
	if c := res.Comments[0]; c.Body != "Cambiar verbo" || c.Quote != "texto marcado" || c.Author != "Luis" || c.Date != "2009-10-01T00:00:00Z" || !c.Anchored {
		t.Errorf("comentario 0: %+v", c)
	}
	if c := res.Comments[1]; c.ParentID != res.Comments[0].ID || c.Body != "Hecho" || c.Author != "Luis" {
		t.Errorf("respuesta 1: %+v", c)
	}
	if c := res.Comments[2]; c.Body != "Nota al margen" || c.Anchored {
		t.Errorf("nota flotante 2: %+v", c)
	}
}
