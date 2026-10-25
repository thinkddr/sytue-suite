// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestRecorridoConSesion pulsa todo lo que se ve con un PDF abierto contra el motor
// falso y cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	campos := &motorDeCampos{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pdf/render" {
			campos.ServeHTTP(w, r)
			return
		}
		// El recorrido enseña un documento dibujable. El motorDeCampos conserva
		// sus respuestas de error para los tests de campos que no piden imágenes.
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.FormValue("formato") != "png" {
			campos.ServeHTTP(w, r)
			return
		}
		escala, err := strconv.ParseFloat(r.FormValue("escala"), 64)
		if err != nil || escala <= 0 {
			http.Error(w, "escala de dibujo inválida", http.StatusBadRequest)
			return
		}
		var imagen bytes.Buffer
		// El contenido de motorDeCampos mide 100×140, el dibujo del arnés es A4.
		if err := png.Encode(&imagen, paginaDePrueba(escala*100/595)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Paginas map[int][]byte `json:"paginas"`
			Total   int            `json:"total"`
		}{map[int][]byte{1: imagen.Bytes(), 2: imagen.Bytes()}, 2})
	}))
	t.Cleanup(srv.Close)
	pdf := api.NewPDF(srv.URL, srv.URL, nil, nil)
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 800))
	if os.Getenv("SYTUE_CAPTURAS_PLAY") != "" {
		// Cargar directamente a la escala de la ficha evita un segundo lote
		// pendiente cuando Retratar aplica las medidas de iPhone, iPad o Mac.
		recorrido.AMedida(w)
	}
	listo := make(chan struct{}, 64)
	p.trasRed = func() { listo <- struct{}{} }
	p.Abrir(&Documento{pdf: pdf, ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}, datos: api.PDFEnBlanco()})
	esperarRed(t, listo) // el contenido
	esperarRed(t, listo) // el primer lote
	if p.Error != "" || len(p.hojas) != 2 {
		t.Fatalf("el documento de las capturas no está listo: error=%q, hojas=%d", p.Error, len(p.hojas))
	}
	for _, h := range p.hojas {
		if h.estado != HojaDibujada {
			t.Fatalf("la página %d sigue cargando o falló antes de retratar: %d", h.Numero(), h.estado)
		}
	}
	montaje.ComprobarRotacion(t, w, "certipdf/documento", iconos.PDF, true)
	recorrido.Comprobar(t, w, 400)
}
