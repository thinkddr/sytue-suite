// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package manifest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/directory"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// idMock imita las rutas admin de W2A que la materialización llama en Sytue ID:
// /oauth2/token (client_credentials), crear/borrar SP SAML, emitir token DCR y
// registrar el client OIDC. Cuenta operaciones y captura el último cuerpo para
// afirmar QUÉ se crea/borra. failSAML fuerza un 500 al crear el SP (id «falla a media»).
type idMock struct {
	mu        sync.Mutex
	samlPosts int
	samlDels  int
	dcrTokens int
	regPosts  int
	lastSAML  map[string]any
	lastReg   map[string]any
	failSAML  bool
}

func (m *idMock) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		switch {
		case r.URL.Path == "/oauth2/token":
			writeJSON(w, http.StatusOK, map[string]any{"access_token": "svc-tok", "expires_in": 3600})

		case r.URL.Path == "/api/v1/admin/apps/saml-sps" && r.Method == http.MethodPost:
			assertBearer(t, r, "svc-tok")
			m.samlPosts++
			if m.failSAML {
				writeJSON(w, http.StatusBadGateway, map[string]any{"error": "boom"})
				return
			}
			m.lastSAML = decode(t, r)
			writeJSON(w, http.StatusCreated, map[string]any{
				"id":               "sp-123",
				"initiate_url":     "https://id.test/saml/idp/initiate/sp-123",
				"idp_metadata_url": "https://id.test/saml/idp/metadata",
			})

		case strings.HasPrefix(r.URL.Path, "/api/v1/admin/apps/saml-sps/") && r.Method == http.MethodDelete:
			assertBearer(t, r, "svc-tok")
			m.samlDels++
			writeJSON(w, http.StatusOK, map[string]any{"id": "sp-123", "deleted": true})

		case r.URL.Path == "/api/v1/admin/apps/dcr-tokens" && r.Method == http.MethodPost:
			assertBearer(t, r, "svc-tok")
			m.dcrTokens++
			writeJSON(w, http.StatusCreated, map[string]any{
				"initial_access_token":  "iat-xyz",
				"registration_endpoint": "https://id.test/oauth2/register",
			})

		case r.URL.Path == "/oauth2/register" && r.Method == http.MethodPost:
			assertBearer(t, r, "iat-xyz") // el registro usa el token de acceso inicial, no el de servicio.
			m.regPosts++
			m.lastReg = decode(t, r)
			writeJSON(w, http.StatusCreated, map[string]any{
				"client_id":     "dcr-abc",
				"client_secret": "s3cr3t",
			})

		default:
			t.Errorf("ruta inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newIDApps(url string) manifest.IDApps {
	return directory.New(directory.Config{BaseURL: url, ClientID: "sytue-gateway", ClientSecret: "shh"})
}

func samlManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Name:         "Salesforce",
		Description:  "CRM",
		Color:        "#00A1E0",
		Icon:         "<svg/>",
		Provisioning: manifest.ProvisioningSCIM,
		Identity: manifest.Identity{
			Kind:         manifest.IdentitySAML,
			EntityID:     "https://{subdomain}.my.salesforce.com",
			ACSURL:       "https://{subdomain}.my.salesforce.com",
			NameIDFormat: "email",
		},
		Launch: manifest.Launch{Kind: manifest.LaunchInitiate},
		Fields: []manifest.Field{{Key: "subdomain", Label: "Subdominio", Required: true}},
	}
}

func oidcManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Name:        "GitHub Enterprise",
		Description: "Código",
		Color:       "#6E7681",
		Icon:        "<svg/>",
		Identity: manifest.Identity{
			Kind:         manifest.IdentityOIDC,
			RedirectURIs: []string{"https://{host}/oidc/callback"},
		},
		Launch: manifest.Launch{Kind: manifest.LaunchURL, URL: "https://{host}"},
		Fields: []manifest.Field{{Key: "host", Label: "Host", Required: true}},
	}
}

func TestMaterialize_SAML_createsSPandLaunch(t *testing.T) {
	mock := &idMock{}
	srv := mock.server(t)
	defer srv.Close()
	ids := newIDApps(srv.URL)

	out, _, err := manifest.Materialize(context.Background(), ids, "acme", samlManifest(), map[string]string{"subdomain": "acme"})
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if mock.samlPosts != 1 {
		t.Fatalf("POST saml-sps = %d, quiero 1", mock.samlPosts)
	}
	if got := mock.lastSAML["entity_id"]; got != "https://acme.my.salesforce.com" {
		t.Errorf("entity_id resuelto = %v", got)
	}
	if mock.lastSAML["idp_initiated"] != true || mock.lastSAML["enabled"] != true {
		t.Errorf("el SP debe crearse idp_initiated+enabled: %v", mock.lastSAML)
	}
	if out.Config[manifest.KeySSOSPID] != "sp-123" {
		t.Errorf("_sso_sp_id = %q", out.Config[manifest.KeySSOSPID])
	}
	if out.LaunchURL != "https://id.test/saml/idp/initiate/sp-123" {
		t.Errorf("launch = %q (debe ser la URL IdP-initiated)", out.LaunchURL)
	}
	if out.Config[manifest.KeyLaunchURL] != out.LaunchURL {
		t.Errorf("_launch_url debe persistirse")
	}
	if !out.EnqueueSCIM {
		t.Errorf("provisioning=scim debe pedir EnqueueSCIM")
	}
	if out.SAMLMetadataURL == "" {
		t.Errorf("debe exponer la metadata URL del IdP para el admin")
	}
}

