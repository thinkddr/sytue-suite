// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package consent_test

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
)

func cipher(t *testing.T) *keycrypto.Cipher {
	t.Helper()
	c, err := keycrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatalf("keycrypto.New: %v", err)
	}
	return c
}

func flow() consent.Flow {
	return consent.Flow{
		AuthorizeURL: "https://auth.atlassian.com/authorize",
		TokenURL:     "https://auth.atlassian.com/oauth/token",
		Scopes:       []string{"read:jira-user", "offline_access"},
		ClientID:     "cid-123",
	}
}

const redirectURI = "https://sytue.example/api/v1/oauth/callback"

var t0 = time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)

// TestBegin_urlDeAutorizacion: la ida lleva todo lo que el proveedor necesita, y el
// challenge es el SHA-256 del verificador — no el verificador, que es justo lo que hace
// que interceptar el redirect no sirva para canjear el código (RFC 7636).
func TestBegin_urlDeAutorizacion(t *testing.T) {
	c := cipher(t)
	st, err := consent.Begin(c, flow(), "acme", "ana", "inst-1", redirectURI, "", t0)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	u, err := url.Parse(st.AuthorizeURL)
	if err != nil {
		t.Fatalf("URL ilegible: %v", err)
	}
	q := u.Query()
	if q.Get("response_type") != "code" || q.Get("client_id") != "cid-123" {
		t.Fatalf("parámetros base incorrectos: %v", q)
	}
	if q.Get("redirect_uri") != redirectURI {
		t.Fatalf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("PKCE debe ir con S256, no %q", q.Get("code_challenge_method"))
	}
	if q.Get("scope") != "read:jira-user offline_access" {
		t.Fatalf("scopes = %q", q.Get("scope"))
	}
	if q.Get("code_challenge") == "" || q.Get("state") == "" {
		t.Fatal("faltan code_challenge o state")
	}

	// El state NO debe poder leerse desde fuera: lleva el code_verifier dentro.
	if raw, _ := base64.RawURLEncoding.DecodeString(q.Get("state")); len(raw) > 0 {
		for _, filtrado := range []string{"acme", "inst-1", "\"v\":"} {
			if strings.Contains(string(raw), filtrado) {
				t.Fatalf("el state expone %q en claro", filtrado)
			}
		}
	}

	// Y el challenge tiene que ser el hash del verificador que sobrevive en el state.
	rd := ida(t, c, st, "code-xyz")
	sum := sha256.Sum256([]byte(rd.Verifier))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != q.Get("code_challenge") {
		t.Fatal("code_challenge no es el S256 del code_verifier")
	}
}

// TestCallback_ idaYVuelta: el estado sobrevive el viaje y vuelve con la instancia
// correcta y el redirect_uri EXACTO que se mandó (el canje lo exige idéntico).
func TestCallback_idaYVuelta(t *testing.T) {
	c := cipher(t)
	st, err := consent.Begin(c, flow(), "acme", "ana", "inst-1", redirectURI, "", t0)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	rd := ida(t, c, st, "code-xyz")
	if rd.Tenant != "acme" || rd.Instance != "inst-1" || rd.Code != "code-xyz" {
		t.Fatalf("Redeem = %+v", rd)
	}
	if rd.RedirectURI != redirectURI {
		t.Fatalf("redirect_uri del canje = %q, debe repetir el de la ida", rd.RedirectURI)
	}
	form := rd.Form("cid-123", "secreto")
	if form.Get("grant_type") != "authorization_code" || form.Get("code_verifier") != rd.Verifier {
		t.Fatalf("cuerpo del canje incorrecto: %v", form)
	}
	if form.Get("client_secret") != "secreto" {
		t.Fatal("el client_secret debe ir cuando lo hay")
	}
	// Cliente público: sin secreto, y sin mandar el parámetro vacío.
	if _, hay := rd.Form("cid-123", "")["client_secret"]; hay {
		t.Fatal("un cliente público no debe mandar client_secret vacío")
	}
}

// TestCallback_rechaza es el corazón de la seguridad del flujo: un state que no
// autentica es un intento de CSRF (RFC 6749 §10.12) y uno caducado es un enlace viejo.
// Los dos se rechazan sin contarle al navegador en qué paso falló.
func TestCallback_rechaza(t *testing.T) {
	c := cipher(t)
	st, err := consent.Begin(c, flow(), "acme", "ana", "inst-1", redirectURI, "", t0)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	bueno := stateDe(t, st)

	t.Run("state fabricado", func(t *testing.T) {
		_, err := consent.Callback(c, base64.RawURLEncoding.EncodeToString([]byte("inventado")), "c", "", t0)
		if !errors.Is(err, consent.ErrState) {
			t.Fatalf("err = %v, quiero ErrState", err)
		}
	})
	t.Run("state de otra clave", func(t *testing.T) {
		otra, _ := keycrypto.New([]byte("otra-clave-de-32-bytes-exactos!!"))
		if _, err := consent.Callback(otra, bueno, "c", "", t0); !errors.Is(err, consent.ErrState) {
			t.Fatalf("un state sellado con otra clave debe rechazarse: %v", err)
		}
	})
	t.Run("caducado", func(t *testing.T) {
		tarde := t0.Add(11 * time.Minute)
		if _, err := consent.Callback(c, bueno, "c", "", tarde); !errors.Is(err, consent.ErrState) {
			t.Fatalf("a los 11 min debe estar caducado: %v", err)
		}
	})
	t.Run("dentro de la ventana", func(t *testing.T) {
		if _, err := consent.Callback(c, bueno, "c", "", t0.Add(9*time.Minute)); err != nil {
			t.Fatalf("a los 9 min debe seguir valiendo: %v", err)
		}
	})
	t.Run("sin código", func(t *testing.T) {
		if _, err := consent.Callback(c, bueno, "", "", t0); !errors.Is(err, consent.ErrState) {
			t.Fatalf("sin code debe fallar: %v", err)
		}
	})
	t.Run("el usuario dijo que no", func(t *testing.T) {
		if _, err := consent.Callback(c, bueno, "", "access_denied", t0); !errors.Is(err, consent.ErrDenied) {
			t.Fatalf("err = %v, quiero ErrDenied (no es un fallo, es una decisión)", err)
		}
	})
	t.Run("error del proveedor", func(t *testing.T) {
		if _, err := consent.Callback(c, bueno, "", "invalid_scope", t0); !errors.Is(err, consent.ErrProvider) {
			t.Fatalf("err = %v, quiero ErrProvider", err)
		}
	})
}

