// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/lestrrat-go/jwx/v2/jwt"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// --- parseAuthz ---

func TestParseAuthzQuery(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet,
		"/oauth2/authorize?client_id=cli&redirect_uri=https%3A%2F%2Fapp%2Fcb&response_type=code"+
			"&scope=+openid+profile+&state=st&nonce=n&code_challenge=chal", http.NoBody)
	got := parseAuthz(r)
	if got.ClientID != "cli" || got.RedirectURI != "https://app/cb" || got.ResponseType != "code" {
		t.Fatalf("campos base: %+v", got)
	}
	if got.Scope != "openid profile" {
		t.Fatalf("scope no recortado: %q", got.Scope)
	}
	if got.State != "st" || got.Nonce != "n" || got.CodeChallenge != "chal" {
		t.Fatalf("otros campos: %+v", got)
	}
	// code_challenge presente sin método → S256 por defecto.
	if got.CodeChallengeMethod != "S256" {
		t.Fatalf("método PKCE por defecto=%q", got.CodeChallengeMethod)
	}
}

func TestParseAuthzPostForm(t *testing.T) {
	form := url.Values{
		"client_id":             {"cli"},
		"redirect_uri":          {"https://app/cb"},
		"response_type":         {"code"},
		"code_challenge":        {"c"},
		"code_challenge_method": {"plain"},
	}
	r := httptest.NewRequest(http.MethodPost, "/oauth2/authorize", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	got := parseAuthz(r)
	if got.ClientID != "cli" || got.CodeChallengeMethod != "plain" {
		t.Fatalf("POST form: %+v", got)
	}
}

func TestParseAuthzNoChallengeNoMethod(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/oauth2/authorize?client_id=cli", http.NoBody)
	if got := parseAuthz(r); got.CodeChallengeMethod != "" {
		t.Fatalf("sin challenge no debe forzar método: %q", got.CodeChallengeMethod)
	}
}

// --- Discovery / JWKS ---

func TestHandleAccountAvatarGetPermiteImagenesDelMismoSitio(t *testing.T) {
	// El avatar se consume desde las superficies de la suite, no solo desde
	// id.sytue.eu. La respuesta mantiene la frontera del sitio sin caer en
	// cross-origin, incluso cuando no hay sesión y acaba en 401.
	s := &Server{}
	rr := httptest.NewRecorder()
	s.handleAccountAvatarGet(rr, httptest.NewRequest(http.MethodGet, "/account/avatar", http.NoBody))
	if got := rr.Header().Get("Cross-Origin-Resource-Policy"); got != "same-site" {
		t.Fatalf("Cross-Origin-Resource-Policy=%q, quiero same-site", got)
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, quiero %d sin sesión", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleDiscovery(t *testing.T) {
	s := &Server{cfg: Config{Issuer: "https://id.example"}}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", http.NoBody)
	s.handleDiscovery(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if cc := rr.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=300") {
		t.Fatalf("cache-control=%q", cc)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type=%q", ct)
	}
	var doc oidc.DiscoveryDocument
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	if doc.Issuer != "https://id.example" ||
		doc.TokenEndpoint != "https://id.example/oauth2/token" {
		t.Fatalf("discovery inesperado: %+v", doc)
	}
}

// TestDiscoveryRequiredFields comprueba, sobre el JSON REAL (no la struct), que están
// los metadatos que las librerías cliente exigen para autoconfigurarse (OIDC Discovery
// 1.0 §3 + RFC 8414), y que no se anuncia lo que el servicio no hace.
func TestDiscoveryRequiredFields(t *testing.T) {
	s := &Server{cfg: Config{Issuer: "https://id.example"}}
	rr := httptest.NewRecorder()
	s.handleDiscovery(rr, httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", http.NoBody))
	var doc map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("json: %v", err)
	}

	// Endpoints: presentes y colgando del issuer anunciado.
	for field, want := range map[string]string{
		"issuer":                 "https://id.example",
		"authorization_endpoint": "https://id.example/oauth2/authorize",
		"token_endpoint":         "https://id.example/oauth2/token",
		"userinfo_endpoint":      "https://id.example/oauth2/userinfo",
		"jwks_uri":               "https://id.example/.well-known/jwks.json",
		"introspection_endpoint": "https://id.example/oauth2/introspect",
		"revocation_endpoint":    "https://id.example/oauth2/revoke",
		"end_session_endpoint":   "https://id.example/oauth2/logout",
		"registration_endpoint":  "https://id.example/oauth2/register",
	} {
		if got, _ := doc[field].(string); got != want {
			t.Errorf("%s = %q, se esperaba %q", field, got, want)
		}
	}

	// Listas que las librerías leen para negociar el flujo.
	for _, field := range []string{
		"scopes_supported", "response_types_supported", "response_modes_supported",
		"grant_types_supported", "subject_types_supported",
		"id_token_signing_alg_values_supported", "token_endpoint_auth_methods_supported",
		"introspection_endpoint_auth_methods_supported",
		"revocation_endpoint_auth_methods_supported",
		"code_challenge_methods_supported", "claims_supported",
	} {
		if v, ok := doc[field].([]any); !ok || len(v) == 0 {
			t.Errorf("%s ausente o vacío: %v", field, doc[field])
		}
	}
	// Declaraciones negativas explícitas (no adivinar): deben venir y ser false.
	for _, field := range []string{
		"request_parameter_supported", "request_uri_parameter_supported",
		"claims_parameter_supported",
	} {
		v, ok := doc[field].(bool)
		if !ok || v {
			t.Errorf("%s debería venir como false: %v", field, doc[field])
		}
	}

	strs := func(field string) []string {
		vals := doc[field].([]any)
		out := make([]string, 0, len(vals))
		for _, v := range vals {
			out = append(out, v.(string))
		}
		return out
	}
	// No anunciar de más: sin `plain` (el authorize lo rechaza), sin `fragment` (el
	// authorize siempre responde por query) y sin response types implícitos.
	if got := strs("code_challenge_methods_supported"); !slices.Equal(got, []string{"S256"}) {
		t.Errorf("code_challenge_methods_supported=%v; solo S256 está implementado", got)
	}
	if got := strs("response_modes_supported"); !slices.Equal(got, []string{"query"}) {
		t.Errorf("response_modes_supported=%v; el authorize solo responde por query", got)
	}
	if got := strs("response_types_supported"); !slices.Equal(got, []string{"code"}) {
		t.Errorf("response_types_supported=%v; solo hay Authorization Code", got)
	}
	// Introspección: sin `none`. Anunciarlo decía que se puede preguntar por un
	// token sin autenticarse, que es el hallazgo automático de cualquier revisión
	// de OAuth2 (RFC 7662 §2.1) — y handleIntrospect rechaza a los públicos.
	if got := strs("introspection_endpoint_auth_methods_supported"); slices.Contains(got, "none") {
		t.Errorf("introspection_endpoint_auth_methods_supported=%v; la introspección exige client confidencial", got)
	}
	// Revocación: CON `none`, a propósito. Una app móvil o una SPA no tienen
	// secreto y deben poder anular su refresh token al cerrar sesión (RFC 7009);
	// quitarlo dejaría vivos justo los tokens que el usuario pidió anular.
	if got := strs("revocation_endpoint_auth_methods_supported"); !slices.Contains(got, "none") {
		t.Errorf("revocation_endpoint_auth_methods_supported=%v; un client público debe poder revocar sus tokens", got)
	}
	// Los claims anunciados incluyen los que userinfo devuelve de verdad.
	claims := strs("claims_supported")
	for _, c := range []string{
		"sub", "email", "email_verified", "name", "given_name",
		"family_name", "preferred_username",
	} {
		if !slices.Contains(claims, c) {
			t.Errorf("claims_supported no incluye %q: %v", c, claims)
		}
	}
	if !slices.Contains(claims, "picture") {
		t.Error("claims_supported no anuncia picture: el usuario SÍ tiene avatar desde la 0067")
	}
}

// TestUserinfoClaimsByScope: userinfo solo devuelve lo concedido, y omite lo que el
// usuario no tiene en vez de mandar cadenas vacías.
func TestUserinfoClaimsByScope(t *testing.T) {
	u := domain.User{
		ID: "u-1", Email: "grace@acme.example", EmailVerified: true,
		Name: "Grace Hopper", GivenName: "Grace", FamilyName: "Hopper", Locale: "es-ES",
	}

	// Solo openid: nada más que el sujeto.
	got := userinfoClaims(u, []string{"openid"}, "https://id.example/avatar?v=1")
	if len(got) != 1 || got["sub"] != "u-1" {
		t.Fatalf("con solo openid debería venir sub y nada más: %v", got)
	}

	// email: dirección + verificación, sin perfil.
	got = userinfoClaims(u, []string{"openid", "email"}, "https://id.example/avatar?v=1")
	if got["email"] != "grace@acme.example" || got["email_verified"] != true {
		t.Errorf("scope email: %v", got)
	}
	if _, ok := got["name"]; ok {
		t.Errorf("scope email no debe filtrar el perfil: %v", got)
	}

	// profile: nombre desglosado + preferred_username (el login de Sytue es el email).
	got = userinfoClaims(u, []string{"openid", "profile", "email"}, "")
	for k, want := range map[string]any{
		"name": "Grace Hopper", "given_name": "Grace", "family_name": "Hopper",
		"locale": "es-ES", "preferred_username": "grace@acme.example",
	} {
		if got[k] != want {
			t.Errorf("%s = %v, se esperaba %v", k, got[k], want)
		}
	}
	if _, ok := got["picture"]; ok {
		t.Errorf("sin foto, picture no debe inventarse: %v", got)
	}
	// Con foto, la MISMA URL que el id_token, y solo con el scope profile.
	got = userinfoClaims(u, []string{"openid", "profile"}, "https://id.example/avatar?v=1")
	if got["picture"] != "https://id.example/avatar?v=1" {
		t.Errorf("con foto, picture = %v", got["picture"])
	}

	// Campos vacíos: se OMITEN (una librería no debe pintar cadenas vacías).
	got = userinfoClaims(domain.User{ID: "u-2", Email: "min@acme.example"},
		[]string{"openid", "profile", "email"}, "")
	for _, k := range []string{"name", "given_name", "family_name", "locale"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s vacío debería omitirse: %v", k, got)
		}
	}
	if got["preferred_username"] != "min@acme.example" {
		t.Errorf("preferred_username debería ser el email: %v", got)
	}
}

func TestHandleJWKSEmpty(t *testing.T) {
	s := &Server{keys: oidc.NewKeyManager()}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", http.NoBody)
	s.handleJWKS(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	if _, ok := out["keys"]; !ok {
		t.Fatalf("jwks sin campo keys: %s", rr.Body.String())
	}
}

// --- securityHeaders ---

func TestSecurityHeadersSecure(t *testing.T) {
	mw := securityHeaders(Config{CookieSecure: true})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	h := rr.Header()
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("nosniff ausente")
	}
	if h.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("frame options")
	}
	if !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("CSP")
	}
	if h.Get("Strict-Transport-Security") == "" {
		t.Fatalf("HSTS ausente con CookieSecure")
	}
}

