// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package aihttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/ai/internal/provider"
)

// sistemaVisto guarda el system prompt con el que se llamó al proveedor.
type sistemaVisto struct{ sistema chan string }

func (s sistemaVisto) Complete(_ context.Context, system, _ string, _ provider.Opciones) (provider.Resultado, error) {
	s.sistema <- system
	return provider.Resultado{Texto: "ok"}, nil
}
func (sistemaVisto) Name() string   { return "fake" }
func (sistemaVisto) Modelo() string { return "modelo-de-prueba" }

func sistemaDe(t *testing.T, locale, acceptLanguage string) string {
	t.Helper()
	prov := sistemaVisto{sistema: make(chan string, 1)}
	h := &handlers{cfg: testCfg(), provider: prov}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assist", strings.NewReader(`{"instruction":"resume"}`))
	req = req.WithContext(authn.Into(req.Context(), authn.Claims{Subject: "u1", Tenant: "t1", Locale: locale}))
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	rec := httptest.NewRecorder()
	h.assist(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	return <-prov.sistema
}

// El idioma del perfil manda sobre el del navegador: es el que la persona eligió.
func TestAssist_UsaElIdiomaDelPerfil(t *testing.T) {
	s := sistemaDe(t, "fr", "de-DE,de;q=0.9")
	if !strings.Contains(s, "IDIOMA DEL USUARIO: francés (fr)") {
		t.Fatalf("el system prompt no ordena el idioma del perfil:\n%s", s)
	}
}

// Un token anterior al claim cae al Accept-Language.
func TestAssist_SinIdiomaEnElTokenNegociaAcceptLanguage(t *testing.T) {
	if s := sistemaDe(t, "", "it-IT,it;q=0.9"); !strings.Contains(s, "(it)") {
		t.Fatalf("sin claim debería usar Accept-Language:\n%s", s)
	}
}

// Sin nada que diga el idioma no se fuerza el inglés base.
func TestAssist_SinIdiomaNoHayDirectiva(t *testing.T) {
	if s := sistemaDe(t, "", ""); s != provider.SystemPrompt {
		t.Fatalf("sin idioma el prompt debe quedar intacto:\n%s", s)
	}
}

// Un locale regional del perfil se reduce a su idioma.
func TestAssist_LocaleRegionalSeReduce(t *testing.T) {
	if s := sistemaDe(t, "es-ES", ""); !strings.Contains(s, "español (es)") {
		t.Fatalf("es-ES debería leerse como español:\n%s", s)
	}
}
