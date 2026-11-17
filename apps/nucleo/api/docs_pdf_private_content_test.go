// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"strings"
	"testing"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
)

func TestPrivatePDFContentEditsStayOnDeviceAndPreserveOriginal(t *testing.T) {
	requests := 0
	data := &privatePDFDataFixture{}
	private, err := NewPDF("https://docs.invalid", "https://drive.invalid", nil, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("private content edit reached HTTP")
	})}).WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	w := pdfsimple.Nuevo()
	w.Texto("Private original text", "F1", 12)
	raw := w.Bytes()
	before := bytes.Clone(raw)
	apply := func(body []byte, fields map[string]string, parts ...PDFParte) PDFRespuesta {
		t.Helper()
		response, err := private.Motor(t.Context(), "/pdf/edit", fields, parts, PDFFichero{Nombre: "private.pdf", Datos: body})
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	analysis := apply(raw, map[string]string{"op": "analizar"})
	var editable localpdf.AnalisisEdicion
	if err := json.Unmarshal(analysis.Datos, &editable); err != nil || len(editable.Fragmentos) == 0 {
		t.Fatal("private editable text unavailable", err)
	}
	changes, err := json.Marshal([]localpdf.CambioDeTexto{{Pagina: 0, ID: editable.Fragmentos[0].ID, Nuevo: "Private revised text"}})
	if err != nil {
		t.Fatal(err)
	}
	edited := apply(raw, map[string]string{"op": "texto", "cambios": string(changes)})
	parsed, err := localpdf.LeerPDF(edited.Datos)
	if err != nil {
		t.Fatal(err)
	}
	text, err := parsed.TextoPlano(0)
	if err != nil || !strings.Contains(text, "Private revised text") {
		t.Fatal("private text edit lost its change", err)
	}
	search := `{"buscar":"original","reemplazar":"changed"}`
	listed := apply(raw, map[string]string{"op": "buscar-reemplazar", "busqueda": search})
	var found struct {
		Matches []localpdf.Coincidencia `json:"coincidencias"`
	}
	if err := json.Unmarshal(listed.Datos, &found); err != nil || len(found.Matches) != 1 {
		t.Fatal("private replacement preview unavailable", err)
	}
	replaced := apply(raw, map[string]string{"op": "buscar-reemplazar", "busqueda": search, "aplicar": "1"})
	if replaced.Cabecera.Get("X-Sytue-Reemplazos") != "1" {
		t.Fatal("replacement count lost", replaced.Cabecera)
	}
	parsed, err = localpdf.LeerPDF(replaced.Datos)
	if err != nil {
		t.Fatal(err)
	}
	text, err = parsed.TextoPlano(0)
	if err != nil || !strings.Contains(text, "changed") {
		t.Fatal("private replacement lost", err)
	}
	bookmarked := apply(raw, map[string]string{"op": "indice", "marcadores": `[{"Titulo":"Private section","Pagina":0,"Nivel":0}]`})
	parsed, err = localpdf.LeerPDF(bookmarked.Datos)
	if err != nil || len(parsed.IndiceDelDocumento()) != 1 {
		t.Fatal("private bookmark lost", err)
	}
	removed := apply(bookmarked.Datos, map[string]string{"op": "indice"})
	parsed, err = localpdf.LeerPDF(removed.Datos)
	if err != nil || len(parsed.IndiceDelDocumento()) != 0 {
		t.Fatal("clearing bookmarks failed", err)
	}
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 20, 20))); err != nil {
		t.Fatal(err)
	}
	imagePDF, err := localpdf.ImagenesAPDF([]localpdf.ImagenAPagina{{Datos: imageData.Bytes(), Nombre: "original.png"}})
	if err != nil {
		t.Fatal(err)
	}
	analysis = apply(imagePDF, map[string]string{"op": "analizar"})
	if err := json.Unmarshal(analysis.Datos, &editable); err != nil || len(editable.Imagenes) == 0 {
		t.Fatal("private image analysis unavailable", err)
	}
	imageRemoved := apply(imagePDF, map[string]string{"op": "imagen", "id": editable.Imagenes[0].ID})
	parsed, err = localpdf.LeerPDF(imageRemoved.Datos)
	if err != nil {
		t.Fatal(err)
	}
	imageAnalysis, err := parsed.AnalizarEdicion(0)
	if err != nil || len(imageAnalysis.Imagenes) != 1 || imageAnalysis.Imagenes[0].Ancho != 1 || imageAnalysis.Imagenes[0].Alto != 1 {
		t.Fatal("private image removal did not preserve the engine's empty placeholder", err)
	}
	var replacementData bytes.Buffer
	if err := png.Encode(&replacementData, image.NewRGBA(image.Rect(0, 0, 50, 10))); err != nil {
		t.Fatal(err)
	}
	imageReplaced := apply(imagePDF, map[string]string{"op": "imagen", "id": editable.Imagenes[0].ID}, PDFParte{Campo: "imagen", Datos: replacementData.Bytes()})
	parsed, err = localpdf.LeerPDF(imageReplaced.Datos)
	if err != nil {
		t.Fatal(err)
	}
	imageAnalysis, err = parsed.AnalizarEdicion(0)
	if err != nil || len(imageAnalysis.Imagenes) != 1 || imageAnalysis.Imagenes[0].Ancho != 50 || imageAnalysis.Imagenes[0].Alto != 10 {
		t.Fatal("private replacement image was not used", err)
	}
	invalid, err := private.Motor(t.Context(), "/pdf/edit", map[string]string{"op": "imagen", "id": editable.Imagenes[0].ID}, []PDFParte{{Campo: "imagen", Datos: []byte("not an image")}}, PDFFichero{Nombre: "private.pdf", Datos: imagePDF})
	if err == nil || len(invalid.Datos) != 0 {
		t.Fatal("invalid replacement image error was lost", err)
	}
	data.rootFailureAt = data.roots + 2
	revoked, err := private.Motor(t.Context(), "/pdf/edit", map[string]string{"op": "indice"}, nil, PDFFichero{Nombre: "private.pdf", Datos: raw})
	if err == nil || len(revoked.Datos) != 0 || requests != 0 || !bytes.Equal(raw, before) {
		t.Fatal("content edit escaped authorization or changed the source", err, requests)
	}
}