func TestSecurityHeadersInsecureNoHSTS(t *testing.T) {
	mw := securityHeaders(Config{CookieSecure: false})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if rr.Header().Get("Strict-Transport-Security") != "" {
		t.Fatalf("HSTS no debe emitirse sin CookieSecure")
	}
}

// --- bearer ---

func TestBearer(t *testing.T) {
	cases := []struct {
		hdr    string
		want   string
		wantOK bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer xyz", "xyz", true},
		{"Bearer   spaced  ", "spaced", true},
		{"Basic abc", "", false},
		{"", "", false},
		{"Bearer", "", false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		if tc.hdr != "" {
			r.Header.Set("Authorization", tc.hdr)
		}
		got, ok := bearer(r)
		if got != tc.want || ok != tc.wantOK {
			t.Fatalf("bearer(%q)=(%q,%v), se esperaba (%q,%v)", tc.hdr, got, ok, tc.want, tc.wantOK)
		}
	}
}

// --- requireSCIMAuth ---

func TestRequireSCIMAuthUnconfigured(t *testing.T) {
	s := &Server{cfg: Config{SCIMBearerToken: ""}}
	rr := httptest.NewRecorder()
	called := false
	s.requireSCIMAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).
		ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/scim/v2/Users", http.NoBody))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}
	if called {
		t.Fatal("no debía llamar a next")
	}
}

