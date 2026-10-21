// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package gws_test

// Tests OFFLINE del driver Google Workspace contra un httptest (sin red ni Google real).
// El egress va por el Runtime real de la pasarela (policy.BuildRuntime → ssrfhttp). El
// driver firma un JWT (RS256) con una cuenta de servicio generada EN EL TEST y lo canjea
// por un token en el token endpoint del httptest; el token minteado va como Bearer en el
// Directory API (el Runtime NO inyecta bearer: estos conectores no tienen uno estático).
// El camino feliz relaja la barrera SSRF para loopback (SetIPPolicyForTest) y la
// restaura; el test de host privado NO la relaja y afirma ErrBlocked. Sin t.Parallel():
// la política SSRF es una var de proceso.

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/gws"
	sytueoauth2 "git.sytue.eu/sytue/one/services/integrations/internal/oauth2"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

func allowLoopback(t *testing.T) {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
}

func runtime() connector.Runtime {
	// GWS mintea su propio token (jwt-bearer): sin inyección de bearer.
	return policy.BuildRuntime(policy.RuntimeParams{OrgID: "org-1", InstanceID: "inst-1", Log: slog.Default()})
}

func mustFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

// serviceAccountJSON genera una cuenta de servicio de prueba (clave RSA propia, token_uri
// apuntando al httptest) y devuelve su JSON y la clave pública para verificar la firma.
func serviceAccountJSON(t *testing.T, tokenURI string) (string, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generar clave: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	saJSON, err := json.Marshal(map[string]any{
		"type":         "service_account",
		"client_email": "svc@proj.iam.gserviceaccount.com",
		"private_key":  string(pemBytes),
		"token_uri":    tokenURI,
	})
	if err != nil {
		t.Fatalf("marshal SA: %v", err)
	}
	return string(saJSON), &key.PublicKey
}

// gwsServer levanta un Directory API mínimo: token endpoint (jwt-bearer) + la colección
// users en dos páginas (nextPageToken). Captura el Authorization visto por el Directory y
// el assertion (JWT) recibido por el token endpoint.
func gwsServer(t *testing.T) (*httptest.Server, *string, *string) {
	t.Helper()
	page1 := mustFixture(t, "directory_users.json")
	page2 := mustFixture(t, "directory_users_page2.json")
	token := mustFixture(t, "token.json")
	var lastAuth, lastAssertion string

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		lastAssertion = r.PostForm.Get("assertion")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(token)
	})
	mux.HandleFunc("/admin/directory/v1/users", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "TOKEN2" {
			_, _ = w.Write(page2)
			return
		}
		_, _ = w.Write(page1)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastAuth, &lastAssertion
}

func instance(base, saJSON string) connector.Instance {
	return connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: gws.Kind,
		Config: map[string]string{
			gws.ConfigAdminEmail:    "admin@acme.com",
			gws.ConfigDirectoryBase: base,
		},
		Secrets: map[string]string{"service_account_key": saJSON},
	}
}

func TestQueryMintsTokenPaginatesNormalizes(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, lastAssertion := gwsServer(t)
	saJSON, pub := serviceAccountJSON(t, srv.URL+"/token")

	tbl, err := gws.New(sytueoauth2.NewCache()).Query(t.Context(), instance(srv.URL, saJSON), runtime(), []byte(`{"resource":"users"}`))
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	// Paginación por nextPageToken: 2 filas de la página 1 + 1 de la página 2.
	if len(tbl.Rows) != 3 {
		t.Fatalf("filas = %d, quiero 3 (paginación por nextPageToken)", len(tbl.Rows))
	}
	// Columnas = unión alfabética de claves (sin order preferente).
	want := []string{"id", "isAdmin", "primaryEmail", "suspended"}
	if len(tbl.Columns) != len(want) {
		t.Fatalf("columnas = %+v, quiero %v", tbl.Columns, want)
	}
	ct := map[string]string{}
	for i, c := range tbl.Columns {
		if c.Name != want[i] {
			t.Fatalf("columna %d = %q, quiero %q", i, c.Name, want[i])
		}
		ct[c.Name] = c.Type
	}
	if ct["id"] != "text" || ct["isAdmin"] != "boolean" || ct["suspended"] != "boolean" {
		t.Fatalf("tipos inferidos mal: %v", ct)
	}
	// El token minteado por el driver va como Bearer (el Runtime NO lo inyecta).
	if *lastAuth != "Bearer gws-access-token-xyz" {
		t.Fatalf("Authorization = %q, quiero Bearer gws-access-token-xyz (token minteado)", *lastAuth)
	}
	// El assertion es un JWT RS256 bien firmado con la cuenta de servicio y con los
	// claims correctos (iss/sub/aud/scope) — la ruta de seguridad del driver.
	assertClaims(t, *lastAssertion, pub, srv.URL+"/token")
}