// TestBegin_exigeLoImprescindible: sin cifrador no se puede sellar el state, y sin
// client_id no hay con qué presentarse. En los dos casos es mejor no arrancar el flujo
// que mandar al admin a una pantalla que va a fallar.
func TestBegin_exigeLoImprescindible(t *testing.T) {
	if _, err := consent.Begin(nil, flow(), "acme", "ana", "i", redirectURI, "", t0); err == nil {
		t.Error("sin cifrador no debe arrancar")
	}
	f := flow()
	f.ClientID = ""
	if _, err := consent.Begin(cipher(t), f, "acme", "ana", "i", redirectURI, "", t0); err == nil {
		t.Error("sin client_id no debe arrancar")
	}
}

// TestBegin_noPisaLaQueryDelProveedor: hay authorize_url con parámetros propios
// (Salesforce y ServiceNow los usan); añadir los nuestros no debe borrarlos.
func TestBegin_noPisaLaQueryDelProveedor(t *testing.T) {
	f := flow()
	f.AuthorizeURL = "https://ejemplo.service-now.com/oauth_auth.do?instance=acme"
	st, err := consent.Begin(cipher(t), f, "acme", "ana", "i", redirectURI, "", t0)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	u, _ := url.Parse(st.AuthorizeURL)
	if u.Query().Get("instance") != "acme" {
		t.Fatalf("se ha perdido el parámetro propio del proveedor: %s", st.AuthorizeURL)
	}
	if u.Query().Get("state") == "" {
		t.Fatal("no se ha añadido el state")
	}
}

func stateDe(t *testing.T, st consent.Start) string {
	t.Helper()
	u, err := url.Parse(st.AuthorizeURL)
	if err != nil {
		t.Fatalf("URL ilegible: %v", err)
	}
	return u.Query().Get("state")
}

func ida(t *testing.T, c *keycrypto.Cipher, st consent.Start, code string) consent.Redeem {
	t.Helper()
	rd, err := consent.Callback(c, stateDe(t, st), code, "", t0)
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}
	return rd
}

func TestSlackUserConsentNeverRequestsBotScopes(t *testing.T) {
	f := flow()
	f.AuthorizeURL = "https://slack.com/oauth/v2/authorize"
	f.TokenURL = "https://slack.com/api/oauth.v2.access"
	f.ScopeParameter = "user_scope"
	f.Scopes = []string{"channels:read", "channels:history", "chat:write"}
	start, err := consent.Begin(cipher(t), f, "acme", "alice", "personal-account", redirectURI, "rooms", t0)
	if err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(start.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	if target.Query().Get("scope") != "" || target.Query().Get("user_scope") != "channels:read,channels:history,chat:write" || target.Query().Get("state") == "" || target.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("wrong user consent: %s", target)
	}
}

func TestOpenIDConsentIncludesUniqueNonce(t *testing.T) {
	cipher, err := keycrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 2 {
		start, err := consent.Begin(cipher, consent.Flow{AuthorizeURL: "https://accounts.example.eu/auth", ClientID: "fixture", Scopes: []string{"openid", "email"}}, "acme", "ana", "instance", "https://callback.example.eu", "external-mail", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		target, _ := url.Parse(start.AuthorizeURL)
		nonce := target.Query().Get("nonce")
		if len(nonce) < 32 || seen[nonce] {
			t.Fatal("OIDC nonce missing or reused")
		}
		seen[nonce] = true
	}
}

func TestRequestedServiceScopesRemainBoundToTheSealedConsent(t *testing.T) {
	cipher, err := keycrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	flow := consent.Flow{ClientID: "app", AuthorizeURL: "https://accounts.google.com/authorize", Scopes: []string{"mail", "contacts"}}
	start, err := consent.Begin(cipher, flow, "acme", "alice", "instance", "https://gateway.example/callback", "external-mail", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	flow.Scopes[0] = "admin"
	target, err := url.Parse(start.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	query := target.Query()
	query.Set("scope", "admin")
	target.RawQuery = query.Encode()
	result, err := consent.Callback(cipher, target.Query().Get("state"), "code", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.RequestedScopes, []string{"mail", "contacts"}) {
		t.Fatalf("changed request expanded sealed service selection: %v", result.RequestedScopes)
	}
}
