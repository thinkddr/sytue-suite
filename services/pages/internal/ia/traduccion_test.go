// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

func sitioParaTraduccion() ir.Sitio {
	return ir.Sitio{
		Languages: ir.IdiomasSitio{Default: "es", Others: []string{"en"}},
		SEO:       ir.SeoSitio{SiteName: ir.Textos{"es": "Nombre global"}},
		Header:    &ir.Nodo{ID: "cabecera", Type: "button", Label: ir.Textos{"es": "Global"}},
		Pages: []ir.Pagina{
			{ID: "home", Title: ir.Textos{"es": "Inicio", "en": "Manual title"}, Description: ir.Textos{"es": "Descripción", "en": ""}, Sections: []ir.Nodo{
				{ID: "txt-home", Type: "text", Content: ir.TextosRicos{"es": []ir.Tramo{{Text: "Hola"}}, "en": []ir.Tramo{{Text: "Manual", Bold: true}}}},
				{ID: "button-home", Type: "button", Label: ir.Textos{"es": "Entrar"}},
			}},
			{ID: "about", Title: ir.Textos{"es": "Sobre nosotros"}, Sections: []ir.Nodo{{ID: "txt-about", Type: "text", Content: ir.TextosRicos{"es": []ir.Tramo{{Text: "Otra página"}}}}}},
		},
		Components: []ir.Componente{{Root: ir.Nodo{ID: "shared", Type: "button", Label: ir.Textos{"es": "Componente compartido"}}}},
	}
}

func TestElAlcanceDePaginaYConservarNoTraducenOtrosTextos(t *testing.T) {
	site := sitioParaTraduccion()
	input := domain.TranslateInput{From: "es", To: []string{"en"}, Scope: "page:home"}
	texts, err := textosParaTraducir(&site, input, "en")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]ir.TextoTraducible{
		"home/description": {Texto: "Descripción"}, "button-home/label": {Texto: "Entrar"},
	}
	if !reflect.DeepEqual(texts, want) {
		t.Fatalf("conservar/página incluyó existentes o textos globales: %+v", texts)
	}
	input.Overwrite = true
	texts, err = textosParaTraducir(&site, input, "en")
	if err != nil || len(texts) != 4 || texts["home/title"].Texto != "Inicio" || texts["txt-home/content"].Texto != "Hola" {
		t.Fatalf("sobrescribir no incluye traducciones existentes: %+v %v", texts, err)
	}
	input.Scope = "site"
	texts, err = textosParaTraducir(&site, input, "en")
	if err != nil || texts["about/title"].Texto == "" || texts["cabecera/label"].Texto == "" || texts["shared/label"].Texto == "" || texts["seo/site_name"].Texto == "" {
		t.Fatalf("todo el sitio omitió textos: %+v %v", texts, err)
	}
	input.Scope = "page:missing"
	if _, err = textosParaTraducir(&site, input, "en"); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("una página inexistente amplió su alcance: %v", err)
	}
}

func TestCadaDestinoConservaSusPropiasTraducciones(t *testing.T) {
	site := sitioParaTraduccion()
	input := domain.TranslateInput{From: "es", Scope: "page:home"}
	en, err := textosParaTraducir(&site, input, "en")
	if err != nil {
		t.Fatal(err)
	}
	fr, err := textosParaTraducir(&site, input, "fr")
	if err != nil {
		t.Fatal(err)
	}
	if len(en) != 2 || len(fr) != 4 {
		t.Fatalf("el inglés conservado bloqueó un idioma que aún faltaba: en=%v fr=%v", en, fr)
	}
	if site.Pages[0].Title["en"] != "Manual title" {
		t.Fatal("la preparación modificó el borrador")
	}
}

func TestAlcanceInvalidoYTiendaNoCreanTrabajos(t *testing.T) {
	s := Nuevo(config.Config{}, nil, nil, nil, nil, nil, nil, nil)
	for _, scope := range []string{"page", "page:", "page:home/other", "everything"} {
		_, err := s.LanzarTranslate(context.Background(), "acme", "user", uuid.Nil, domain.TranslateInput{Scope: scope}, "pro")
		if !errors.Is(err, ErrAlcanceTraduccion) {
			t.Fatalf("%q llegó a reservar cupo o consultar el store: %v", scope, err)
		}
	}
	if _, err := s.LanzarTranslate(context.Background(), "acme", "user", uuid.Nil, domain.TranslateInput{Scope: "store"}, "pro"); !errors.Is(err, ErrTraduccionTiendaNoDisponible) {
		t.Fatalf("tienda se confundió con sitio: %v", err)
	}
}

func TestLaRespuestaDeIASoloPuedeCambiarElLoteSolicitado(t *testing.T) {
	site := sitioParaTraduccion()
	input := domain.TranslateInput{From: "es", Scope: "page:home"}
	texts, err := textosParaTraducir(&site, input, "en")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request asistente.PeticionPages
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		var batch map[string]string
		if err := json.Unmarshal([]byte(request.Context), &batch); err != nil {
			t.Error(err)
		}
		if len(batch) != 2 || batch["home/title"] != "" || batch["about/title"] != "" {
			t.Errorf("envió textos que debía conservar o de otra página: %v", batch)
		}
		_ = json.NewEncoder(w).Encode(asistente.RespuestaPages{Text: `{"button-home/label":"Enter","home/description":"Description","home/title":"unrequested overwrite","about/title":"out of scope"}`})
	}))
	defer srv.Close()
	s := Nuevo(config.Config{AISecret: "secret"}, nil, asistente.Nuevo(srv.URL, time.Second), nil, nil, nil, nil, nil)
	patch := map[string]map[string]any{}
	if err := s.traducirIdioma(context.Background(), &domain.AIJob{}, "es", "en", "", texts, patch); err != nil {
		t.Fatal(err)
	}
	if len(patch) != 2 || patch["button-home/label"]["en"] != "Enter" || patch["home/title"] != nil || patch["about/title"] != nil {
		t.Fatalf("la IA cambió una clave fuera del lote: %v", patch)
	}
}