// TestRequireSCIMAuthSinOrg: con token pero SIN organización ligada, SCIM no se
// sirve. El token SCIM es único para todo el despliegue: sin tenant al que
// acotar, /Users enumeraría el directorio ENTERO, de todas las organizaciones.
func TestRequireSCIMAuthSinOrg(t *testing.T) {
	s := &Server{cfg: Config{SCIMBearerToken: "secret"}} // SCIMOrgSlug vacío
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/scim/v2/Users", http.NoBody)
	r.Header.Set("Authorization", "Bearer secret")
	called := false
	s.requireSCIMAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(rr, r)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d; quería 503 (SCIM no configurado)", rr.Code)
	}
	if called {
		t.Fatal("no debía llamar a next: sin org ligada no hay ámbito que servir")
	}
}

func TestRequireSCIMAuthInvalidToken(t *testing.T) {
	s := &Server{cfg: Config{SCIMBearerToken: "secret", SCIMOrgSlug: "acme"}}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/scim/v2/Users", http.NoBody)
	r.Header.Set("Authorization", "Bearer wrong")
	s.requireSCIMAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestRequireSCIMAuthValid(t *testing.T) {
	s := &Server{cfg: Config{SCIMBearerToken: "secret", SCIMOrgSlug: "acme"}}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/scim/v2/Users", http.NoBody)
	r.Header.Set("Authorization", "Bearer secret")
	called := false
	s.requireSCIMAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, r)
	if !called || rr.Code != http.StatusOK {
		t.Fatalf("token válido no pasó: called=%v code=%d", called, rr.Code)
	}
}

