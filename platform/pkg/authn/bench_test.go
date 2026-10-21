// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package authn

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// testVerifier monta un Verifier real contra un JWKS servido en memoria y devuelve
// la clave privada del set, para firmar tokens que ese Verifier acepte.
func testVerifier(tb testing.TB, aud string) (*Verifier, jwk.Key) {
	tb.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tb.Fatalf("generar RSA: %v", err)
	}
	priv, err := jwk.FromRaw(raw)
	if err != nil {
		tb.Fatalf("jwk privada: %v", err)
	}
	_ = priv.Set(jwk.KeyIDKey, "bench-kid")
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256)

	pub, err := jwk.FromRaw(raw.Public())
	if err != nil {
		tb.Fatalf("jwk pública: %v", err)
	}
	_ = pub.Set(jwk.KeyIDKey, "bench-kid")
	_ = pub.Set(jwk.AlgorithmKey, jwa.RS256)
	_ = pub.Set(jwk.KeyUsageKey, "sig")
	set := jwk.NewSet()
	if err := set.AddKey(pub); err != nil {
		tb.Fatalf("añadir clave: %v", err)
	}
	body, err := json.Marshal(set)
	if err != nil {
		tb.Fatalf("serializar JWKS: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	tb.Cleanup(srv.Close)

	v, err := NewVerifier(context.Background(), VerifierConfig{
		Issuer:   testIssuer,
		JWKSURL:  srv.URL,
		Audience: aud,
	})
	if err != nil {
		tb.Fatalf("NewVerifier: %v", err)
	}
	return v, priv
}

const testIssuer = "https://id.sytue.eu"

// benchVerifier devuelve el Verifier de testVerifier y un token firmado con la
// forma real de la suite. Es el montaje mínimo para medir Verify: el resto del
// coste (red, base) no participa.
func benchVerifier(tb testing.TB, aud string) (*Verifier, string) {
	tb.Helper()
	v, priv := testVerifier(tb, aud)
	// Token con la forma real de la suite: sub/tenant/roles/scope/email/mailbox.
	tok, err := jwt.NewBuilder().
		Issuer(testIssuer).
		Subject("usuario-de-prueba").
		Audience([]string{aud}).
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour)).
		Claim("email", "ada@sytue.eu").
		Claim("mailbox", "ada@acme.sytue.eu").
		Claim("name", "Ada Lovelace").
		Claim("tenant", "acme").
		Claim("roles", []string{"member", "admin"}).
		Claim("scope", "drive.read drive.write mail.read").
		Build()
	if err != nil {
		tb.Fatalf("build token: %v", err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, priv))
	if err != nil {
		tb.Fatalf("firmar: %v", err)
	}
	return v, string(signed)
}

// BenchmarkVerify mide el coste de validar un JWT: es el trabajo que ocurre en
// TODA petición autenticada de la suite, así que su ns/op es el suelo de latencia
// y sus asignaciones el suelo de presión sobre el GC de cada réplica.
func BenchmarkVerify(b *testing.B) {
	v, token := benchVerifier(b, "sytue-one")
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := v.Verify(ctx, token); err != nil {
			b.Fatalf("Verify: %v", err)
		}
	}
}

// BenchmarkVerifyParalelo mide lo mismo con todos los núcleos ocupados, que es
// el régimen real de una réplica con carga: si la verificación serializa por un
// candado, aquí se ve y en el benchmark de un solo hilo no.
func BenchmarkVerifyParalelo(b *testing.B) {
	v, token := benchVerifier(b, "sytue-one")
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		for pb.Next() {
			if _, err := v.Verify(ctx, token); err != nil {
				b.Fatalf("Verify: %v", err)
			}
		}
	})
}

// BenchmarkCheckHeader aísla la inspección previa de la cabecera JWS. Se mide
// aparte porque parsea el token COMPLETO (payload y firma incluidos) solo para
// leer alg/kid, y jwt.ParseString vuelve a parsearlo entero después.
func BenchmarkCheckHeader(b *testing.B) {
	v, token := benchVerifier(b, "sytue-one")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := v.checkHeader(token); err != nil {
			b.Fatalf("checkHeader: %v", err)
		}
	}
}

// Una app de terceros con scopes de Google entra SOLO por RequireGoogleAPIs, y solo
// con el scope de la operación; por RequireAuth sigue fuera. Un token de la suite
// pasa por los dos sin scopes de Google.
func TestRequireGoogleAPIs(t *testing.T) {
	v, priv := testVerifier(t, "sytue-one")
	firmar := func(aud []string, scope string) string {
		tok, err := jwt.NewBuilder().Issuer(testIssuer).Subject("u-1").Audience(aud).
			IssuedAt(time.Now()).Expiration(time.Now().Add(time.Hour)).
			Claim("tenant", "acme").Claim("scope", scope).Build()
		if err != nil {
			t.Fatal(err)
		}
		signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, priv))
		if err != nil {
			t.Fatal(err)
		}
		return string(signed)
	}
	suite := firmar([]string{"sytue-one"}, "openid")
	const g = "https://www.googleapis.com/auth/"
	lectura := firmar([]string{"https://id.sytue.eu", "app-crm", GoogleAPIsAudience}, g+"calendar.readonly")
	escritura := firmar([]string{"https://id.sytue.eu", "app-crm", GoogleAPIsAudience}, g+"calendar")
	sinGoogle := firmar([]string{"https://id.sytue.eu", "app-crm"}, g+"calendar")

	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	google := v.RequireGoogleAPIs([]string{g + "calendar.readonly"}, []string{g + "calendar"})(ok)
	nativo := v.RequireAuth(ok)
	pedir := func(h http.Handler, method, token string) int {
		r := httptest.NewRequest(method, "/compat/google/calendar/v3/x", http.NoBody)
		r.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	for _, c := range []struct {
		nombre string
		h      http.Handler
		method string
		token  string
		quiero int
	}{
		{"suite por google", google, http.MethodPost, suite, http.StatusOK},
		{"suite por nativo", nativo, http.MethodGet, suite, http.StatusOK},
		{"lectura lee", google, http.MethodGet, lectura, http.StatusOK},
		{"lectura no escribe", google, http.MethodPost, lectura, http.StatusForbidden},
		{"escritura lee", google, http.MethodGet, escritura, http.StatusOK},
		{"escritura escribe", google, http.MethodDelete, escritura, http.StatusOK},
		{"app por nativo", nativo, http.MethodGet, escritura, http.StatusUnauthorized},
		{"sin audiencia de google", google, http.MethodGet, sinGoogle, http.StatusUnauthorized},
	} {
		if got := pedir(c.h, c.method, c.token); got != c.quiero {
			t.Errorf("%s: %d, quiero %d", c.nombre, got, c.quiero)
		}
	}
}
