// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package federation

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// allowLoopback relaja la política SSRF de IP (httptest escucha en 127.0.0.1) y el
// requisito de https (httptest sirve por http), para tests herméticos. Restaura al
// terminar. NUNCA se usa en producción.
func allowLoopback(t *testing.T) {
	t.Helper()
	restoreIP := ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false })
	restoreScheme := SetInsecureForTest()
	t.Cleanup(func() { restoreIP(); restoreScheme() })
}

// fakeIDP es un IdP OIDC upstream simulado (discovery + jwks + token + userinfo).
type fakeIDP struct {
	srv      *httptest.Server
	priv     jwk.Key
	set      jwk.Set
	clientID string
	// idToken es el id_token que /token devolverá; el test lo fija por caso.
	idToken  string
	userinfo map[string]any
}

func genKeys(t *testing.T, kid string) (jwk.Key, jwk.Set) {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	priv, err := jwk.FromRaw(raw)
	if err != nil {
		t.Fatalf("jwk priv: %v", err)
	}
	_ = priv.Set(jwk.KeyIDKey, kid)
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256)
	pub, err := jwk.FromRaw(raw.Public())
	if err != nil {
		t.Fatalf("jwk pub: %v", err)
	}
	_ = pub.Set(jwk.KeyIDKey, kid)
	_ = pub.Set(jwk.AlgorithmKey, jwa.RS256)
	_ = pub.Set(jwk.KeyUsageKey, "sig")
	set := jwk.NewSet()
	if err := set.AddKey(pub); err != nil {
		t.Fatalf("add key: %v", err)
	}
	return priv, set
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	priv, set := genKeys(t, "kid-1")
	f := &fakeIDP{priv: priv, set: set, clientID: "client-123"}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		iss := f.srv.URL
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                           iss,
			"authorization_endpoint":           iss + "/authorize",
			"token_endpoint":                   iss + "/token",
			"userinfo_endpoint":                iss + "/userinfo",
			"jwks_uri":                         iss + "/jwks",
			"code_challenge_methods_supported": []string{"S256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.set)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id_token": f.idToken, "access_token": "at-1", "token_type": "Bearer",
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.userinfo)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// signID firma un id_token con la clave dada y las claims dadas.
func (f *fakeIDP) signID(t *testing.T, key jwk.Key, iss, aud, nonce string, extra map[string]any) string {
	t.Helper()
	b := jwt.NewBuilder().
		Issuer(iss).Subject("ext-sub-1").Audience([]string{aud}).
		IssuedAt(time.Now()).Expiration(time.Now().Add(time.Hour)).
		Claim("nonce", nonce)
	for k, v := range extra {
		b = b.Claim(k, v)
	}
	tok, err := b.Build()
	if err != nil {
		t.Fatalf("build id_token: %v", err)
	}
	raw, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, key))
	if err != nil {
		t.Fatalf("sign id_token: %v", err)
	}
	return string(raw)
}

func (f *fakeIDP) provider() domain.IdentityProvider {
	return domain.IdentityProvider{
		Issuer:                f.srv.URL,
		ClientID:              f.clientID,
		ClientSecret:          "shh",
		AuthorizationEndpoint: f.srv.URL + "/authorize",
		TokenEndpoint:         f.srv.URL + "/token",
		UserinfoEndpoint:      f.srv.URL + "/userinfo",
		JWKSURI:               f.srv.URL + "/jwks",
		PKCE:                  true,
		Enabled:               true,
	}
}

// --- SSRF ---

func TestDiscover_BlocksLoopbackByDefault(t *testing.T) {
	f := newFakeIDP(t)
	// Sin relajar la política SSRF: 127.0.0.1 (loopback) DEBE bloquearse.
	if _, err := New().Discover(context.Background(), f.srv.URL); err == nil {
		t.Fatal("Discover alcanzó un destino loopback (SSRF no bloqueó)")
	}
}

func TestDiscover_RejectsHTTPScheme(t *testing.T) {
	f := newFakeIDP(t)
	// Relaja SOLO la IP, no el esquema: un issuer http debe rechazarse (solo https).
	restore := ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false })
	defer restore()
	if _, err := New().Discover(context.Background(), f.srv.URL); err == nil {
		t.Fatal("Discover aceptó un issuer http (debe exigir https)")
	}
}

// --- Discovery ---

func TestDiscover_OK(t *testing.T) {
	allowLoopback(t)
	f := newFakeIDP(t)
	eps, err := New().Discover(context.Background(), f.srv.URL)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if eps.Token != f.srv.URL+"/token" || eps.JWKS != f.srv.URL+"/jwks" || !eps.PKCE {
		t.Fatalf("endpoints inesperados: %+v", eps)
	}
}

func TestDiscover_IssuerMismatch(t *testing.T) {
	allowLoopback(t)
	// Servidor cuyo documento anuncia OTRO issuer distinto de su URL.
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 "https://evil.example",
			"authorization_endpoint": "https://evil.example/a",
			"token_endpoint":         "https://evil.example/t",
			"jwks_uri":               "https://evil.example/jwks",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	if _, err := New().Discover(context.Background(), srv.URL); err == nil {
		t.Fatal("Discover aceptó un issuer del documento que no coincide")
	}
}

