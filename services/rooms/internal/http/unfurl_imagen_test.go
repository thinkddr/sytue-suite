// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// TestUnfurlNoDevuelveLaURLDeOrigen prueba la propiedad que se buscaba: el host
// que eligió quien pegó el enlace NO llega al navegador de quien lee. Si llegara,
// ese host vería la IP de cada lector y la hora a la que abrió el canal.
//
// Se comprueba sobre la respuesta JSON —lo que consume el cliente— y no sobre una
// función interna: si alguien vuelve a poner ahí la URL remota, este test cae
// aunque el resto del código parezca correcto.
func TestUnfurlNoDevuelveLaURLDeOrigen(t *testing.T) {
	const remota = "https://cdn.tercero.example/portada.png"
	f := &fakeService{unfurl: domain.LinkUnfurl{
		URL:      "https://ejemplo.eu/articulo",
		Title:    "Un artículo",
		ImageURL: remota,
	}}
	rec := httptest.NewRecorder()
	newHandlers(f).Unfurl(rec, req(http.MethodGet, "/?url=https://ejemplo.eu/articulo", "", claims(), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("código %d, cuerpo %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "cdn.tercero.example") {
		t.Fatalf("la respuesta filtra el host remoto, que es lo que hay que evitar: %s", rec.Body.String())
	}
	var resp struct {
		Unfurl struct {
			ImageURL string `json:"image_url"`
		} `json:"unfurl"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if !strings.HasPrefix(resp.Unfurl.ImageURL, "/api/v1/unfurl/imagen/") {
		t.Fatalf("image_url debe apuntar a nuestro proxy, apunta a %q", resp.Unfurl.ImageURL)
	}
}

// TestUnfurlSinImagenNoInventaProxy: si el enlace no traía imagen, no se debe
// devolver una ruta de proxy que luego responda 404 en cada vista previa.
func TestUnfurlSinImagenNoInventaProxy(t *testing.T) {
	f := &fakeService{unfurl: domain.LinkUnfurl{URL: "https://ejemplo.eu/a", Title: "Sin portada"}}
	rec := httptest.NewRecorder()
	newHandlers(f).Unfurl(rec, req(http.MethodGet, "/?url=https://ejemplo.eu/a", "", claims(), nil))

	if strings.Contains(rec.Body.String(), "unfurl/imagen") {
		t.Fatalf("sin imagen no debe haber ruta de proxy: %s", rec.Body.String())
	}
}
