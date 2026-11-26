// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package asistente

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
)

func nuevo(t *testing.T) (*Asistente, *espacio.Espacio) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	e, err := espacio.Abrir("")
	if err != nil {
		t.Fatal(err)
	}
	a := Abrir(e, secretos.SoloFichero(t.TempDir()))
	// Los servidores de estas pruebas escuchan en este equipo: sin el cliente (el del
	// portero, en la edición), la IA no sale (R-RED).
	a.HTTP = &http.Client{}
	return a, e
}

// Sin cliente, el asistente no sale a la red: falla cerrado aunque falte la cáscara.
func TestSinClienteNoSale(t *testing.T) {
	a, _ := nuevo(t)
	a.HTTP = nil
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("la petición llegó al servidor")
	}))
	defer srv.Close()
	if err := a.Guardar(Config{Proveedor: Local, URL: srv.URL + "/v1", Modelo: "m"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Probar(t.Context()); err == nil {
		t.Fatal("sin cliente ha contestado")
	}
}

func TestApagadaPorDefectoNoDaFuncion(t *testing.T) {
	a, _ := nuevo(t)
	if a.Funcion() != nil || a.Activa() {
		t.Fatal("la IA nace encendida")
	}
	if _, err := a.Asistir(t.Context(), api.PeticionIA{}); !errors.Is(err, ErrApagada) {
		t.Fatal(err)
	}
}

func TestModeloLocalCompatibleConOpenAISinClave(t *testing.T) {
	var cuerpo map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("ruta %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"texto corregido"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	a, e := nuevo(t)
	if err := a.Guardar(Config{Proveedor: Local, URL: srv.URL + "/v1", Modelo: "llama3.2"}, "ignorada"); err != nil {
		t.Fatal(err)
	}
	if a.Clave() != "" {
		t.Fatal("se guardó una clave para un proveedor que no la pide")
	}
	got, err := a.Funcion()(t.Context(), api.PeticionIA{Accion: "corregir", Instruccion: "corrige", Contexto: "ola"})
	if err != nil || got != "texto corregido" {
		t.Fatal(got, err)
	}
	if cuerpo["model"] != "llama3.2" || !strings.Contains(mensajes(cuerpo), "CONTEXTO DEL DOCUMENTO") {
		t.Fatal("prompt sin el contexto delimitado", cuerpo)
	}
	// La configuración se guarda sin la clave.
	raw, _ := os.ReadFile(e.Ajuste("ia.json"))
	if strings.Contains(string(raw), "ignorada") {
		t.Fatal(string(raw))
	}
	if d := a.Destino(); !strings.Contains(d, "en este equipo") {
		t.Fatal(d)
	}
	// La pantalla pone el nombre y «en este equipo» en su idioma.
	if d := a.DestinoCon(func(Preajuste) string { return "Local model" }, "on this device"); d != "Local model (on this device)" {
		t.Fatal(d)
	}
}

func TestAnthropicConClave(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "sk-ant-prueba" || r.URL.Path != "/v1/messages" {
			t.Errorf("cabeceras o ruta: %s %s", r.Header.Get("x-api-key"), r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hola"}],"stop_reason":"end_turn"}`))
	}))
	defer srv.Close()
	a, _ := nuevo(t)
	if err := a.Guardar(Config{Proveedor: Anthropic, URL: "https://api.anthropic.com/v1"}, "sk-ant-prueba"); err != nil {
		t.Fatal(err)
	}
	// Las pruebas no salen a Internet: se cambia la dirección por el servidor de prueba
	// saltándose la validación de https (que exige Guardar).
	a.cfg.URL = srv.URL + "/v1"
	if got, err := a.Asistir(t.Context(), api.PeticionIA{Accion: "x", Instruccion: "y"}); err != nil || got != "hola" {
		t.Fatal(got, err)
	}
}

// El preajuste lleva la dirección de Gemini: `ia/proveedor` ya no la pone (solo la usa el
// dispositivo, no el servicio).
func TestElPreajusteDeGeminiLlevaSuDireccion(t *testing.T) {
	p, ok := PreajusteDe(Gemini)
	if !ok || p.URL != GeminiURL || !strings.HasPrefix(GeminiURL, "https://") || !strings.HasSuffix(GeminiURL, "/v1beta/openai") {
		t.Fatalf("preajuste de Gemini: %+v", p)
	}
}

func TestGeminiUsaSuExtremoCompatible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/openai/chat/completions" || r.Header.Get("Authorization") != "Bearer gk" {
			t.Errorf("ruta %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	a, _ := nuevo(t)
	if err := a.Guardar(Config{Proveedor: Gemini}, "gk"); err != nil {
		t.Fatal(err)
	}
	a.cfg.URL = srv.URL + "/v1beta/openai"
	if got, err := a.Asistir(t.Context(), api.PeticionIA{Accion: "x", Instruccion: "y"}); err != nil || got != "ok" {
		t.Fatal(got, err)
	}
}

func TestSoloHTTPSFueraDelEquipo(t *testing.T) {
	a, _ := nuevo(t)
	if err := a.Guardar(Config{Proveedor: Otro, URL: "http://ejemplo.eu/v1"}, "k"); err == nil {
		t.Fatal("aceptó http hacia fuera")
	}
}

func TestSistemaPasaPorSuPuente(t *testing.T) {
	a, _ := nuevo(t)
	if err := a.Guardar(Config{Proveedor: Sistema}, ""); err != nil {
		t.Fatal(err)
	}
	a.sistema = func(_ context.Context, _ Config, system, user string) (string, error) {
		if !strings.Contains(system, "asistente") || !strings.Contains(user, "resume") {
			t.Error("prompt")
		}
		return "resumen", nil
	}
	if got, err := a.Asistir(t.Context(), api.PeticionIA{Accion: "resumir", Instruccion: "resume"}); err != nil || got != "resumen" {
		t.Fatal(got, err)
	}
}

func mensajes(c map[string]any) string {
	raw, _ := json.Marshal(c["messages"])
	return string(raw)
}
