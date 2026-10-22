// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"encoding/json"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

func dosPDF(t *testing.T, campos map[string]string, a, b string, sesion bool) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, texto := range []string{a, b} {
		e := pdfsimple.Nuevo()
		e.Texto(texto, "F1", 14)
		p, _ := w.CreateFormFile("file", "doc.pdf")
		_, _ = p.Write(e.Bytes())
	}
	for k, v := range campos {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/pdf/compare", &buf)
	r.Header.Set("Content-Type", w.FormDataContentType())
	if sesion {
		r = r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: "irati", Tenant: "acme"}))
	}
	return r
}

func TestCompararVisualPorHTTP(t *testing.T) {
	h := &Handler{}

	// Resumen: sin imágenes, con regiones.
	rec := httptest.NewRecorder()
	h.pdfComparar(rec, dosPDF(t, map[string]string{"modo": "visual"}, "El plazo es de treinta dias", "El plazo es de sesenta dias", true))
	if rec.Code != http.StatusOK {
		t.Fatalf("visual: %d %s", rec.Code, rec.Body)
	}
	var res struct {
		Modo    string `json:"modo"`
		Iguales bool   `json:"iguales"`
		Paginas []struct {
			Pagina     int     `json:"pagina"`
			Porcentaje float64 `json:"porcentaje"`
			Imagen     string  `json:"imagen"`
		} `json:"paginas"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Modo != "visual" || res.Iguales || len(res.Paginas) != 1 || res.Paginas[0].Porcentaje <= 0 {
		t.Fatalf("resumen: %s", rec.Body)
	}
	if res.Paginas[0].Imagen != "" {
		t.Error("el resumen no debería llevar imágenes")
	}

	// Una página: con su resaltado, que es un PNG.
	rec = httptest.NewRecorder()
	h.pdfComparar(rec, dosPDF(t, map[string]string{"modo": "visual", "pagina": "1", "escala": "1"}, "El plazo es de treinta dias", "El plazo es de sesenta dias", true))
	if rec.Code != http.StatusOK {
		t.Fatalf("pagina: %d %s", rec.Code, rec.Body)
	}
	var una struct {
		Pagina struct {
			Imagen []byte `json:"imagen"`
			Escala float64
		} `json:"pagina"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &una); err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(una.Pagina.Imagen)); err != nil {
		t.Fatalf("la imagen no es un PNG: %v", err)
	}

	// Iguales.
	rec = httptest.NewRecorder()
	h.pdfComparar(rec, dosPDF(t, map[string]string{"modo": "visual"}, "mismo", "mismo", true))
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"iguales":true`)) {
		t.Fatalf("iguales: %d %s", rec.Code, rec.Body)
	}

	// El modo de texto sigue siendo el de siempre.
	rec = httptest.NewRecorder()
	h.pdfComparar(rec, dosPDF(t, nil, "uno", "dos", true))
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"diferencias"`)) {
		t.Fatalf("texto: %d %s", rec.Code, rec.Body)
	}
}

func TestCompararVisualRechazaLoQueNoEncaja(t *testing.T) {
	h := &Handler{}
	casos := map[string]map[string]string{
		"modo desconocido":       {"modo": "magico"},
		"escala enorme":          {"modo": "visual", "escala": "16"},
		"escala no numérica":     {"modo": "visual", "escala": "grande"},
		"tolerancia 0":           {"modo": "visual", "tolerancia": "0"},
		"tolerancia 900":         {"modo": "visual", "tolerancia": "900"},
		"vecindad negativa":      {"modo": "visual", "vecindad": "-1"},
		"vecindad enorme":        {"modo": "visual", "vecindad": "50"},
		"página que no existe":   {"modo": "visual", "pagina": "9"},
		"página no numérica":     {"modo": "visual", "pagina": "x"},
		"página cero":            {"modo": "visual", "pagina": "0"},
		"página con signo":       {"modo": "visual", "pagina": "-1"},
		"escala con exponente":   {"modo": "visual", "escala": "1e400"},
		"escala NaN (no válida)": {"modo": "visual", "escala": "NaN"},
	}
	for nombre, campos := range casos {
		rec := httptest.NewRecorder()
		h.pdfComparar(rec, dosPDF(t, campos, "a", "b", true))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s (esperaba 400)", nombre, rec.Code, rec.Body)
		}
	}
}

func TestCompararVisualExigeSesionYDosFicheros(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	h.pdfComparar(rec, dosPDF(t, map[string]string{"modo": "visual"}, "a", "b", false))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin sesión: %d", rec.Code)
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	e := pdfsimple.Nuevo()
	e.Texto("solo uno", "F1", 12)
	p, _ := w.CreateFormFile("file", "uno.pdf")
	_, _ = p.Write(e.Bytes())
	_ = w.WriteField("modo", "visual")
	_ = w.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/pdf/compare", &buf)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r = r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: "irati", Tenant: "acme"}))
	rec = httptest.NewRecorder()
	h.pdfComparar(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("con un solo PDF: %d", rec.Code)
	}
}
