// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

// Una plantilla exportada se vuelve a abrir como lo que es: el mismo contenido, marcada
// como plantilla (abrirla crea un documento nuevo), tanto por la extensión como por lo
// que declara el paquete cuando viene renombrado. Y un documento normal NO es plantilla.
func TestPlantillasDeOficinaIdaYVuelta(t *testing.T) {
	casos := []struct {
		kind   Kind
		format BlobFormat
		ext    string
		mime   string
		doc    string // la extensión del documento normal
		estado func() *State
		texto  string
	}{
		{KindDocument, FormatOOXML, "dotx", "application/vnd.openxmlformats-officedocument.wordprocessingml.template", "docx", func() *State { return stateWithContent(t, "doc:html", "<h1>Carta tipo</h1><p>Estimado cliente</p>") }, "Estimado cliente"},
		{KindDocument, FormatODF, "ott", "application/vnd.oasis.opendocument.text-template", "odt", func() *State { return stateWithContent(t, "doc:html", "<h1>Carta tipo</h1><p>Estimado cliente</p>") }, "Estimado cliente"},
		{KindSpreadsheet, FormatOOXML, "xltx", "application/vnd.openxmlformats-officedocument.spreadsheetml.template", "xlsx", func() *State {
			return stateWithContent(t, "doc:sheet", `{"sheets":[{"name":"H","cells":{"A1":"Estimado cliente"}}]}`)
		}, "Estimado cliente"},
		{KindSpreadsheet, FormatODF, "ots", "application/vnd.oasis.opendocument.spreadsheet-template", "ods", func() *State {
			return stateWithContent(t, "doc:sheet", `{"sheets":[{"name":"H","cells":{"A1":"Estimado cliente"}}]}`)
		}, "Estimado cliente"},
		{KindPresentation, FormatOOXML, "potx", "application/vnd.openxmlformats-officedocument.presentationml.template", "pptx", func() *State { return stateWithContent(t, "doc:slides", slidesJSON(t)) }, "Segunda diapositiva"},
		{KindPresentation, FormatODF, "otp", "application/vnd.oasis.opendocument.presentation-template", "odp", func() *State { return stateWithContent(t, "doc:slides", slidesJSON(t)) }, "Segunda diapositiva"},
	}
	for _, c := range casos {
		t.Run(c.ext, func(t *testing.T) {
			normal, _, _, err := Export(c.kind, "Plantilla", c.estado(), c.format)
			if err != nil {
				t.Fatal(err)
			}
			datos, mime, ext, err := ComoPlantilla(c.kind, c.format, normal)
			if err != nil || ext != c.ext || mime != c.mime {
				t.Fatalf("ComoPlantilla: %v %q %q", err, ext, mime)
			}
			if c.format == FormatODF {
				zr, _ := zip.NewReader(bytes.NewReader(datos), int64(len(datos)))
				if zr.File[0].Name != "mimetype" || zr.File[0].Method != zip.Store {
					t.Fatal("ODF: el mimetype debe seguir primero y sin comprimir")
				}
			}
			res, err := Import("Plantilla."+c.ext, datos)
			if err != nil {
				t.Fatalf("importar la plantilla: %v", err)
			}
			if res.Kind != c.kind || !res.Template || !strings.Contains(res.Content, c.texto) {
				t.Fatalf("plantilla importada: kind=%s template=%v contenido=%.120s", res.Kind, res.Template, res.Content)
			}
			// Renombrada a la extensión del documento, la declaración del paquete la delata.
			if res, err := Import("Plantilla."+c.doc, datos); err != nil || !res.Template || res.Kind != c.kind {
				t.Fatalf("renombrada a .%s: %v %+v", c.doc, err, res)
			}
			// Y el documento de partida no es plantilla.
			if res, err := Import("Normal."+c.doc, normal); err != nil || res.Template {
				t.Fatalf("un documento normal no es plantilla: %v %+v", err, res)
			}
			// El paquete de la plantilla declara la plantilla y NO el documento.
			if c.format == FormatOOXML {
				ct := zipParts(t, datos)["[Content_Types].xml"]
				if !strings.Contains(ct, ".template.main+xml") || strings.Contains(ct, ".document.main+xml") ||
					strings.Contains(ct, "spreadsheetml.sheet.main+xml") || strings.Contains(ct, "presentationml.presentation.main+xml") {
					t.Fatalf("[Content_Types].xml: %s", ct)
				}
			}
		})
	}
}

func TestComoPlantillaRechazaLoQueNoTiene(t *testing.T) {
	if _, _, _, err := ComoPlantilla(KindDocument, FormatPDF, []byte("x")); !errors.Is(err, ErrInvalidFormat) {
		t.Fatalf("PDF: %v", err)
	}
	// Un paquete que no es del tipo declarado no se «convierte» en silencio.
	odt, _, _, _ := Export(KindDocument, "T", textState(t, "x"), FormatODF)
	if _, _, _, err := ComoPlantilla(KindSpreadsheet, FormatODF, odt); !errors.Is(err, ErrInvalidFormat) {
		t.Fatalf("tipo cruzado: %v", err)
	}
	if _, _, _, err := ComoPlantilla(KindDocument, FormatOOXML, []byte("no soy un zip")); !errors.Is(err, ErrInvalidFormat) {
		t.Fatalf("no zip: %v", err)
	}
}

func TestEsPlantillaDeOficinaPorExtension(t *testing.T) {
	for _, n := range []string{"a.dotx", "A.XLTX", "x.potm", "y.ott", "y.ots", "y.otp"} {
		if !esPlantillaDeOficina(n, nil) {
			t.Errorf("%s debería ser plantilla", n)
		}
	}
	for _, n := range []string{"a.docx", "b.odt", "c.txt", "plantilla"} {
		if esPlantillaDeOficina(n, []byte("texto")) {
			t.Errorf("%s no es plantilla", n)
		}
	}
}
