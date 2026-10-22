// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/hub/internal/instaladores"
)

type catalogoFalso struct {
	pedida string
	sumas  []byte
}

func (c *catalogoFalso) Sumas(context.Context) ([]byte, string, error) {
	if c.sumas == nil {
		return nil, "", instaladores.ErrNoEsta
	}
	return c.sumas, "MEUCIQ==", nil
}

func (c *catalogoFalso) Listar(_ context.Context, p string) ([]instaladores.Instalador, error) {
	c.pedida = p
	return []instaladores.Instalador{{App: "hub", Plataforma: p, Fichero: "sytue-hub-macos-universal.dmg", Version: "3.6.0", SHA256: "ab", Bytes: 3}}, nil
}

func (c *catalogoFalso) Abrir(_ context.Context, n string) (io.ReadCloser, instaladores.Instalador, error) {
	if n != "sytue-hub-macos-universal.dmg" {
		return nil, instaladores.Instalador{}, instaladores.ErrNoEsta
	}
	return io.NopCloser(strings.NewReader("dmg")), instaladores.Instalador{Fichero: n, Bytes: 3}, nil
}

// TestLosInstaladoresSeListanYSeBajanConElToken: lo que pide el actualizador de las
// apps de escritorio. Sin bucket es un 503 (las apps siguen como están), una plataforma
// inventada es un 400, y lo que no es un instalador publicado es un 404.
func TestLosInstaladoresSeListanYSeBajanConElToken(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	h.ListarInstaladores(rec, request(t, "GET", "/api/v1/instaladores?plataforma=macos", userClaims(), ""))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"code":"installers_unconfigured"`) {
		t.Fatalf("sin bucket, un 503 con su código: %d %s", rec.Code, rec.Body)
	}

	cat := &catalogoFalso{}
	h.instaladores = cat
	rec = httptest.NewRecorder()
	h.ListarInstaladores(rec, request(t, "GET", "/api/v1/instaladores?plataforma=macos", userClaims(), ""))
	var cuerpo struct {
		Instaladores []instaladores.Instalador `json:"instaladores"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); rec.Code != http.StatusOK || err != nil ||
		len(cuerpo.Instaladores) != 1 || cuerpo.Instaladores[0].Version != "3.6.0" || cat.pedida != "macos" {
		t.Fatalf("lista: %d %s (%v)", rec.Code, rec.Body, err)
	}
	rec = httptest.NewRecorder()
	h.ListarInstaladores(rec, request(t, "GET", "/api/v1/instaladores", userClaims(), ""))
	if rec.Code != http.StatusOK || cat.pedida != "" {
		t.Errorf("sin plataforma son todas (la página de descargas): %d, pedida %q", rec.Code, cat.pedida)
	}
	rec = httptest.NewRecorder()
	h.ListarInstaladores(rec, request(t, "GET", "/api/v1/instaladores?plataforma=beos", userClaims(), ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("plataforma inventada: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.BajarInstalador(rec, withURLParam(request(t, "GET", "/api/v1/instaladores/x", userClaims(), ""), "fichero", "sytue-hub-macos-universal.dmg"))
	if rec.Code != http.StatusOK || rec.Body.String() != "dmg" || rec.Header().Get("Content-Length") != "3" {
		t.Errorf("bajar: %d %q %v", rec.Code, rec.Body, rec.Header())
	}
	rec = httptest.NewRecorder()
	h.BajarInstalador(rec, withURLParam(request(t, "GET", "/api/v1/instaladores/x", userClaims(), ""), "fichero", "SHA256SUMS"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("lo que no es un instalador: %d", rec.Code)
	}
}

// TestElHubDaElSHA256SUMSYSuFirma: el actualizador de las apps los necesita para
// comprobar con su clave embebida lo que instala (CI-05). Sin SHA256SUMS, 404.
func TestElHubDaElSHA256SUMSYSuFirma(t *testing.T) {
	cat := &catalogoFalso{}
	h := &Handlers{instaladores: cat}
	rec := httptest.NewRecorder()
	h.SumasInstaladores(rec, request(t, "GET", "/api/v1/instaladores/sumas", userClaims(), ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("sin SHA256SUMS, 404: %d %s", rec.Code, rec.Body)
	}
	cat.sumas = []byte("# sytue-instaladores 1\n")
	rec = httptest.NewRecorder()
	h.SumasInstaladores(rec, request(t, "GET", "/api/v1/instaladores/sumas", userClaims(), ""))
	var cuerpo struct{ Sumas, Firma string }
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); rec.Code != http.StatusOK || err != nil ||
		cuerpo.Sumas != "# sytue-instaladores 1\n" || cuerpo.Firma != "MEUCIQ==" {
		t.Fatalf("sumas: %d %s (%v)", rec.Code, rec.Body, err)
	}
}
