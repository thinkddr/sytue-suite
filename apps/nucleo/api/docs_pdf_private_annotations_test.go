// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

func TestPrivatePDFAnnotationsStayLocalAndKeepAuthenticatedReplyActor(t *testing.T) {
	requests := 0
	original := NewPDF("https://docs.invalid", "https://drive.invalid", nil, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("private annotation reached HTTP")
	})})
	data := &privatePDFDataFixture{}
	private, err := original.WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	annotations, _ := json.Marshal([]localpdf.Anotacion{{Pagina: 0, Tipo: localpdf.AnotNota, Rect: [4]float64{10, 10, 40, 40}, Texto: "private annotation", Autor: "initial author"}})
	apply := func(body []byte, fields map[string]string) []byte {
		t.Helper()
		response, err := private.Motor(t.Context(), "/pdf/annotate", fields, nil, PDFFichero{Nombre: "private.pdf", Datos: body})
		if err != nil {
			t.Fatal(err)
		}
		return response.Datos
	}
	added := apply(PDFEnBlanco(), map[string]string{"op": "poner", "anotaciones": string(annotations)})
	replied := apply(added, map[string]string{"op": "responder", "indice": "1", "texto": "private reply", "autor": "forged actor"})
	doc, err := localpdf.LeerPDF(replied)
	if err != nil {
		t.Fatal(err)
	}
	values := doc.AnotacionesDelDocumento()
	if len(values) != 2 || values[1].Texto != "private reply" || values[1].Autor != "owner" {
		t.Fatalf("reply actor/content changed: %+v", values)
	}
	removed := apply(replied, map[string]string{"op": "borrar", "indices": "1"})
	doc, err = localpdf.LeerPDF(removed)
	if err != nil || len(doc.AnotacionesDelDocumento()) != 1 {
		t.Fatal("delete did not stay local", err)
	}
	flattened := apply(removed, map[string]string{"op": "aplanar"})
	doc, err = localpdf.LeerPDF(flattened)
	if err != nil || len(doc.AnotacionesDelDocumento()) != 0 {
		t.Fatal("flatten did not stay local", err)
	}
	if requests != 0 {
		t.Fatal("private annotation used HTTP")
	}
	data.rootFailureAt = data.roots + 2
	response, err := private.Motor(t.Context(), "/pdf/annotate", map[string]string{"op": "poner", "anotaciones": string(annotations)}, nil, PDFFichero{Nombre: "private.pdf", Datos: PDFEnBlanco()})
	if err == nil || len(response.Datos) != 0 || requests != 0 {
		t.Fatal("revoked private annotation escaped", err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := private.Motor(canceled, "/pdf/annotate", nil, nil, PDFFichero{Nombre: "private.pdf", Datos: PDFEnBlanco()}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled annotation started", err)
	}
}