// --- requireRole ---

func TestRequireRole(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	s := &Server{}

	// Sin claims en contexto → 401.
	rr := httptest.NewRecorder()
	s.requireRole("admin")(next).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("sin claims status=%d", rr.Code)
	}

	// Rol correcto → pasa.
	rr = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	r = r.WithContext(context.WithValue(r.Context(), ctxKeyClaims{}, tokenClaims{Roles: []string{"admin"}}))
	s.requireRole("owner", "admin")(next).ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("rol correcto status=%d", rr.Code)
	}

	// Rol insuficiente → 403.
	rr = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	r = r.WithContext(context.WithValue(r.Context(), ctxKeyClaims{}, tokenClaims{Roles: []string{"member"}}))
	s.requireRole("admin")(next).ServeHTTP(rr, r)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("rol insuficiente status=%d", rr.Code)
	}
}

// --- claimsFrom / claimStrings ---

func TestClaimsFrom(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKeyClaims{}, tokenClaims{Subject: "u"})
	if c, ok := claimsFrom(ctx); !ok || c.Subject != "u" {
		t.Fatalf("claimsFrom=%+v %v", c, ok)
	}
	if _, ok := claimsFrom(context.Background()); ok {
		t.Fatal("contexto vacío no debe traer claims")
	}
}

func TestClaimStrings(t *testing.T) {
	build := func(v any) jwt.Token {
		tok := jwt.New()
		_ = tok.Set("k", v)
		return tok
	}
	if got := claimStrings(build([]string{"a", "b"}), "k"); len(got) != 2 || got[0] != "a" {
		t.Fatalf("[]string: %v", got)
	}
	if got := claimStrings(build([]any{"a", 1, "c"}), "k"); len(got) != 2 || got[1] != "c" {
		t.Fatalf("[]any filtrado: %v", got)
	}
	if got := claimStrings(build("a b c"), "k"); len(got) != 3 {
		t.Fatalf("string fields: %v", got)
	}
	if got := claimStrings(build(42), "k"); got != nil {
		t.Fatalf("tipo no soportado: %v", got)
	}
	if got := claimStrings(jwt.New(), "missing"); got != nil {
		t.Fatalf("clave ausente: %v", got)
	}
}

// TestHasAnyRole cubre la autorización por-org del hallazgo 1: la comprobación de
// owner/admin sobre los roles reales del solicitante en la org objetivo.
func TestHasAnyRole(t *testing.T) {
	if !hasAnyRole([]string{"admin"}, domain.RoleOwner, domain.RoleAdmin) {
		t.Error("admin debería satisfacer owner|admin")
	}
	if !hasAnyRole([]string{"owner"}, domain.RoleOwner) {
		t.Error("owner debería satisfacer owner")
	}
	if hasAnyRole([]string{"member"}, domain.RoleOwner, domain.RoleAdmin) {
		t.Error("member NO debe satisfacer owner|admin")
	}
	if hasAnyRole([]string{"admin"}, domain.RoleOwner) {
		t.Error("admin NO debe satisfacer owner (no puede conceder owner)")
	}
	if hasAnyRole(nil, domain.RoleOwner, domain.RoleAdmin) {
		t.Error("sin roles (fail-closed) NO debe autorizar")
	}
}

