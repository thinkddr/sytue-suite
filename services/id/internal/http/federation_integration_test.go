// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del login federado (Sytue ID como broker/RP OIDC) contra un
// PostgreSQL real (pgtest, build-tag `integration`) y un IdP upstream SIMULADO
// (httptest: discovery + jwks + token + userinfo con firma RS256 real). Cubren:
//   - client_secret CIFRADO en reposo (KEK/AES-GCM) y aislamiento por org del CRUD;
//   - callback con id_token válido ⇒ sesión SSO + aprovisionamiento JIT;
//   - segunda vez ⇒ casa por federated_identities (mismo usuario, sin alta nueva);
//   - casa por email VERIFICADO de un miembro existente de la org;
//   - email NO verificado ⇒ NO liga (anti account-takeover);
//   - usuario de OTRA org con el mismo email ⇒ rechazo (aislamiento tenant).
package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/federation"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// fedHarness es un Server con store cifrado (KEK real) para ejercer la federación.
type fedHarness struct {
	srv *Server
	st  *store.Store
	ctx context.Context
	mux http.Handler
}

func newFedHarness(t *testing.T) *fedHarness {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	key := bytes.Repeat([]byte{0x2a}, keycrypto.KeyLen) // KEK de test (32B)
	cipher, err := keycrypto.New(key)
	if err != nil {
		t.Fatalf("keycrypto: %v", err)
	}
	st := store.NewWithCipher(h.Pool, cipher)
	ctx := context.Background()
	km := oidc.NewKeyManager()
	if err := km.Bootstrap(ctx, st); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	iss := oidc.NewIssuer(km, itIssuer, itAud, 15*time.Minute, time.Hour)
	cfg := Config{
		Issuer: itIssuer, ResourceAudience: itAud, RPDisplayName: "Sytue",
		SessionTTL: 12 * time.Hour, CookieSecure: false,
	}
	srv := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st, km, iss)
	return &fedHarness{srv: srv, st: st, ctx: ctx, mux: srv.Router()}
}

// relaxSSRF permite loopback para alcanzar el IdP httptest (127.0.0.1) en este
// test; restaura la política de producción al terminar.
func relaxSSRF(t *testing.T) {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
}

// --- IdP upstream simulado (firma RS256 real) ---

type fedIdP struct {
	srv      *httptest.Server
	priv     jwk.Key
	set      jwk.Set
	clientID string
	idToken  string
	userinfo map[string]any
}

func newFedIdP(t *testing.T) *fedIdP {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	priv, _ := jwk.FromRaw(raw)
	_ = priv.Set(jwk.KeyIDKey, "kid-1")
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256)
	pub, _ := jwk.FromRaw(raw.Public())
	_ = pub.Set(jwk.KeyIDKey, "kid-1")
	_ = pub.Set(jwk.AlgorithmKey, jwa.RS256)
	_ = pub.Set(jwk.KeyUsageKey, "sig")
	set := jwk.NewSet()
	_ = set.AddKey(pub)

	f := &fedIdP{priv: priv, set: set, clientID: "cli-1"}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id_token": f.idToken, "access_token": "at", "token_type": "Bearer",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.set)
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.userinfo)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fedIdP) sign(t *testing.T, aud, nonce, email string, verified bool) string {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Issuer(f.srv.URL).Subject("ext-sub-1").Audience([]string{aud}).
		IssuedAt(time.Now()).Expiration(time.Now().Add(time.Hour)).
		Claim("nonce", nonce).Claim("email", email).Claim("email_verified", verified).
		Claim("name", "Ada Lovelace").Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, f.priv))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return string(raw)
}

