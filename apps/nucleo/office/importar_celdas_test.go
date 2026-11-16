// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package office

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// En el modo privado, la nota de una celda de un .ods llega a la app con su celda, para
// guardarse anclada a ella como en el servicio.
func TestImportarHojaConNotaLlevaSuCelda(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for nombre, cuerpo := range map[string]string{
		"mimetype": "application/vnd.oasis.opendocument.spreadsheet",
		"content.xml": `<?xml version="1.0"?><office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:dc="http://purl.org/dc/elements/1.1/">` +
			`<office:body><office:spreadsheet><table:table table:name="Hoja1"><table:table-row><table:table-cell/>` +
			`<table:table-cell office:value-type="string"><office:annotation><dc:creator>Ana</dc:creator><text:p>Revisar</text:p></office:annotation><text:p>x</text:p></table:table-cell>` +
			`</table:table-row></table:table></office:spreadsheet></office:body></office:document-content>`,
	} {
		w, _ := zw.Create(nombre)
		_, _ = w.Write([]byte(cuerpo))
	}
	_ = zw.Close()
	imp, err := (&Repository{}).ImportDocument(context.Background(), "libro.ods", buf.Bytes(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Comments) != 1 || imp.Comments[0].Cell == nil ||
		*imp.Comments[0].Cell != (api.ImportadoCelda{Hoja: "h:Hoja1", Fila: "t0", Col: "t1", Text: "B1"}) || imp.Comments[0].Text != "Ana: Revisar" {
		t.Fatalf("comentarios = %+v", imp.Comments)
	}
}

// En el modo privado, un ancla que el servicio docs rechazaría (aquí, un texto citado que la
// pasa de tamaño) tampoco se guarda: la regla es la misma (office/ancla).
func TestComentarioPrivadoConAnclaNoValida(t *testing.T) {
	_, err := (&Repository{}).CreateAnchoredDocumentComment(context.Background(), "doc", "hola", "abc", strings.Repeat("x", 5000))
	if !errors.Is(err, api.ErrAnclaNoValida) {
		t.Fatalf("err = %v, quiero ErrAnclaNoValida", err)
	}
}
