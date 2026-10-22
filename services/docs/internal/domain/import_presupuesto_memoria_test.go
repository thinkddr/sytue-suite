// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// parteEnTrozos es una parte de un paquete que se escribe a trozos: cabecera, `n` veces lo
// que devuelve `trozo(i)` y cola. Así el test no tiene la parte entera en memoria.
type parteEnTrozos struct {
	nombre, cabecera, cola string
	trozo                  func(i int) string
}

func paqueteEnTrozos(t *testing.T, n int, fijas map[string]string, partes ...parteEnTrozos) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for nombre, cuerpo := range fijas {
		w, err := zw.Create(nombre)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, cuerpo)
	}
	for _, p := range partes {
		w, err := zw.Create(p.nombre)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, p.cabecera)
		for i := range n {
			_, _ = io.WriteString(w, p.trozo(i))
		}
		_, _ = io.WriteString(w, p.cola)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// textoDeRelleno es el texto de cada celda o párrafo; con el número delante, ninguno se repite.
func textoDeRelleno(i int) string {
	return strconv.Itoa(i) + " Lorem ipsum dolor sit amet, consectetur adipiscing elit"
}

const odfNS = ` xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0"`

// ficheroDeTexto es un fichero del formato de `nombre` con `n` celdas o párrafos de texto.
// El .xlsx va con tabla de cadenas compartidas, que es lo que más carga el presupuesto por
// byte convertido (la cadena se lee en la tabla y otra vez al colocarla en su celda).
func ficheroDeTexto(t *testing.T, nombre string, n int) []byte {
	parrafo := func(abre, cierra string) func(int) string {
		return func(i int) string { return abre + textoDeRelleno(i) + cierra }
	}
	switch nombre {
	case "grande.xlsx":
		return paqueteEnTrozos(t, n, map[string]string{
			"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Datos" r:id="rId1"/></sheets></workbook>`,
			"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		}, parteEnTrozos{
			"xl/sharedStrings.xml", `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`, `</sst>`,
			parrafo(`<si><t>`, `</t></si>`),
		},
			parteEnTrozos{
				"xl/worksheets/sheet1.xml", `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`, `</sheetData></worksheet>`,
				func(i int) string {
					f := strconv.Itoa(i + 1)
					return `<row r="` + f + `"><c r="A` + f + `" t="s"><v>` + strconv.Itoa(i) + `</v></c></row>`
				},
			})
	case "grande.ods":
		return paqueteEnTrozos(t, n, map[string]string{"mimetype": "application/vnd.oasis.opendocument.spreadsheet"},
			parteEnTrozos{
				"content.xml", `<office:document-content` + odfNS + `><office:body><office:spreadsheet><table:table table:name="Datos">`,
				`</table:table></office:spreadsheet></office:body></office:document-content>`,
				parrafo(`<table:table-row><table:table-cell office:value-type="string"><text:p>`, `</text:p></table:table-cell></table:table-row>`),
			})
	case "grande.docx":
		return paqueteEnTrozos(t, n, nil, parteEnTrozos{
			"word/document.xml",
			`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`, `</w:body></w:document>`,
			parrafo(`<w:p><w:r><w:t>`, `</w:t></w:r></w:p>`),
		})
	case "grande.odt":
		return paqueteEnTrozos(t, n, map[string]string{"mimetype": "application/vnd.oasis.opendocument.text"},
			parteEnTrozos{
				"content.xml", `<office:document-content` + odfNS + `><office:body><office:text>`,
				`</office:text></office:body></office:document-content>`, parrafo(`<text:p>`, `</text:p>`),
			})
	case "grande.pptx":
		return paqueteEnTrozos(t, n, map[string]string{
			"ppt/presentation.xml":            `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="rId1"/></p:sldIdLst></p:presentation>`,
			"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="slides/slide1.xml"/></Relationships>`,
		}, parteEnTrozos{
			"ppt/slides/slide1.xml",
			`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody>`,
			`</p:txBody></p:sp></p:spTree></p:cSld></p:sld>`, parrafo(`<a:p><a:r><a:t>`, `</a:t></a:r></a:p>`),
		})
	case "grande.odp":
		return paqueteEnTrozos(t, n, map[string]string{"mimetype": "application/vnd.oasis.opendocument.presentation"},
			parteEnTrozos{
				"content.xml", `<office:document-content` + odfNS + `><office:body><office:presentation><draw:page><draw:frame svg:width="20cm" svg:height="10cm"><draw:text-box>`,
				`</draw:text-box></draw:frame></draw:page></office:presentation></office:body></office:document-content>`, parrafo(`<text:p>`, `</text:p>`),
			})
	}
	t.Fatalf("formato sin generador: %s", nombre)
	return nil
}

// conRace lo pone a true race_test.go: con -race, las dos pruebas de abajo tardarían minutos.
var conRace bool

var formatosConPresupuesto = []string{"grande.xlsx", "grande.ods", "grande.docx", "grande.odt", "grande.pptx", "grande.odp"}

