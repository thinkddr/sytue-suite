// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package authn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClaimsRolesAndScopes(t *testing.T) {
	c := Claims{Roles: []string{"admin", "member"}, Scopes: []string{"drive.read"}}
	if !c.HasRole("admin") || c.HasRole("owner") {
		t.Fatal("HasRole incorrecto")
	}
	if !c.HasAnyRole("billing", "member") || c.HasAnyRole("billing") {
		t.Fatal("HasAnyRole incorrecto")
	}
	if !c.HasScope("drive.read") || c.HasScope("drive.write") {
		t.Fatal("HasScope incorrecto")
	}
}

func TestClaimsTenant(t *testing.T) {
	if _, ok := (Claims{Tenant: "acme"}).TenantOrEmpty(); !ok {
		t.Fatal("tenant válido rechazado")
	}
	if _, ok := (Claims{Tenant: "BAD SLUG"}).TenantOrEmpty(); ok {
		t.Fatal("tenant inválido aceptado")
	}
	if _, ok := (Claims{}).TenantOrEmpty(); ok {
		t.Fatal("tenant vacío aceptado")
	}
}

func TestContextRoundTrip(t *testing.T) {
	ctx := Into(context.Background(), Claims{Subject: "user-1"})
	got, ok := FromContext(ctx)
	if !ok || got.Subject != "user-1" {
		t.Fatalf("FromContext = %+v, %v", got, ok)
	}
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("contexto vacío no debe traer claims")
	}
}

// TestRateKey: la clave de rate-limit sale de la IDENTIDAD del token. Es lo que
// separa «este usuario se ha desbocado» de «todos los clientes comparten un
// cubo», que es lo que pasa al usar la IP detrás del ingress: allí todas las
// peticiones llegan con la misma dirección y el primero que abusa deja fuera al
// resto. Dos usuarios de la MISMA organización tampoco pueden compartir cubo.
func TestRateKey(t *testing.T) {
	req := func(ctx context.Context, xff string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/x", http.NoBody)
		r.RemoteAddr = "10.0.0.1:5555" // IP del ingress: la misma para todos
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		if ctx != nil {
			r = r.WithContext(ctx)
		}
		return r
	}
	ada := Into(context.Background(), Claims{Subject: "u-ada", Tenant: "acme"})
	bob := Into(context.Background(), Claims{Subject: "u-bob", Tenant: "acme"})
	kAda, kBob := RateKey(req(ada, "")), RateKey(req(bob, ""))
	if kAda == kBob {
		t.Fatalf("dos usuarios comparten cubo: %q", kAda)
	}
	// La misma persona en dos organizaciones tampoco es el mismo sujeto de cuota.
	otra := Into(context.Background(), Claims{Subject: "u-ada", Tenant: "otra"})
	if RateKey(req(otra, "")) == kAda {
		t.Fatal("el tenant no entra en la clave")
	}
	// La IP NO manda cuando hay token: si mandara, ada y bob (misma IP de
	// ingress) caerían en el mismo cubo.
	if RateKey(req(ada, "203.0.113.9")) != kAda {
		t.Fatal("la IP no debe alterar la clave de un token")
	}
	// Sin claims (ruta pública o token inválido) queda la dirección de QUIEN
	// CONECTA, y `X-Forwarded-For` NO puede cambiarla. Esa cabecera la escribe el
	// cliente y cada salto le añade por la derecha: su primera entrada —la que se
	// leía aquí— es justo la que el atacante controla, así que una cabecera nueva
	// por intento estrenaba cubo y el tope no topaba nada. Este test es el que lo
	// impide, y se vio en rojo contra la versión anterior.
	sinToken := RateKey(req(nil, ""))
	for _, xff := range []string{"203.0.113.9", "203.0.113.9, 10.0.0.1", "no-es-una-ip", "  ,  "} {
		if got := RateKey(req(nil, xff)); got != sinToken {
			t.Fatalf("X-Forwarded-For %q cambia la clave: %q (quiero %q)", xff, got, sinToken)
		}
	}
	if sinToken != "peer:10.0.0.1:5555" {
		t.Fatalf("sin claims la clave es %q", sinToken)
	}
}

func TestToStringSlice(t *testing.T) {
	if got := toStringSlice("a b c"); len(got) != 3 || got[2] != "c" {
		t.Fatalf("string scope = %v", got)
	}
	if got := toStringSlice([]any{"x", 1, "y"}); len(got) != 2 || got[1] != "y" {
		t.Fatalf("[]any = %v", got)
	}
	if got := toStringSlice([]string{"z"}); len(got) != 1 {
		t.Fatalf("[]string = %v", got)
	}
	if toStringSlice(nil) != nil {
		t.Fatal("nil debe dar nil")
	}
}

func TestNewVerifierRequiresIssuer(t *testing.T) {
	if _, err := NewVerifier(context.Background(), VerifierConfig{}); err == nil {
		t.Fatal("esperaba error por issuer ausente")
	}
}

func TestNewVerifierWaitsForInitialJWKS(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) < 3 {
			http.Error(w, "identity starting", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer srv.Close()

	_, err := NewVerifier(context.Background(), VerifierConfig{
		Issuer:                "https://id.sytue.eu",
		JWKSURL:               srv.URL,
		InitialRefreshTimeout: time.Second,
		InitialRetryInterval:  5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewVerifier no esperó al IdP: %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("intentos JWKS = %d, quiero 3", got)
	}
}

func TestNewVerifierBoundsInitialJWKSWait(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, "identity unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	started := time.Now()
	_, err := NewVerifier(context.Background(), VerifierConfig{
		Issuer:                "https://id.sytue.eu",
		JWKSURL:               srv.URL,
		InitialRefreshTimeout: 60 * time.Millisecond,
		InitialRetryInterval:  5 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("un IdP permanentemente caído debe impedir el arranque")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("el timeout JWKS no se respetó: %s", elapsed)
	}
	if attempts.Load() < 2 {
		t.Fatalf("no hubo reintentos JWKS: %d", attempts.Load())
	}
}