// seedProvider crea una org y un provider apuntando al IdP simulado (endpoints ya
// resueltos), habilitado. Devuelve (orgID, provider).
func (h *fedHarness) seedProvider(t *testing.T, f *fedIdP, slug string, domains []string) (string, domain.IdentityProvider) {
	t.Helper()
	org, err := h.st.CreateOrg(h.ctx, nil, domain.Organization{Slug: slug, Name: strings.ToUpper(slug)})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	p, err := h.st.CreateIdentityProvider(h.ctx, nil, domain.IdentityProvider{
		OrgID: org.ID, Type: "oidc", Name: "Corp IdP", Slug: "corp",
		Issuer: f.srv.URL, ClientID: f.clientID, ClientSecret: "top-secret",
		Scopes: "openid email profile", AllowedEmailDomains: domains,
		AuthorizationEndpoint: f.srv.URL + "/authorize", TokenEndpoint: f.srv.URL + "/token",
		UserinfoEndpoint: f.srv.URL + "/userinfo", JWKSURI: f.srv.URL + "/jwks",
		PKCE: true, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateIdentityProvider: %v", err)
	}
	return org.ID, p
}

// login ejecuta /start (extrae state+nonce+cookie), firma el id_token con ese
// nonce y ejecuta /callback. Devuelve el recorder del callback.
func (h *fedHarness) login(t *testing.T, f *fedIdP, providerID, email string, verified bool) *httptest.ResponseRecorder {
	t.Helper()
	startRec := httptest.NewRecorder()
	h.mux.ServeHTTP(startRec, httptest.NewRequest(http.MethodGet, "/federated/"+providerID+"/start", http.NoBody))
	if startRec.Code != http.StatusFound {
		t.Fatalf("/start código=%d body=%s", startRec.Code, startRec.Body.String())
	}
	loc, _ := url.Parse(startRec.Header().Get("Location"))
	state := loc.Query().Get("state")
	nonce := loc.Query().Get("nonce")
	if state == "" || nonce == "" {
		t.Fatalf("/start sin state/nonce: %s", loc)
	}
	var fedCookie *http.Cookie
	for _, c := range startRec.Result().Cookies() {
		if c.Name == fedCookieName {
			fedCookie = c
		}
	}
	if fedCookie == nil {
		t.Fatal("/start no fijó la cookie de binding")
	}
	f.idToken = f.sign(t, f.clientID, nonce, email, verified)

	cbRec := httptest.NewRecorder()
	cbReq := httptest.NewRequest(http.MethodGet,
		"/federated/"+providerID+"/callback?"+url.Values{"state": {state}, "code": {"code-1"}}.Encode(), http.NoBody)
	cbReq.AddCookie(fedCookie)
	h.mux.ServeHTTP(cbRec, cbReq)
	return cbRec
}

func hasSessionCookie(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return true
		}
	}
	return false
}

// --- store: cifrado y aislamiento ---

func TestFederation_SecretEncryptedAtRest(t *testing.T) {
	h := newFedHarness(t)
	f := newFedIdP(t)
	_, p := h.seedProvider(t, f, "acme", []string{"acme.example"})

	// La columna en BD DEBE guardar el sobre cifrado (encv1:…), nunca el claro.
	var stored string
	if err := h.st.DB().QueryRow(h.ctx,
		`SELECT client_secret FROM identity_providers WHERE id=$1`, p.ID).Scan(&stored); err != nil {
		t.Fatalf("leer columna: %v", err)
	}
	if !strings.HasPrefix(stored, "encv1:") || strings.Contains(stored, "top-secret") {
		t.Fatalf("client_secret no cifrado en reposo: %q", stored)
	}
	// GetIdentityProvider lo descifra para uso interno.
	got, err := h.st.GetIdentityProvider(h.ctx, p.OrgID, p.ID)
	if err != nil || got.ClientSecret != "top-secret" {
		t.Fatalf("descifrado incorrecto: %q err=%v", got.ClientSecret, err)
	}
	// El listado NUNCA devuelve el secreto.
	list, _ := h.st.ListIdentityProviders(h.ctx, p.OrgID)
	if len(list) != 1 || list[0].ClientSecret != "" {
		t.Fatalf("listado expone el secreto: %+v", list)
	}
}