// picoDeMemoria es lo más que ocupa el montón mientras corre f, por encima de lo que ya
// ocupaba. El recolector va más a menudo de lo normal para que lo medido sea lo vivo y no
// la basura que aún no se ha recogido.
func picoDeMemoria(f func()) uint64 {
	defer debug.SetGCPercent(debug.SetGCPercent(10))
	runtime.GC()
	muestra := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(muestra)
	base := muestra[0].Value.Uint64()
	var pico atomic.Uint64
	fin, hecho := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(hecho)
		m := []metrics.Sample{{Name: muestra[0].Name}}
		reloj := time.NewTicker(time.Millisecond)
		defer reloj.Stop()
		for {
			metrics.Read(m)
			if v := m[0].Value.Uint64(); v > pico.Load() {
				pico.Store(v)
			}
			select {
			case <-fin:
				return
			case <-reloj.C:
			}
		}
	}()
	f()
	close(fin)
	<-hecho
	return max(pico.Load(), base) - base
}

// TestImportarGrandeCortaAlConvertir: lo que no cabe en un documento se tiene que cortar
// MIENTRAS se convierte. Un .xlsx de 14 MB llegaba a 2 GB de memoria antes de fallar,
// porque el tope de lo convertido se miraba al final, y lo mismo pasaba con los otros
// cinco formatos; con dos importaciones así a la vez, el pod caía. Con el presupuesto
// (import_presupuesto.go) lo que ocupa la importación es, como mucho, el paquete
// descomprimido y poco más.
func TestImportarGrandeCortaAlConvertir(t *testing.T) {
	if testing.Short() || conRace {
		t.Skip("genera decenas de MB de XML por formato: sin -short y sin -race")
	}
	for _, nombre := range formatosConPresupuesto {
		data := ficheroDeTexto(t, nombre, 300_000)
		descomprimido := uint64(0)
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range zr.File {
			descomprimido += f.UncompressedSize64
		}
		var errImp error
		var res *ImportResult
		pico := picoDeMemoria(func() { res, errImp = Import(nombre, data) })
		t.Logf("%s: %d MB descomprimidos, pico de %d MB", nombre, descomprimido>>20, pico>>20)
		// Un libro que no cabe se abre recortado (recortarLibro); lo demás no se abre.
		if hoja := nombre == "grande.xlsx"; hoja && (errImp != nil || !res.Truncated) {
			t.Errorf("%s: quiero el libro recortado, tengo %v", nombre, errImp)
		} else if !hoja && !errors.Is(errImp, ErrDemasiadoGrande) {
			t.Errorf("%s: quiero ErrDemasiadoGrande, tengo %v", nombre, errImp)
		}
		// Lo descomprimido se tiene entero en memoria (readZip), y el modelo puede crecer hasta
		// que el presupuesto corta: el doble de lo que cabe, varias veces eso en estructuras. El
		// .xlsx, que se recorta, tiene además su tabla de cadenas entera y el modelo de la
		// primera N que no cabe (hasta cuatro veces lo que cabe).
		// Medido el 2026-10-16: antes del presupuesto, de 120 a 246 MB; después, de 36 a 84.
		if tope := descomprimido + 64<<20; pico > tope {
			t.Errorf("%s: la importación llegó a %d MB; el tope es %d MB (los %d MB descomprimidos y 64 más)",
				nombre, pico>>20, tope>>20, descomprimido>>20)
		}
	}
}

// TestPresupuestoNoCortaLoQueCabe: el presupuesto carga una estimación por arriba, con el
// corte en el doble de lo que cabe. Un fichero que convertido cabe justo tiene que abrirse
// igual que sin presupuesto, en todos los formatos.
func TestPresupuestoNoCortaLoQueCabe(t *testing.T) {
	if conRace {
		t.Skip("una docena de conversiones de casi 1 MB: sin -race")
	}
	for _, nombre := range formatosConPresupuesto {
		tam := func(n int) int {
			data := ficheroDeTexto(t, nombre, n)
			_, content, _, err := convert(detectFormat(nombre, data), data, false, "", nil)
			if err != nil {
				t.Fatalf("%s: %v", nombre, err)
			}
			return tamanoSembrado(content)
		}
		// El tamaño crece en línea con n: con dos puntos se apunta al 99 % del tope.
		t1, t2 := tam(1000), tam(2000)
		n := 1000 + int(float64(MaxContenidoSembrado*99/100-t1)/float64(t2-t1)*1000)
		for tam(n) > MaxContenidoSembrado {
			n -= n / 100
		}
		if res, err := Import(nombre, ficheroDeTexto(t, nombre, n)); err != nil {
			t.Errorf("%s: con %d elementos cabe (%d bytes) y la importación falla: %v", nombre, n, tam(n), err)
		} else if tamanoSembrado(res.Content) < MaxContenidoSembrado*9/10 {
			t.Errorf("%s: la prueba no está junto al tope: %d bytes", nombre, tamanoSembrado(res.Content))
		}
	}
}