// assertClaims verifica que el assertion es un JWT RS256 firmado por pub y con los claims
// esperados (iss = cuenta de servicio, sub = admin impersonado, aud = token endpoint,
// scope de solo lectura del Directory).
func assertClaims(t *testing.T, jwt string, pub *rsa.PublicKey, tokenURI string) {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("assertion no es un JWT de 3 partes: %q", jwt)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("firma no es base64url: %v", err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("firma RS256 inválida: %v", err)
	}
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("claims no es base64url: %v", err)
	}
	var c struct {
		Iss, Sub, Aud, Scope string
	}
	if err := json.Unmarshal(claimsJSON, &c); err != nil {
		t.Fatalf("claims no es JSON: %v", err)
	}
	if c.Iss != "svc@proj.iam.gserviceaccount.com" {
		t.Errorf("iss = %q, quiero el email de la cuenta de servicio", c.Iss)
	}
	if c.Sub != "admin@acme.com" {
		t.Errorf("sub = %q, quiero el admin impersonado", c.Sub)
	}
	if c.Aud != tokenURI {
		t.Errorf("aud = %q, quiero el token endpoint %q", c.Aud, tokenURI)
	}
	if !strings.Contains(c.Scope, "admin.directory.user.readonly") {
		t.Errorf("scope = %q, quiero el de solo lectura del Directory", c.Scope)
	}
}

// TestQueryAPIError: un 403 del Directory con cuerpo de error se mapea a un error legible.
func TestQueryAPIError(t *testing.T) {
	allowLoopback(t)
	token := mustFixture(t, "token.json")
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(token) })
	mux.HandleFunc("/admin/directory/v1/users", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"Not Authorized to access this resource/api"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	saJSON, _ := serviceAccountJSON(t, srv.URL+"/token")

	_, err := gws.New(sytueoauth2.NewCache()).Query(t.Context(), instance(srv.URL, saJSON), runtime(), []byte(`{"resource":"users"}`))
	if err == nil {
		t.Fatal("un 403 del Directory debería propagarse como error")
	}
	if !strings.Contains(err.Error(), "Not Authorized") {
		t.Fatalf("err = %v, quiero el message de Google", err)
	}
}

// TestQueryPrivateURLBlocked: SIN relajar la barrera, las bases apuntan a loopback ⇒ el
// egress (empezando por el token endpoint) se bloquea con ssrfhttp.ErrBlocked.
func TestQueryPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"x"}`))
	}))
	defer srv.Close()
	saJSON, _ := serviceAccountJSON(t, srv.URL+"/token")

	_, err := gws.New(sytueoauth2.NewCache()).Query(t.Context(), instance(srv.URL, saJSON), runtime(), []byte(`{"resource":"users"}`))
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (host privado)", err)
	}
}

// TestTestReachability: Test sondea la colección users (reachability, SIN auth) y exige
// que el Directory no responda 5xx.
func TestTestReachability(t *testing.T) {
	allowLoopback(t)
	srv, _, _ := gwsServer(t)
	if err := gws.New(sytueoauth2.NewCache()).Test(t.Context(), instance(srv.URL, ""), runtime()); err != nil {
		t.Fatalf("Test camino feliz: %v", err)
	}
}

// TestTestPrivateBlocked: SIN relajar la barrera, Test a loopback ⇒ ssrfhttp.ErrBlocked.
func TestTestPrivateBlocked(t *testing.T) {
	priv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer priv.Close()
	err := gws.New(sytueoauth2.NewCache()).Test(t.Context(), instance(priv.URL, ""), runtime())
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("Test host privado: err = %v, quiero ssrfhttp.ErrBlocked", err)
	}
}

func TestQueryBadSpec(t *testing.T) {
	saJSON, _ := serviceAccountJSON(t, "https://oauth2.googleapis.com/token")
	// Recurso no soportado ⇒ ErrBadQuery, sin tocar la red.
	_, err := gws.New(sytueoauth2.NewCache()).Query(t.Context(), instance("https://admin.googleapis.com", saJSON), runtime(), []byte(`{"resource":"devices"}`))
	if !errors.Is(err, connector.ErrBadQuery) {
		t.Fatalf("err = %v, quiero connector.ErrBadQuery (recurso no soportado)", err)
	}
	// Falta la clave de cuenta de servicio ⇒ ErrBadQuery.
	_, err = gws.New(sytueoauth2.NewCache()).Query(t.Context(), instance("https://admin.googleapis.com", ""), runtime(), []byte(`{"resource":"users"}`))
	if !errors.Is(err, connector.ErrBadQuery) {
		t.Fatalf("err = %v, quiero connector.ErrBadQuery (falta el secreto)", err)
	}
}
