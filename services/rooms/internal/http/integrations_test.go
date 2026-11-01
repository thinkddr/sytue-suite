// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// TestNewRouterMountsIntegrations verifica que el árbol de rutas se monta SIN conflicto
// de chi al añadir /hooks/{token} y el mount /api/bot junto al mount /api/v1 (los mounts
// hermanos bajo /api y las rutas top-level conviven). Deps vacío: las rutas se registran
// sin invocar dependencias (los method values sobre punteros nil no se llaman aquí).
func TestNewRouterMountsIntegrations(t *testing.T) {
	r := NewRouter(Deps{})
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/healthz", http.StatusOK},
		{"/definitely/not/a/route", http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, http.NoBody))
		if rec.Code != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.path, rec.Code, tc.want)
		}
	}
}

// withToken construye una request con el parámetro de ruta {token} fijado (como si
// chi ya hubiera enrutado), para probar el endpoint público sin montar el router.
func withToken(r *http.Request, token string) *http.Request {
	rc := chi.NewRouteContext()
	if token != "" {
		rc.URLParams.Add("token", token)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

// TestBotAuthRejectsMissingOrNonBotToken cubre «no-token 401»: sin Bearer, o con un
// Bearer que no es un token de bot (p. ej. un JWT), el middleware corta con 401 y NO
// invoca al handler protegido (ni toca el servicio: svc puede ser nil aquí).
func TestBotAuthRejectsMissingOrNonBotToken(t *testing.T) {
	ih := &intHandlers{svc: nil}
	nextCalled := false
	guarded := ih.botAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true }))

	cases := []struct {
		name string
		auth string
	}{
		{"sin cabecera", ""},
		{"bearer vacío", "Bearer "},
		{"jwt (no bot_)", "Bearer eyJhbGciOiJSUzI1NiJ9.payload.sig"},
		{"prefijo ajeno", "Bearer test_token_abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/bot/channels/x/messages", http.NoBody)
			if c.auth != "" {
				r.Header.Set("Authorization", c.auth)
			}
			rec := httptest.NewRecorder()
			nextCalled = false
			guarded.ServeHTTP(rec, r)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("quiero 401, got %d", rec.Code)
			}
			if nextCalled {
				t.Fatal("el handler protegido NO debe ejecutarse sin un token de bot válido")
			}
		})
	}
}

// TestPostIncomingEmptyToken cubre el rechazo antes de tocar el servicio: sin token
// en la ruta, 404 (no se filtra si el token existiría).
func TestPostIncomingEmptyToken(t *testing.T) {
	ih := &intHandlers{svc: nil}
	r := withToken(httptest.NewRequest(http.MethodPost, "/hooks/", http.NoBody), "")
	rec := httptest.NewRecorder()
	ih.PostIncoming(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("token vacío debe dar 404, got %d", rec.Code)
	}
}

func TestHookRateKeyHashesToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/hooks/supersecret", http.NoBody)
	r = withToken(r, "supersecret")
	r.RemoteAddr = "203.0.113.9:5555"
	key := hookRateKey(r)
	if key == "" {
		t.Fatal("clave vacía")
	}
	// El token en CLARO nunca debe aparecer en la clave (va a la tabla de límites).
	if strings.Contains(key, "supersecret") {
		t.Fatalf("la clave no debe contener el token en claro: %q", key)
	}
	if !strings.Contains(key, "hook:") {
		t.Fatalf("la clave debe llevar el prefijo hook:, got %q", key)
	}
}

func TestClientIPPrefersForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	r.RemoteAddr = "10.0.0.1:1"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")
	if got := clientIP(r); got != "198.51.100.7" {
		t.Fatalf("clientIP debe tomar el primero de XFF, got %q", got)
	}
}

// El secreto de un entrante lleva el dominio del correo a canal SOLO si está encendido:
// es lo que decide si la web y las apps enseñan la dirección de correo.
func TestElSecretoDelEntranteLlevaElDominioDelCorreo(t *testing.T) {
	hook := domain.IncomingWebhook{Name: "CI"}
	apagado := (&intHandlers{publicURL: "https://rooms.sytue.eu"}).secretoEntrante(hook, "tk")
	if _, hay := apagado["channel_mail_domain"]; hay || apagado["url"] != "https://rooms.sytue.eu/hooks/tk" {
		t.Errorf("apagado: %v", apagado)
	}
	encendido := (&intHandlers{publicURL: "https://rooms.sytue.eu", channelMailDomain: "rooms.sytue.eu"}).secretoEntrante(hook, "tk")
	if encendido["channel_mail_domain"] != "rooms.sytue.eu" || encendido["token"] != "tk" {
		t.Errorf("encendido: %v", encendido)
	}
}