func TestUnSegundoLoteFallidoNoPublicaUnParcheParcial(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request asistente.PeticionPages
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if len(request.Context) > 6000 {
			t.Errorf("lote superó el límite: %d bytes", len(request.Context))
		}
		if requests == 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		var batch map[string]string
		if err := json.Unmarshal([]byte(request.Context), &batch); err != nil {
			t.Error(err)
		}
		for key := range batch {
			batch[key] = "Translation from first batch"
		}
		translated, _ := json.Marshal(batch)
		_ = json.NewEncoder(w).Encode(asistente.RespuestaPages{Text: string(translated)})
	}))
	defer srv.Close()
	s := Nuevo(config.Config{AISecret: "secret"}, nil, asistente.Nuevo(srv.URL, time.Second), nil, nil, nil, nil, nil)
	site := ir.Sitio{Languages: ir.IdiomasSitio{Default: "es"}, Pages: []ir.Pagina{{ID: "home", Title: ir.Textos{"es": strings.Repeat("a", 4000)}, Description: ir.Textos{"es": strings.Repeat("b", 4000)}}}}
	job := &domain.AIJob{Status: domain.AIJobStatusRunning, Result: json.RawMessage(`{"stale":"result"}`)}
	s.traducirBorrador(context.Background(), job, &site, domain.TranslateInput{From: "es", To: []string{"en"}, Scope: "site"}, "")
	if requests != 2 || job.Status != domain.AIJobStatusFailed || job.Error == "" || len(job.Result) != 0 {
		t.Fatalf("un error acabó como traducción parcial aplicable: requests=%d job=%+v", requests, job)
	}
}

func TestRespuestaIncompletaYTextoExcesivoFallanSinResultado(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_ = json.NewEncoder(w).Encode(asistente.RespuestaPages{Text: `{"home/title":"Home"}`})
	}))
	defer srv.Close()
	s := Nuevo(config.Config{AISecret: "secret"}, nil, asistente.Nuevo(srv.URL, time.Second), nil, nil, nil, nil, nil)
	site := ir.Sitio{Languages: ir.IdiomasSitio{Default: "es"}, Pages: []ir.Pagina{{ID: "home", Title: ir.Textos{"es": "Inicio"}, Description: ir.Textos{"es": "Descripción"}}}}
	input := domain.TranslateInput{From: "es", To: []string{"en"}, Scope: "site"}
	job := &domain.AIJob{Status: domain.AIJobStatusRunning}
	s.traducirBorrador(context.Background(), job, &site, input, "")
	if requests != 1 || job.Status != domain.AIJobStatusFailed || len(job.Result) != 0 {
		t.Fatalf("un JSON incompleto se presentó como done: %+v", job)
	}
	site.Pages[0].Description["es"] = strings.Repeat(`"`, 4000) // el límite cuenta escapes JSON, no caracteres
	job = &domain.AIJob{Status: domain.AIJobStatusRunning}
	s.traducirBorrador(context.Background(), job, &site, input, "")
	if requests != 1 || job.Status != domain.AIJobStatusFailed || len(job.Result) != 0 {
		t.Fatalf("un lote excesivo llegó al proveedor: requests=%d job=%+v", requests, job)
	}
}

func TestIdiomasCompatiblesYDestinoNuevoEnSitioMonolingue(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request asistente.PeticionPages
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		var batch map[string]string
		if err := json.Unmarshal([]byte(request.Context), &batch); err != nil {
			t.Error(err)
		}
		for key := range batch {
			batch[key] = "Translated"
		}
		translated, _ := json.Marshal(batch)
		_ = json.NewEncoder(w).Encode(asistente.RespuestaPages{Text: string(translated)})
	}))
	defer srv.Close()
	s := Nuevo(config.Config{AISecret: "secret"}, nil, asistente.Nuevo(srv.URL, time.Second), nil, nil, nil, nil, nil)
	site := ir.Sitio{Languages: ir.IdiomasSitio{Default: "es"}, Pages: []ir.Pagina{{ID: "home", Title: ir.Textos{"es": "Inicio"}}}}
	job := &domain.AIJob{Status: domain.AIJobStatusRunning}
	s.traducirBorrador(context.Background(), job, &site, domain.TranslateInput{From: "es", To: []string{"en"}, Scope: "site"}, "")
	if requests != 1 || job.Status != domain.AIJobStatusDone || len(job.Result) == 0 {
		t.Fatalf("bloqueó el nuevo idioma de un sitio monolingüe: %+v", job)
	}
	job = &domain.AIJob{Status: domain.AIJobStatusRunning}
	s.traducirBorrador(context.Background(), job, &site, domain.TranslateInput{From: "en", To: []string{"es"}, Scope: "site"}, "")
	if requests != 1 || job.Status != domain.AIJobStatusFailed || len(job.Result) != 0 {
		t.Fatalf("tradujo desde un idioma aún no habilitado: %+v", job)
	}
	for _, input := range []domain.TranslateInput{
		{From: "es", To: []string{"fr"}},
		{From: "fr", To: []string{"en"}},
		{From: "es", To: []string{"en", "en"}},
		{From: "es", To: []string{"es"}},
	} {
		if err := ValidarIdiomasTraduccion(input); !errors.Is(err, ErrIdiomasTraduccion) {
			t.Fatalf("aceptó idiomas incompatibles: %+v %v", input, err)
		}
	}
}
