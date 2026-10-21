// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package integrationshttp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// proveedor es el token endpoint falso de las pruebas del consentimiento: apunta cada
// canje y responde un token que dice con qué código se pidió.
var proveedor struct {
	srv    *httptest.Server
	mu     sync.Mutex
	canjes []url.Values
}

func canjesDelProveedor() []url.Values {
	proveedor.mu.Lock()
	defer proveedor.mu.Unlock()
	return append([]url.Values(nil), proveedor.canjes...)
}

// TestMain arranca el proveedor antes que nada. El canje sale por el egress de la
// pasarela, que exige https y verifica el certificado con las raíces del sistema; el
// del proveedor falso se añade con SSL_CERT_FILE, que Go solo lee la PRIMERA vez que
// verifica un certificado en el proceso (Linux: el frontal y el CI).
func TestMain(m *testing.M) {
	proveedor.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		proveedor.mu.Lock()
		proveedor.canjes = append(proveedor.canjes, r.PostForm)
		proveedor.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.PostForm.Get("code") == "rooms-slack-grant" {
			_, _ = fmt.Fprint(w, `{"access_token":"slack-token","token_type":"bearer","incoming_webhook":{"url":"https://hooks.slack.com/services/T1/B1/secret-only-vault","channel_id":"C123","channel":"#general"},"team":{"id":"T1","name":"Acme"}}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"access_token":"token-de-%s","token_type":"bearer","expires_in":3600}`, r.PostForm.Get("code"))
	}))
	dir, err := os.MkdirTemp("", "proveedor-oauth")
	if err != nil {
		panic(err)
	}
	ca := filepath.Join(dir, "ca.pem")
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: proveedor.srv.Certificate().Raw})
	if err := os.WriteFile(ca, cert, 0o600); err != nil {
		panic(err)
	}
	_ = os.Setenv("SSL_CERT_FILE", ca)
	code := m.Run()
	proveedor.srv.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// manifiestoConConsentimiento es una plantilla con consentimiento contra el proveedor
// falso; la organización trae su propia app (`_oauth_client_id`).
func manifiestoConConsentimiento() string {
	return `{
  "name": "Prueba", "description": "d", "color": "#123456", "icon": "<svg/>",
  "identity": { "kind": "none" },
  "launch": { "kind": "none" },
  "data": "odata",
  "fields": [ { "key": "service_url", "label": "URL", "required": true } ],
  "oauth": { "authorize_url": "https://login.proveedor.example/authorize", "token_url": "` +
		proveedor.srv.URL + `/token", "scopes": ["leer"] }
}`
}

// consentimiento es una conexión de «acme» lista para conectar por consentimiento, y
// el router de la prueba. `rutas` monta lo que cada prueba necesite.
type consentimiento struct {
	st   *store.Store
	cif  *keycrypto.Cipher
	srv  *httptest.Server
	inst string
}

func montarConsentimiento(t *testing.T, rutas func(h *Handlers, r chi.Router)) *consentimiento {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
	pg := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	ctx := context.Background()
	if _, err := pg.Pool.Exec(ctx,
		`INSERT INTO app_templates (key, manifest, tier, enabled) VALUES ('prueba-oauth', $1::jsonb, 'standard', true)`,
		manifiestoConConsentimiento()); err != nil {
		t.Fatalf("plantilla: %v", err)
	}
	k := make([]byte, keycrypto.KeyLen)
	_, _ = rand.Read(k)
	cifrador, err := keycrypto.New(k)
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(pg.Pool, cifrador)
	cfg, _ := json.Marshal(map[string]string{"service_url": "https://erp.acme.example/odata", configKeyClientID: "app-de-acme"})
	inst, err := st.CreateInstance(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), "prueba-oauth", "Prueba", cfg, nil)
	if err != nil {
		t.Fatalf("instancia: %v", err)
	}
	h := &Handlers{
		store:         st,
		oauthClients:  newOAuthClientResolver(st, nil),
		oauthCipher:   cifrador,
		publicURL:     "https://integraciones.ejemplo",
		consoleURL:    "https://admin.ejemplo/admin",
		egressTimeout: 5 * time.Second,
		fondo:         ctx,
	}
	r := chi.NewRouter()
	// La sesión la pone la prueba con dos cabeceras: lo que haría RequireAuth +
	// RequireTenant con un token de Sytue ID.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sub := r.Header.Get("X-Prueba-Sub"); sub != "" {
				org := r.Header.Get("X-Prueba-Org")
				c := authn.Into(r.Context(), authn.Claims{Subject: sub, Tenant: org, Roles: []string{"owner"}})
				r = r.WithContext(tenancy.Into(c, tenancy.Tenant{Slug: org}))
			}
			next.ServeHTTP(w, r)
		})
	})
	r.Post("/instances/{id}/oauth/start", h.OAuthStart)
	r.Get("/oauth/callback", h.OAuthCallback)
	if rutas != nil {
		rutas(h, r)
	}
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &consentimiento{st: st, cif: cifrador, srv: srv, inst: inst.ID}
}