// --- Authenticate / validación del id_token ---

func TestAuthenticate_ValidIDToken(t *testing.T) {
	allowLoopback(t)
	f := newFakeIDP(t)
	p := f.provider()
	f.idToken = f.signID(t, f.priv, p.Issuer, p.ClientID, "nonce-xyz", map[string]any{
		"email": "ada@acme.example", "email_verified": true, "name": "Ada Lovelace",
		"groups": []string{"eng", "leads"},
	})
	claims, err := New().Authenticate(context.Background(), p, "code-1", "https://id.test/cb", "verifier", "nonce-xyz")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if claims.Subject != "ext-sub-1" || claims.Email != "ada@acme.example" || !claims.EmailVerified {
		t.Fatalf("claims inesperados: %+v", claims)
	}
	if claims.Name != "Ada Lovelace" || len(claims.Groups) != 2 {
		t.Fatalf("claims mapeados mal: %+v", claims)
	}
}

func TestAuthenticate_RejectsBadSignature(t *testing.T) {
	allowLoopback(t)
	f := newFakeIDP(t)
	p := f.provider()
	otherPriv, _ := genKeys(t, "kid-1") // mismo kid, otra clave ⇒ firma no valida
	f.idToken = f.signID(t, otherPriv, p.Issuer, p.ClientID, "n", map[string]any{
		"email": "a@b.c", "email_verified": true,
	})
	if _, err := New().Authenticate(context.Background(), p, "c", "https://id.test/cb", "v", "n"); err == nil {
		t.Fatal("Authenticate aceptó una firma inválida")
	}
}

func TestAuthenticate_RejectsWrongIssuer(t *testing.T) {
	allowLoopback(t)
	f := newFakeIDP(t)
	p := f.provider()
	f.idToken = f.signID(t, f.priv, "https://evil.example", p.ClientID, "n", map[string]any{
		"email": "a@b.c", "email_verified": true,
	})
	if _, err := New().Authenticate(context.Background(), p, "c", "https://id.test/cb", "v", "n"); err == nil {
		t.Fatal("Authenticate aceptó un issuer incorrecto en el id_token")
	}
}

func TestAuthenticate_RejectsWrongAudience(t *testing.T) {
	allowLoopback(t)
	f := newFakeIDP(t)
	p := f.provider()
	f.idToken = f.signID(t, f.priv, p.Issuer, "otro-client", "n", map[string]any{
		"email": "a@b.c", "email_verified": true,
	})
	if _, err := New().Authenticate(context.Background(), p, "c", "https://id.test/cb", "v", "n"); err == nil {
		t.Fatal("Authenticate aceptó una audiencia incorrecta")
	}
}

func TestAuthenticate_RejectsBadNonce(t *testing.T) {
	allowLoopback(t)
	f := newFakeIDP(t)
	p := f.provider()
	f.idToken = f.signID(t, f.priv, p.Issuer, p.ClientID, "nonce-del-token", map[string]any{
		"email": "a@b.c", "email_verified": true,
	})
	// Esperamos otro nonce distinto del que trae el token ⇒ rechazo (anti-replay).
	if _, err := New().Authenticate(context.Background(), p, "c", "https://id.test/cb", "v", "nonce-esperado"); err == nil {
		t.Fatal("Authenticate aceptó un nonce que no coincide")
	}
}

func TestAuthenticate_UserinfoFallback(t *testing.T) {
	allowLoopback(t)
	f := newFakeIDP(t)
	p := f.provider()
	// id_token SIN email; userinfo lo aporta (con el MISMO sub).
	f.idToken = f.signID(t, f.priv, p.Issuer, p.ClientID, "n", nil)
	f.userinfo = map[string]any{"sub": "ext-sub-1", "email": "ada@acme.example", "email_verified": true}
	claims, err := New().Authenticate(context.Background(), p, "c", "https://id.test/cb", "v", "n")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if claims.Email != "ada@acme.example" || !claims.EmailVerified {
		t.Fatalf("no completó desde userinfo: %+v", claims)
	}
}

// --- helpers de dominio (puros) ---

func TestDomainAllowed(t *testing.T) {
	p := domain.IdentityProvider{AllowedEmailDomains: []string{"acme.example"}}
	if !p.DomainAllowed("x@ACME.example") {
		t.Fatal("dominio permitido (case-insensitive) rechazado")
	}
	if p.DomainAllowed("x@evil.example") {
		t.Fatal("dominio no permitido aceptado")
	}
	// Lista vacía ⇒ sin restricción.
	if !(domain.IdentityProvider{}).DomainAllowed("x@cualquiera.example") {
		t.Fatal("lista vacía debe permitir")
	}
}

func TestClaimName(t *testing.T) {
	p := domain.IdentityProvider{Mapping: map[string]string{"email": "mail"}}
	if p.ClaimName("email") != "mail" {
		t.Fatal("mapeo de email no aplicado")
	}
	if p.ClaimName("name") != "name" {
		t.Fatal("default de name incorrecto")
	}
}

func TestScheme_notLeaked(t *testing.T) {
	// El error de esquema debe ser ErrScheme (para el mapeo a 400 en admin).
	if !strings.Contains(ErrScheme.Error(), "https") {
		t.Fatal("ErrScheme no menciona https")
	}
}
