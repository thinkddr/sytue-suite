// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
)

func TestFamiliaIncrustadaIdentificaProgramaMapaYVariante(t *testing.T) {
	construir := func(l *lienzo, programa []byte, unicode string, flags int64) *fuentePDF {
		return l.construir(pdfDict{
			"Subtype": pdfName("TrueType"), "BaseFont": pdfName("ABCDEF+Subset"),
			"FontDescriptor": pdfDict{"Flags": flags, "FontFile2": &pdfStream{Dict: pdfDict{}, Raw: programa}},
			"ToUnicode":      &pdfStream{Dict: pdfDict{}, Raw: []byte("1 beginbfchar <41> <" + unicode + "> endbfchar")},
		})
	}
	nuevo := func() *lienzo {
		return &lienzo{d: &PDFDoc{}, caraCSS: map[string]bool{}, fuentes: map[string]*fuentePDF{}}
	}
	l := nuevo()
	primera := construir(l, goregular.TTF, "0041", 4)
	misma := construir(l, goregular.TTF, "0041", 4)
	if primera.familia != misma.familia || strings.Count(l.defs.String(), "@font-face") != 1 {
		t.Error("la misma fuente debe conservar su identidad sin duplicar la regla CSS")
	}
	segundaPagina := nuevo()
	construir(segundaPagina, gobold.TTF, "0041", 4)
	if otraPagina := construir(segundaPagina, goregular.TTF, "0041", 4); otraPagina.familia != primera.familia {
		t.Fatal("la identidad no debe depender de la página ni del orden de encuentro")
	}
	for _, caso := range []struct {
		nombre   string
		programa []byte
		unicode  string
		flags    int64
	}{
		{"programa", gobold.TTF, "0041", 4},
		{"mapa", goregular.TTF, "0042", 4},
		{"variante", goregular.TTF, "0041", 68},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			if otra := construir(nuevo(), caso.programa, caso.unicode, caso.flags); otra.familia == primera.familia {
				t.Fatal("fuentes distintas comparten una identidad CSS global")
			}
		})
	}
}