func TestFederation_OrgIsolation(t *testing.T) {
	h := newFedHarness(t)
	f := newFedIdP(t)
	_, p := h.seedProvider(t, f, "acme", []string{"acme.example"})
	other, err := h.st.CreateOrg(h.ctx, nil, domain.Organization{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	// Un provider de acme NO se lee ni borra desde beta.
	if _, err := h.st.GetIdentityProvider(h.ctx, other.ID, p.ID); err == nil {
		t.Fatal("GetIdentityProvider cruzó de org (fuga cross-tenant)")
	}
	if ok, _ := h.st.DeleteIdentityProvider(h.ctx, other.ID, p.ID); ok {
		t.Fatal("DeleteIdentityProvider borró un provider de otra org")
	}
	// El enrutado por dominio resuelve el provider correcto.
	got, err := h.st.FindProviderByEmailDomain(h.ctx, "acme.example")
	if err != nil || got.ID != p.ID {
		t.Fatalf("FindProviderByEmailDomain: %v id=%s", err, got.ID)
	}
}

// --- callback: aprovisionamiento y casado ---

func TestFederatedCallback_JITProvision(t *testing.T) {
	relaxSSRF(t)
	defer federation.SetInsecureForTest()()
	h := newFedHarness(t)
	f := newFedIdP(t)
	orgID, p := h.seedProvider(t, f, "acme", []string{"acme.example"})

	rec := h.login(t, f, p.ID, "ada@acme.example", true)
	if rec.Code != http.StatusFound || !hasSessionCookie(rec) {
		t.Fatalf("callback JIT: code=%d cookie=%v body=%s", rec.Code, hasSessionCookie(rec), rec.Body.String())
	}
	u, err := h.st.GetUserByEmail(h.ctx, "ada@acme.example")
	if err != nil {
		t.Fatalf("usuario no aprovisionado: %v", err)
	}
	if !u.EmailVerified || u.PasswordHash != "" {
		t.Fatalf("usuario JIT mal formado: verified=%v pwd=%q", u.EmailVerified, u.PasswordHash)
	}
	if _, err := h.st.GetMembership(h.ctx, orgID, u.ID); err != nil {
		t.Fatalf("sin membresía en la org del provider: %v", err)
	}
	if uid, err := h.st.GetFederatedUserID(h.ctx, p.ID, "ext-sub-1"); err != nil || uid != u.ID {
		t.Fatalf("sin enlace federated_identities: %v", err)
	}
}

func TestFederatedCallback_FindByFederatedIdentity(t *testing.T) {
	relaxSSRF(t)
	defer federation.SetInsecureForTest()()
	h := newFedHarness(t)
	f := newFedIdP(t)
	_, p := h.seedProvider(t, f, "acme", []string{"acme.example"})

	_ = h.login(t, f, p.ID, "ada@acme.example", true) // primer login (JIT)
	first, _ := h.st.GetUserByEmail(h.ctx, "ada@acme.example")
	rec := h.login(t, f, p.ID, "ada@acme.example", true) // segundo login
	if rec.Code != http.StatusFound || !hasSessionCookie(rec) {
		t.Fatalf("segundo login: code=%d", rec.Code)
	}
	second, _ := h.st.GetUserByEmail(h.ctx, "ada@acme.example")
	if first.ID != second.ID {
		t.Fatalf("se creó un usuario nuevo en el segundo login: %s vs %s", first.ID, second.ID)
	}
}

func TestFederatedCallback_FindByVerifiedEmail(t *testing.T) {
	relaxSSRF(t)
	defer federation.SetInsecureForTest()()
	h := newFedHarness(t)
	f := newFedIdP(t)
	orgID, p := h.seedProvider(t, f, "acme", []string{"acme.example"})

	// Usuario preexistente MIEMBRO de la org (sin enlace federado aún).
	pre, err := h.st.CreateUser(h.ctx, nil, domain.User{Email: "ada@acme.example", EmailVerified: true, Name: "Ada"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := h.st.UpsertMembership(h.ctx, nil, domain.Membership{OrgID: orgID, UserID: pre.ID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	rec := h.login(t, f, p.ID, "ada@acme.example", true)
	if rec.Code != http.StatusFound || !hasSessionCookie(rec) {
		t.Fatalf("login por email verificado: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if uid, err := h.st.GetFederatedUserID(h.ctx, p.ID, "ext-sub-1"); err != nil || uid != pre.ID {
		t.Fatalf("no enlazó al usuario existente: uid=%s err=%v", uid, err)
	}
}

func TestFederatedCallback_EmailUnverifiedRefused(t *testing.T) {
	relaxSSRF(t)
	defer federation.SetInsecureForTest()()
	h := newFedHarness(t)
	f := newFedIdP(t)
	_, p := h.seedProvider(t, f, "acme", nil) // sin restricción de dominio

	rec := h.login(t, f, p.ID, "ada@acme.example", false) // email_verified=false
	if rec.Code == http.StatusFound || hasSessionCookie(rec) {
		t.Fatalf("aceptó un email NO verificado (takeover): code=%d", rec.Code)
	}
	if _, err := h.st.GetUserByEmail(h.ctx, "ada@acme.example"); err == nil {
		t.Fatal("aprovisionó un usuario con email no verificado")
	}
}

func TestFederatedCallback_CrossOrgRefused(t *testing.T) {
	relaxSSRF(t)
	defer federation.SetInsecureForTest()()
	h := newFedHarness(t)
	f := newFedIdP(t)
	orgA, p := h.seedProvider(t, f, "acme", nil) // provider de la org A

	// Un usuario que YA existe y es miembro SOLO de la org B, con el mismo email.
	orgB, _ := h.st.CreateOrg(h.ctx, nil, domain.Organization{Slug: "beta", Name: "Beta"})
	victim, _ := h.st.CreateUser(h.ctx, nil, domain.User{Email: "ada@acme.example", EmailVerified: true, Name: "Ada"})
	_ = h.st.UpsertMembership(h.ctx, nil, domain.Membership{OrgID: orgB.ID, UserID: victim.ID, Role: domain.RoleOwner})

	rec := h.login(t, f, p.ID, "ada@acme.example", true)
	// El provider de A NO debe poder secuestrar la identidad de un usuario de B.
	if rec.Code == http.StatusFound || hasSessionCookie(rec) {
		t.Fatalf("secuestró un usuario de otra org: code=%d", rec.Code)
	}
	if _, err := h.st.GetFederatedUserID(h.ctx, p.ID, "ext-sub-1"); err == nil {
		t.Fatal("enlazó a un usuario de otra org (takeover cross-tenant)")
	}
	if _, err := h.st.GetMembership(h.ctx, orgA, victim.ID); err == nil {
		t.Fatal("añadió al usuario de B como miembro de A")
	}
}