func TestMaterialize_SAML_rollbackDeletesSP(t *testing.T) {
	mock := &idMock{}
	srv := mock.server(t)
	defer srv.Close()
	ids := newIDApps(srv.URL)

	_, rollback, err := manifest.Materialize(context.Background(), ids, "acme", samlManifest(), map[string]string{"subdomain": "acme"})
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	// Simula «la persistencia posterior falló»: el llamante revierte lo creado en id.
	if err := rollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if mock.samlDels != 1 {
		t.Fatalf("DELETE saml-sps = %d, quiero 1 (sin huérfanos)", mock.samlDels)
	}
}

func TestMaterialize_SAML_idFailsNoOrphan(t *testing.T) {
	mock := &idMock{failSAML: true}
	srv := mock.server(t)
	defer srv.Close()
	ids := newIDApps(srv.URL)

	out, rollback, err := manifest.Materialize(context.Background(), ids, "acme", samlManifest(), map[string]string{"subdomain": "acme"})
	if err == nil {
		t.Fatalf("Materialize debe fallar si id falla al crear el SP")
	}
	if len(out.Config) != 0 {
		t.Errorf("sin recurso creado, no debe haber claves internas: %v", out.Config)
	}
	// El rollback de un fallo de creación es no-op: nada que borrar (no hay huérfano).
	if err := rollback(context.Background()); err != nil {
		t.Errorf("rollback no-op: %v", err)
	}
	if mock.samlDels != 0 {
		t.Errorf("no debe borrarse nada si no se creó: DELETE=%d", mock.samlDels)
	}
}

func TestMaterialize_SAML_idempotentReactivate(t *testing.T) {
	mock := &idMock{}
	srv := mock.server(t)
	defer srv.Close()
	ids := newIDApps(srv.URL)

	// Config de una activación previa: no debe recrear el SP.
	cfg := map[string]string{
		"subdomain":           "acme",
		manifest.KeySSOSPID:   "sp-123",
		manifest.KeyLaunchURL: "https://id.test/saml/idp/initiate/sp-123",
	}
	out, _, err := manifest.Materialize(context.Background(), ids, "acme", samlManifest(), cfg)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if mock.samlPosts != 0 {
		t.Fatalf("reactivar no debe crear otro SP: POST=%d", mock.samlPosts)
	}
	if out.LaunchURL != "https://id.test/saml/idp/initiate/sp-123" {
		t.Errorf("launch debe reusar el guardado: %q", out.LaunchURL)
	}
}

func TestMaterialize_OIDC_registersClient(t *testing.T) {
	mock := &idMock{}
	srv := mock.server(t)
	defer srv.Close()
	ids := newIDApps(srv.URL)

	out, _, err := manifest.Materialize(context.Background(), ids, "acme", oidcManifest(), map[string]string{"host": "github.acme.com"})
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if mock.dcrTokens != 1 || mock.regPosts != 1 {
		t.Fatalf("OIDC = 1 dcr-token + 1 register, tengo %d/%d", mock.dcrTokens, mock.regPosts)
	}
	uris, _ := mock.lastReg["redirect_uris"].([]any)
	if len(uris) != 1 || uris[0] != "https://github.acme.com/oidc/callback" {
		t.Errorf("redirect_uris resueltos = %v", mock.lastReg["redirect_uris"])
	}
	if out.Config[manifest.KeyOIDCClientID] != "dcr-abc" {
		t.Errorf("_oidc_client_id = %q", out.Config[manifest.KeyOIDCClientID])
	}
	if out.OIDCClientSecret != "s3cr3t" {
		t.Errorf("el secreto OIDC debe devolverse una vez: %q", out.OIDCClientSecret)
	}
	if out.LaunchURL != "https://github.acme.com" {
		t.Errorf("launch por URL directa = %q", out.LaunchURL)
	}
	if out.EnqueueSCIM {
		t.Errorf("sin provisioning no debe pedir sync")
	}
}

func TestMaterialize_missingRequiredField(t *testing.T) {
	mock := &idMock{}
	srv := mock.server(t)
	defer srv.Close()
	ids := newIDApps(srv.URL)

	_, _, err := manifest.Materialize(context.Background(), ids, "acme", samlManifest(), map[string]string{})
	if err == nil {
		t.Fatalf("debe fallar si falta el campo obligatorio")
	}
	if mock.samlPosts != 0 {
		t.Errorf("no debe llamar a id sin los campos: POST=%d", mock.samlPosts)
	}
}

func TestDeactivate(t *testing.T) {
	mock := &idMock{}
	srv := mock.server(t)
	defer srv.Close()
	ids := newIDApps(srv.URL)

	// SAML: borra el SP.
	if err := manifest.Deactivate(context.Background(), ids, "acme", map[string]string{manifest.KeySSOSPID: "sp-123"}); err != nil {
		t.Fatalf("Deactivate SAML: %v", err)
	}
	if mock.samlDels != 1 {
		t.Errorf("Deactivate SAML debe borrar el SP: DELETE=%d", mock.samlDels)
	}
	// OIDC: no hay endpoint de borrado; no llama a nada y no falla.
	if err := manifest.Deactivate(context.Background(), ids, "acme", map[string]string{manifest.KeyOIDCClientID: "dcr-abc"}); err != nil {
		t.Fatalf("Deactivate OIDC: %v", err)
	}
	if mock.samlDels != 1 {
		t.Errorf("Deactivate OIDC no debe tocar SPs: DELETE=%d", mock.samlDels)
	}
}

// --- helpers ---

func assertBearer(t *testing.T, r *http.Request, want string) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer "+want {
		t.Errorf("Authorization = %q, quiero Bearer %s", got, want)
	}
}

func decode(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		t.Fatalf("decodificar cuerpo: %v", err)
	}
	return m
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