// TestOAuth2RateKeyNoComparteBucketConLogin fija que el bucket de los endpoints
// OAuth2 y el del login son filas DISTINTAS de platform_rate_limits. Ambos
// limitadores se comparten entre réplicas sobre la misma tabla y ambos se
// aplican por IP; si el de OAuth2 usara la clave por defecto (la IP «desnuda»,
// que es la que usa el login) escribirían la misma fila y el tráfico normal de
// refresh —60/min— recargaría el bucket anti-fuerza-bruta del login, que es de
// 10/min. La clave de OAuth2 debe ir con prefijo propio.
func TestOAuth2RateKeyNoComparteBucketConLogin(t *testing.T) {
	// Detrás del proxy (RemoteAddr privado), que es como llega el tráfico real y
	// la única forma de que X-Forwarded-For cuente.
	r := trasElProxy(httptest.NewRequest(http.MethodPost, "/oauth2/token", http.NoBody), "203.0.113.9, 10.0.0.1")

	got := oauth2RateKey(r)
	if loginKey := loginRateKey(r); got == loginKey {
		t.Fatalf("oauth2RateKey = %q, la MISMA clave que el login: compartirían fila y presupuesto", got)
	}
	if !strings.HasPrefix(got, "oauth2:") {
		t.Errorf("oauth2RateKey = %q, quiero el prefijo de espacio de nombres \"oauth2:\"", got)
	}
	// Sigue siendo por IP: dos clientes distintos, dos buckets.
	otra := trasElProxy(httptest.NewRequest(http.MethodPost, "/oauth2/token", http.NoBody), "198.51.100.4")
	if oauth2RateKey(otra) == got {
		t.Errorf("oauth2RateKey no distingue IPs: %q", got)
	}
}

// trasElProxy prepara una petición como la que entrega Traefik: viene de una red
// privada (peer de confianza) y trae la cadena X-Forwarded-For dada.
func trasElProxy(r *http.Request, xff string) *http.Request {
	r.RemoteAddr = "10.42.0.7:44321"
	r.Header.Set("X-Forwarded-For", xff)
	return r
}

// TestClientIPNoSeCreeUnXFFDeCualquiera es la verja: X-Forwarded-For es texto que
// escribe quien hace la petición. Creerlo sin mirar de dónde viene convertía en
// falsificables el bucket por IP del login (una IP nueva por intento ⇒ el tope de
// fuerza bruta no frena nada), las condiciones por IP del Acceso Condicional y la
// IP del registro de auditoría.
func TestClientIPNoSeCreeUnXFFDeCualquiera(t *testing.T) {
	casos := []struct {
		nombre, remote, xff, quiero string
	}{
		{
			// Conexión DIRECTA (peer público): la cabecera se ignora entera.
			"peer no confiable", "198.51.100.77:5555", "1.2.3.4", "198.51.100.77",
		},
		{
			// Detrás del proxy: la entrada de la derecha que no es proxy nuestro.
			"tras el proxy", "10.42.0.7:44321", "203.0.113.9", "203.0.113.9",
		},
		{
			// Lo que el cliente inventó queda a la IZQUIERDA de lo que añadió el
			// proxy, y por eso se lee de derecha a izquierda.
			"xff inyectado por el cliente", "10.42.0.7:44321", "9.9.9.9, 203.0.113.9", "203.0.113.9",
		},
		{
			// Varios saltos internos: se salta a los proxies y se llega al cliente.
			"cadena con saltos internos", "10.42.0.7:44321", "203.0.113.9, 10.0.0.1, 10.0.0.2", "203.0.113.9",
		},
		{
			// Cabecera basura: se cae al peer en vez de devolver "".
			"xff ilegible", "10.42.0.7:44321", "no-es-una-ip", "10.42.0.7",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/login/", http.NoBody)
			r.RemoteAddr = c.remote
			r.Header.Set("X-Forwarded-For", c.xff)
			if got := clientIP(r); got != c.quiero {
				t.Errorf("clientIP = %q, quiero %q", got, c.quiero)
			}
		})
	}
}
