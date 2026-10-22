// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestUnODPDeDriveSeAbreEnDiapos es el camino nativo de un .odp pulsado en Drive: Drive
// lo reclama para Diapos (`sytue-slides:/open-from-drive?file=…`), Diapos lee esa
// apertura, trae el fichero de Drive, el servicio lo convierte y la presentación se
// enseña —en solo lectura, porque es una conversión— con sus diapositivas y sus cajas
// dentro del lienzo.
//
// El fichero es un .odp de LibreOffice de verdad (el del corpus de Impress que usa el
// importador), y lo que devuelve la conversión es lo que su importador saca de él
// (testdata/odp-libreoffice.json). Las rutas se comprueban: un prefijo mal escrito en el
// cliente pasaría en verde con un servidor que no las mirase.
//
// VISTO EN ROJO (2026-09-24, en el frontal): con la conversión de antes —las cajas en %
// de un lienzo fijo de 25,4 × 14,288 y no de los 28 × 15,75 de Impress— todas las cajas
// de las diapositivas acababan al 104,7 % del ancho.
func TestUnODPDeDriveSeAbreEnDiapos(t *testing.T) {
	odp := webref.Read(t, "services/docs/internal/domain/testdata/odf/impress-7-1.odp")
	convertido, err := os.ReadFile("testdata/odp-libreoffice.json")
	if err != nil {
		t.Fatal(err)
	}

	// Drive reclama el .odp para Diapos, y Diapos lee esa apertura.
	enlace := api.RutaDeApertura("slides", api.Referencia{ID: "f1", Ruta: "/Charlas/charla.odp"})
	if p, ok := api.AppQueAbre("charla.odp"); !ok || p != "slides" {
		t.Fatalf("un .odp lo abre %q", p)
	}
	u, err := url.Parse(api.Esquema("slides") + ":" + enlace)
	if err != nil {
		t.Fatal(err)
	}
	ap, ok := api.LeerApertura("slides", u)
	if !ok || ap.Ref.ID != "f1" {
		t.Fatalf("Diapos no lee la apertura %s: %+v", u, ap)
	}

	var rutas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rutas = append(rutas, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/drive/api/v1/files/f1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(api.File{ID: "f1", Name: "charla.odp", SizeBytes: int64(len(odp))})
		case r.Method == http.MethodGet && r.URL.Path == "/drive/api/v1/files/f1/content":
			_, _ = io.WriteString(w, odp)
		case r.Method == http.MethodPost && r.URL.Path == "/docs/api/v1/import":
			f, h, err := r.FormFile("file")
			if err != nil || h.Filename != "charla.odp" {
				http.Error(w, "sin fichero", http.StatusBadRequest)
				return
			}
			if b, _ := io.ReadAll(f); string(b) != odp {
				http.Error(w, "no es el fichero de Drive", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(api.Importado{Kind: api.DocSlides, Title: "charla", Content: string(convertido)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	imp, err := api.ImportarDeDrive(context.Background(),
		api.NewDrive(srv.URL+"/drive", nil, srv.Client()), api.NewDocs(srv.URL+"/docs", nil, srv.Client()), ap.Ref.ID)
	if err != nil {
		t.Fatalf("no se trae ni se convierte (%v): %v", err, rutas)
	}
	a := NuevaApp(traductor(t), "es", nil, nil)
	a.lanzar = func(f func()) { f() }
	test.NewTempWindow(t, a.Objeto()).Resize(fyne.NewSize(1280, 800))
	if err := a.AbrirImportado(imp); err != nil {
		t.Fatal(err)
	}
	e := a.Editor()
	if e == nil || !a.Contexto().SoloLectura {
		t.Fatal("la conversión se enseña en el editor, en solo lectura")
	}
	m := e.Modelo()
	if len(m.Diapos) < 2 || m.NoPintado() != nil {
		t.Fatalf("se enseñan %d diapositivas y no se pinta %v", len(m.Diapos), m.NoPintado())
	}
	for i, d := range m.Diapos {
		for _, c := range d.Cajas {
			if c.X < -0.5 || c.Y < -0.5 || c.X+c.W > 100.5 || c.Y+c.H > 100.5 {
				t.Errorf("diapositiva %d: la caja %s se sale del lienzo (%v, %v, %v × %v)", i+1, c.ID, c.X, c.Y, c.W, c.H)
			}
		}
	}
	if !strings.Contains(e.Serializar(), "Hello World") {
		t.Error("el texto de la presentación no ha llegado")
	}
	// Lo que no es una presentación no se enseña aquí: vuelve a la web.
	if a.AbrirImportado(api.Importado{Kind: "document", Title: "x"}) == nil {
		t.Error("un documento de texto convertido no es una presentación")
	}
}