// Word incrusta fuentes simples simbólicas con códigos renumerados y una cmap
// Macintosh formato 6. Esa plataforma no convierte los códigos del PDF en Unicode.
func TestFuenteSimpleMacintoshPorCodigo(t *testing.T) {
	fuente, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	a, _ := fuente.GlyphIndex(&buf, 'A')
	e, _ := fuente.GlyphIndex(&buf, 'e')
	be := binary.BigEndian
	tabla := make([]byte, 12+14)
	be.PutUint16(tabla[2:], 1)
	be.PutUint16(tabla[4:], 1) // Macintosh Roman
	be.PutUint32(tabla[8:], 12)
	be.PutUint16(tabla[12:], 6)
	be.PutUint16(tabla[14:], 14)
	be.PutUint16(tabla[18:], 65)
	be.PutUint16(tabla[20:], 2)
	be.PutUint16(tabla[22:], uint16(e))
	be.PutUint16(tabla[24:], uint16(a))
	programa, ok := reemplazarTablaSfnt(goregular.TTF, "cmap", tabla)
	if !ok {
		t.Fatal("no se pudo construir la fuente de prueba")
	}
	// El subconjunto del PDF no necesita la tabla de nombres de glifo post.
	programa, ok = reemplazarTablaSfnt(programa, "post", nil)
	if !ok {
		t.Fatal("no se pudo retirar la metadata post")
	}
	programa, ok = reemplazarTablaSfnt(programa, "OS/2", nil)
	if !ok {
		t.Fatal("no se pudo retirar la metadata OS/2")
	}
	for _, raster := range []bool{false, true} {
		nombre := "svg"
		if raster {
			nombre = "png"
		}
		t.Run(nombre, func(t *testing.T) {
			l := &lienzo{d: &PDFDoc{}, caraCSS: map[string]bool{}, fuentes: map[string]*fuentePDF{}}
			if raster {
				l.r = &rasterizador{}
				l.soloRaster = true
			}
			f := l.construir(pdfDict{
				"Subtype": pdfName("TrueType"), "BaseFont": pdfName("ABCDEF+Subset"),
				"FontDescriptor": pdfDict{"Flags": int64(4), "FontFile2": &pdfStream{Dict: pdfDict{}, Raw: programa}},
				"ToUnicode":      &pdfStream{Dict: pdfDict{}, Raw: []byte("2 beginbfchar <41> <0065> <42> <0041> endbfchar")},
			})
			var cara *sfnt.Font
			if raster {
				cara, _ = f.caraSfnt()
			} else {
				css := l.defs.String()
				_, resto, ok := strings.Cut(css, "base64,")
				if !ok {
					t.Fatal("el SVG no incrustó la fuente")
				}
				datos, err := base64.StdEncoding.DecodeString(strings.SplitN(resto, "\"", 2)[0])
				if err != nil {
					t.Fatal(err)
				}
				if len(datosTablaSfnt(datos, "post")) != 32 || len(datosTablaSfnt(datos, "OS/2")) != 78 {
					t.Fatal("faltan metadatos del subconjunto para el navegador")
				}
				cara, err = sfnt.Parse(datos)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, caso := range []struct {
				codigo uint32
				texto  string
				gid    sfnt.GlyphIndex
			}{{65, "e", e}, {66, "A", a}} {
				var gid sfnt.GlyphIndex
				if raster {
					var ok bool
					gid, ok = f.indiceDeGlifo(cara, &buf, f.glifoDe(caso.codigo, false))
					if !ok {
						t.Fatal("glifo ausente")
					}
				} else {
					gid, err = cara.GlyphIndex(&buf, primeraRuna(caso.texto))
					if err != nil {
						t.Fatal(err)
					}
				}
				if !raster {
					visual, err := cara.GlyphIndex(&buf, rune(0xE000+caso.codigo))
					if err != nil || visual != caso.gid {
						t.Errorf("el código visual %d perdió su contorno: %d", caso.codigo, visual)
					}
				}
				if gid != caso.gid {
					t.Errorf("%s: código %d dibuja glifo %d, debe dibujar %d", caso.texto, caso.codigo, gid, caso.gid)
				}
			}
			if !raster {
				e := ejecucion{l: l, tm: identidad, estado: estadoG{tamano: 12, escalaH: 1, ctm: identidad, relleno: "#000000"}}
				e.emitirTexto(f, []glifo{f.glifoDe(65, false), f.glifoDe(66, false)}, 12)
				if !strings.Contains(l.sb.String(), "\ue041\ue042") || !strings.Contains(l.sb.String(), `-webkit-user-select:none;user-select:none;pointer-events:none`) || !strings.Contains(l.sb.String(), `fill-opacity="0">eA</text>`) {
					t.Fatal("SVG no separa contorno visual y texto seleccionable")
				}
			}
		})
	}
}

// Una Macintosh formato 0 auténtica sí se consulta por carácter Unicode: el
// PDF puede usar un código distinto y ToUnicode sigue mandando sobre el texto.
func TestFuenteMacintoshFormatoCeroConservaCaracteres(t *testing.T) {
	cara, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	a, _ := cara.GlyphIndex(&buf, 'A')
	tabla := make([]byte, 12+262)
	be := binary.BigEndian
	be.PutUint16(tabla[2:], 1)
	be.PutUint16(tabla[4:], 1)
	be.PutUint32(tabla[8:], 12)
	be.PutUint16(tabla[14:], 262)
	tabla[18+'A'] = byte(a)
	programa, ok := reemplazarTablaSfnt(goregular.TTF, "cmap", tabla)
	if !ok {
		t.Fatal("cmap inválida")
	}
	fuente := &fuentePDF{programa: programa}
	cara, _ = fuente.caraSfnt()
	gid, ok := fuente.indiceDeGlifo(cara, &buf, glifo{codigo: 0x50, texto: "A"})
	if !ok || gid != a {
		t.Fatalf("Macintosh formato0: glifo %d, debe ser %d", gid, a)
	}
}

func TestCIDToGIDDoesNotWrapAnOversizedCode(t *testing.T) {
	face, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	var buffer sfnt.Buffer
	font := &fuentePDF{dosBytes: true, cidAGid: []byte{0, 1}}
	if gid, ok := font.indiceDeGlifo(face, &buffer, glifo{codigo: 0}); !ok || gid != 1 {
		t.Fatal("valid glyph mapping changed")
	}
	for _, code := range []uint32{0x10000, 0x80000000, 0xffffffff} {
		if _, ok := font.indiceDeGlifo(face, &buffer, glifo{codigo: code}); ok {
			t.Fatalf("oversized CID %#x wrapped into the mapping", code)
		}
	}
}

func TestMetadatosSfntRechazanTablaFueraDelFichero(t *testing.T) {
	for _, truncado := range []bool{false, true} {
		datos := append([]byte(nil), goregular.TTF...)
		if truncado {
			datos = datos[:20]
		} else {
			// Entrada head cuyo desplazamiento escapa del fichero. La suma se hace
			// en 64 bits, sin envolver y aceptar bytes de otra tabla por accidente.
			for i := 0; i < int(binary.BigEndian.Uint16(datos[4:6])); i++ {
				pos := 12 + 16*i
				if string(datos[pos:pos+4]) == "head" {
					binary.BigEndian.PutUint32(datos[pos+8:pos+12], 0xFFFFFFF0)
					break
				}
			}
		}
		if len(datosTablaSfnt(datos, "head")) != 0 {
			t.Fatal("se aceptó una tabla fuera del fichero")
		}
		f := &fuentePDF{}
		if salida := f.completarMetadatosTTF(datos); !bytes.Equal(datos, salida) {
			t.Fatal("se reconstruyó metadata de una fuente truncada")
		}
	}
}

func TestSVGCopiaActualTextDeLaLigaduraSinCodigosPrivados(t *testing.T) {
	l := &lienzo{d: &PDFDoc{}}
	e := ejecucion{l: l, tm: identidad, estado: estadoG{tamano: 12, escalaH: 1, ctm: identidad, relleno: "#000000"}}
	f := &fuentePDF{cmapVisual: true}
	e.aplicar("BDC", []pdfObj{pdfName("Span"), pdfDict{"ActualText": pdfString("ti")}})
	e.emitirTexto(f, []glifo{{codigo: 65, texto: "J"}}, 6)
	e.aplicar("EMC", nil)
	if !strings.Contains(l.sb.String(), `fill-opacity="0">ti</text>`) {
		t.Fatal("la ligadura no conserva su texto lógico")
	}
	if !strings.Contains(l.sb.String(), "\ue041</text>") {
		t.Fatal("el texto lógico alteró el contorno de la ligadura")
	}
	if got := e.textoDeLaMarca("J"); got != "J" {
		t.Fatalf("ActualText escapó fuera de su grupo: %q", got)
	}
}

func TestActualTextReferenciadoGrandeNoSeDuplicaEnLaPila(t *testing.T) {
	grande := pdfString(bytes.Repeat([]byte("x"), maxTextoMarcadoBytes+1))
	l := &lienzo{d: &PDFDoc{objs: map[int]pdfObj{7: grande}}}
	e := ejecucion{l: l}
	ops := []pdfObj{pdfName("Span"), pdfDict{"ActualText": pdfRef{Num: 7}}}
	for i := 0; i < 257; i++ {
		e.aplicar("BDC", ops)
	}
	if len(e.marcas) != 256 || e.marcasOmitidas != 1 {
		t.Fatal("la pila de metadata no está acotada")
	}
	for _, m := range e.marcas {
		if m.presente || m.actual != "" {
			t.Fatal("se decodificó y retuvo ActualText excesivo")
		}
	}
	e.aplicar("EMC", nil)
	if len(e.marcas) != 256 || e.marcasOmitidas != 0 {
		t.Fatal("cerrar un grupo omitido altera la pila")
	}
	if got := e.textoDeLaMarca("texto del PDF"); got != "texto del PDF" {
		t.Fatal("metadata excesiva sustituyó el texto del PDF")
	}
}
