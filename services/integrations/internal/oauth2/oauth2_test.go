// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package oauth2_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	sytueoauth2 "git.sytue.eu/sytue/one/services/integrations/internal/oauth2"
)

// tokenServer devuelve un token endpoint falso que cuenta cuántas veces lo llaman.
func tokenServer(t *testing.T, expiresIn int, body string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("el canje debe ir en un formulario: %v", err)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if body != "" {
			_, _ = w.Write([]byte(body))
			return
		}
		_, _ = fmt.Fprintf(w, `{"access_token":"tok-%d","expires_in":%d}`, calls.Load(), expiresIn)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// rt devuelve un Runtime con el cliente por defecto: estos tests ejercitan la caché, no
// la cadena de egress (que tiene los suyos en internal/policy).
func rt() connector.Runtime { return connector.Runtime{HTTP: http.DefaultClient} }

func req(tokenURL, key string) sytueoauth2.Request {
	return sytueoauth2.Request{
		Key:      key,
		TokenURL: tokenURL,
		Form:     url.Values{"grant_type": {"client_credentials"}},
		Provider: "prueba",
	}
}

// TestCache_reusesLiveToken: el segundo uso no vuelve a pedir token. Es el defecto que
// motivó el paquete — Microsoft Graph minteaba uno en CADA consulta.
func TestCache_reusesLiveToken(t *testing.T) {
	srv, calls := tokenServer(t, 3600, "")
	c := sytueoauth2.NewCache()

	first, err := c.Token(context.Background(), rt(), req(srv.URL, "k"))
	if err != nil {
		t.Fatalf("primer token: %v", err)
	}
	second, err := c.Token(context.Background(), rt(), req(srv.URL, "k"))
	if err != nil {
		t.Fatalf("segundo token: %v", err)
	}
	if first != second {
		t.Errorf("el segundo token = %q, quiero el mismo que el primero (%q)", second, first)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("llamadas al token endpoint = %d, quiero 1: la caché no está sirviendo", got)
	}
}

// TestCache_separatesKeys: dos claves distintas NUNCA comparten token. Compartirlo sería
// entregarle a una organización el token de otra.
func TestCache_separatesKeys(t *testing.T) {
	srv, calls := tokenServer(t, 3600, "")
	c := sytueoauth2.NewCache()

	a, err := c.Token(context.Background(), rt(), req(srv.URL, "org-a"))
	if err != nil {
		t.Fatalf("token de org-a: %v", err)
	}
	b, err := c.Token(context.Background(), rt(), req(srv.URL, "org-b"))
	if err != nil {
		t.Fatalf("token de org-b: %v", err)
	}
	if a == b {
		t.Error("dos claves distintas comparten token")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("llamadas = %d, quiero 2 (una por clave)", got)
	}
}

// TestCache_refetchesWhenExpired: un expires_in corto obliga a pedir de nuevo. El margen
// de refresco hace que un token de 1 s se considere agotado enseguida.
func TestCache_refetchesWhenExpired(t *testing.T) {
	srv, calls := tokenServer(t, 1, "")
	c := sytueoauth2.NewCache()

	if _, err := c.Token(context.Background(), rt(), req(srv.URL, "k")); err != nil {
		t.Fatalf("primer token: %v", err)
	}
	time.Sleep(600 * time.Millisecond)
	if _, err := c.Token(context.Background(), rt(), req(srv.URL, "k")); err != nil {
		t.Fatalf("segundo token: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("llamadas = %d, quiero 2: un token caducado debe renovarse", got)
	}
}

// TestCache_forgetDropsToken: Forget descarta el token, para el caso de un 401 por
// revocación antes de su caducidad.
func TestCache_forgetDropsToken(t *testing.T) {
	srv, calls := tokenServer(t, 3600, "")
	c := sytueoauth2.NewCache()

	if _, err := c.Token(context.Background(), rt(), req(srv.URL, "k")); err != nil {
		t.Fatalf("primer token: %v", err)
	}
	c.Forget("k")
	if _, err := c.Token(context.Background(), rt(), req(srv.URL, "k")); err != nil {
		t.Fatalf("segundo token: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("llamadas = %d, quiero 2: Forget debe invalidar el token cacheado", got)
	}
}

// TestCache_surfacesProviderError: el motivo REAL del proveedor llega al mensaje. Un
// «401» a secas no dice si caducó el secreto, si falta consentimiento o si el client no
// existe, y esa diferencia es la que arregla el administrador.
func TestCache_surfacesProviderError(t *testing.T) {
	srv, _ := tokenServer(t, 0, `{"error":"invalid_client","error_description":"client secret caducado"}`)
	c := sytueoauth2.NewCache()

	_, err := c.Token(context.Background(), rt(), req(srv.URL, "k"))
	if err == nil {
		t.Fatal("un error del token endpoint debe propagarse")
	}
	for _, want := range []string{"prueba", "invalid_client", "client secret caducado"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("el error %q no menciona %q", err, want)
		}
	}
}

// TestCache_rejectsEmptyKey: sin clave no hay caché posible, y compartir una clave vacía
// entre instancias mezclaría tokens de organizaciones distintas.
func TestCache_rejectsEmptyKey(t *testing.T) {
	srv, calls := tokenServer(t, 3600, "")
	c := sytueoauth2.NewCache()

	if _, err := c.Token(context.Background(), rt(), req(srv.URL, "")); err == nil {
		t.Error("una petición sin clave debe rechazarse")
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("llamadas = %d, quiero 0: ni siquiera debe salir a la red", got)
	}
}
