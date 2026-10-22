// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package asistente

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPedir(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p Peticion
		_ = json.NewDecoder(r.Body).Decode(&p)
		switch {
		case r.URL.Path != "/api/v1/assist" || r.Header.Get("Authorization") != "Bearer tok":
			w.WriteHeader(http.StatusUnauthorized)
		case p.Modo == "calidad":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			_ = json.NewEncoder(w).Encode(map[string]string{"text": "```json\n{\"a\":1}\n```"})
		}
	}))
	defer srv.Close()
	c := Nuevo(srv.URL+"/", 5*time.Second)

	texto, err := c.Pedir(context.Background(), "tok", Peticion{Instruction: "x"})
	if err != nil || JSON(texto) != `{"a":1}` {
		t.Fatalf("texto=%q err=%v", texto, err)
	}
	var estado ErrEstado
	if _, err := c.Pedir(context.Background(), "tok", Peticion{Modo: "calidad"}); !errors.As(err, &estado) || estado != http.StatusTooManyRequests {
		t.Fatalf("un 429 debe llegar como ErrEstado(429): %v", err)
	}
	if _, err := Nuevo("", time.Second).Pedir(context.Background(), "tok", Peticion{}); !errors.Is(err, ErrSinServicio) {
		t.Fatalf("sin base: %v", err)
	}
	if JSON("no hay objeto") != "" {
		t.Fatal("sin llaves no hay JSON")
	}
}

func TestPedirPages(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/assist" {
			t.Errorf("Path = %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret-123" {
			t.Errorf("Authorization = %s", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Sytue-Tenant") != "org-test" || r.Header.Get("X-Sytue-Sub") != "usr-test" {
			t.Errorf("Identidad afirmada incorrecta: %s / %s", r.Header.Get("X-Sytue-Tenant"), r.Header.Get("X-Sytue-Sub"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var p PeticionPages
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Errorf("Decode: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		_ = json.NewEncoder(w).Encode(RespuestaPages{
			Text:          "respuesta generada",
			Modelo:        "qwen3.8-27b",
			Degradado:     false,
			TokensEntrada: 50,
			TokensSalida:  100,
		})
	}))
	defer srv.Close()

	c := Nuevo(srv.URL, 5*time.Second)
	res, err := c.PedirPages(context.Background(), "secret-123", Identidad{Tenant: "org-test", Sub: "usr-test"}, PeticionPages{
		Action:      "design",
		Instruction: "Crear cabecera",
		Sistema:     "pages",
	})
	if err != nil {
		t.Fatalf("PedirPages: %v", err)
	}
	if res.Text != "respuesta generada" || res.TokensEntrada != 50 || res.TokensSalida != 100 {
		t.Errorf("res = %+v", res)
	}

	// Cliente sin servicio configurado
	cInactivo := Nuevo("", 5*time.Second)
	if _, err := cInactivo.PedirPages(context.Background(), "s", Identidad{}, PeticionPages{}); !errors.Is(err, ErrSinServicio) {
		t.Fatalf("esperaba ErrSinServicio, got %v", err)
	}
}

func TestListarModelos(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"modelos": []string{"gpt-4o", "gpt-4o-mini"}})
	}))
	defer srv.Close()

	c := Nuevo(srv.URL, 5*time.Second)
	modelos, err := c.ListarModelos(context.Background(), "secret-123", Identidad{Tenant: "org-test", Sub: "usr-test"}, BYOK{
		Proveedor: "openai",
		Clave:     "sk-test",
	})
	if err != nil {
		t.Fatalf("ListarModelos: %v", err)
	}
	if len(modelos) != 2 || modelos[0] != "gpt-4o" {
		t.Errorf("modelos = %+v", modelos)
	}
}
