// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// Describir enseña la MINIATURA al modelo, en base64 y etiquetada como JPEG, con la
// lengua de la sesión en la cabecera, y lee el JSON aunque venga envuelto.
func TestDescribirEnsenyaLaMiniaturaYLeeLaRespuesta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/assets/f1/variants/miniatura":
			_, _ = io.WriteString(w, "JPEG")
		case "POST /api/v1/assist":
			var c map[string]string
			_ = json.NewDecoder(r.Body).Decode(&c)
			if c["action"] != "describir-foto" || c["kind"] != "photo" || c["esfuerzo"] != "medium" ||
				c["imagen"] != base64.StdEncoding.EncodeToString([]byte("JPEG")) || c["imagen_tipo"] != "image/jpeg" ||
				c["context"] != "" || c["instruction"] != instruccionDescribir {
				t.Errorf("cuerpo de /assist: %v", c)
			}
			if r.Header.Get("Accept-Language") != "es" {
				t.Errorf("idioma: %q", r.Header.Get("Accept-Language"))
			}
			_, _ = io.WriteString(w, `{"text":"Claro:\n`+"```json"+`\n{\"descripcion\":\"Una cala {al} atardecer\",\"etiquetas\":[\"Playa\",\"barca\",\"playa\",\"\"]}\n`+"```"+`"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	d, e, err := NewPhotos(srv.URL, nil, nil).Describir(context.Background(), NewIA(srv.URL, nil, nil), "f1", "es")
	if err != nil || d != "Una cala {al} atardecer" || !slices.Equal(e, []string{"playa", "barca"}) {
		t.Fatalf("Describir = %q %v %v", d, e, err)
	}
}

// Sin modelo, el servicio contesta 503 `ai_not_configured` y la app lo dice con su
// texto propio, no con el de «ha fallado».
func TestIANoConfiguradaSeDistingue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"code":"ai_not_configured","message":"x"}`)
	}))
	defer srv.Close()
	_, err := TerminosRelacionados(context.Background(), NewIA(srv.URL, nil, nil), "playa", "es")
	if !IANoConfigurada(err) {
		t.Fatalf("un 503 ai_not_configured no se reconoce: %v", err)
	}
}

func TestLeerTerminos(t *testing.T) {
	got := LeerTerminos("Aquí van: [\"Orilla\", \"mar\", 3, \"orilla\", \"arena\", \"ola\", \"costa\", \"sol\", \"agua\"] ¡listo!")
	if !slices.Equal(got, []string{"orilla", "mar", "arena", "ola", "costa", "sol"}) {
		t.Errorf("LeerTerminos = %v", got)
	}
	if got := LeerTerminos("no sé"); got == nil || len(got) != 0 {
		t.Errorf("sin lista debe ser vacía y no nil: %v", got)
	}
}

// Los rangos de la receta mandan sobre lo que diga el modelo, y un recorte del lienzo
// entero no es un recorte.
func TestLeerEdicionAcotaYDescartaElLienzoEntero(t *testing.T) {
	e, ok := LeerEdicion(`{"explicacion":"Más luz.","ajustes":{"exposicion":7.123,"contraste":-0.456,"saturacion":"x"},"recorte":{"x":0,"y":0,"ancho":1,"alto":1}}`)
	if !ok || *e.Ajustes.Exposicion != 5 || *e.Ajustes.Contraste != -0.46 || e.Ajustes.Saturacion != nil || e.Recorte != nil {
		t.Fatalf("LeerEdicion = %+v %v", e, ok)
	}
	e, ok = LeerEdicion(`{"explicacion":"Reencuadre.","ajustes":{},"recorte":{"x":0.1,"y":0.2,"ancho":0.95,"alto":0.5}}`)
	if !ok || e.Recorte == nil || e.Recorte.Ancho != 0.9 || e.Recorte.Alto != 0.5 {
		t.Fatalf("recorte: %+v %v", e.Recorte, ok)
	}
	if _, ok := LeerEdicion(`{"explicacion":"Nada.","ajustes":{}}`); ok {
		t.Error("sin ajustes ni recorte no hay edición")
	}
}