// pedir hace una petición con la sesión de `sub` en `org` (sin sesión si sub es "").
// No sigue redirecciones.
func (c *consentimiento) pedir(t *testing.T, metodo, ruta, sub, org string, cuerpo any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(cuerpo)
	req, err := http.NewRequest(metodo, c.srv.URL+ruta, strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if sub != "" {
		req.Header.Set("X-Prueba-Sub", sub)
		req.Header.Set("X-Prueba-Org", org)
	}
	cli := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// arrancar empieza el consentimiento como `sub` de acme y devuelve el `state` que el
// proveedor devolverá al navegador.
func (c *consentimiento) arrancar(t *testing.T, sub string) string {
	t.Helper()
	resp := c.pedir(t, http.MethodPost, "/instances/"+c.inst+"/oauth/start", sub, "acme", map[string]string{})
	var out oauthStartResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("start = %d (%v)", resp.StatusCode, err)
	}
	return stateDe(t, out.AuthorizeURL)
}

// stateDe saca el `state` de una URL de autorización.
func stateDe(t *testing.T, authorizeURL string) string {
	t.Helper()
	u, err := url.Parse(authorizeURL)
	if err != nil || u.Query().Get("state") == "" {
		t.Fatalf("authorize_url sin state: %q", authorizeURL)
	}
	return u.Query().Get("state")
}

// token es el access token guardado en la conexión ("" si no hay).
func (c *consentimiento) token(t *testing.T) string {
	t.Helper()
	inst, err := c.st.LoadInstanceWithSecrets(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), c.inst)
	if err != nil {
		t.Fatal(err)
	}
	return inst.Secrets[store.SecretKeyBearer]
}

// NEG-10. La vuelta del proveedor no lleva sesión. Si canjea ella, el `state` es la
// única autorización, y el estado se puede llevar a otro navegador: Ana arranca el
// consentimiento de la conexión de su organización, manda la URL de autorización a
// Bea (o a cualquiera), y lo que Bea autoriza en SU cuenta del proveedor acaba en la
// conexión de Ana. La vuelta solo puede reenviar el navegador a quien sí tiene sesión.
func TestLaVueltaDelProveedorSinSesionNoGuardaNingunToken(t *testing.T) {
	c := montarConsentimiento(t, nil)
	antes := len(canjesDelProveedor())
	state := c.arrancar(t, "ana")

	// El navegador de Bea vuelve del proveedor con el código de SU cuenta.
	resp := c.pedir(t, http.MethodGet, "/oauth/callback?"+url.Values{"state": {state}, "code": {"codigo-de-bea"}}.Encode(), "", "", nil)

	if tok := c.token(t); tok != "" {
		t.Errorf("la vuelta sin sesión guardó un token en la conexión de Ana: %q", tok)
	}
	if n := len(canjesDelProveedor()) - antes; n != 0 {
		t.Errorf("la vuelta sin sesión canjeó %d códigos", n)
	}
	destino, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusFound || destino == nil ||
		destino.Host != "admin.ejemplo" || destino.Path != "/admin/api/admin/apps/oauth-finish" ||
		destino.Query().Get("state") != state || destino.Query().Get("code") != "codigo-de-bea" {
		t.Errorf("la vuelta tiene que reenviar a la consola con state y code: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}
