// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// Una plantilla con un campo que es una URL (la raíz de un OData) y consentimiento
// con el host fijo.
const manifiestoDePrueba = `{
  "name": "X", "description": "d", "color": "#123456", "icon": "<svg/>",
  "identity": { "kind": "none" },
  "launch": { "kind": "none" },
  "data": "odata",
  "fields": [ { "key": "service_url", "label": "URL", "required": true } ],
  "oauth": { "authorize_url": "https://login.proveedor.com/authorize", "token_url": "https://login.proveedor.com/token", "scopes": ["leer"] }
}`

func manifiesto(t *testing.T) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Parse([]byte(manifiestoDePrueba))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// NEG-07: las raíces de API alternativas de los drivers no se pueden poner por la API.
// Solo un campo que la plantilla declara dice adónde se sale.
func TestSoloLosCamposDeclaradosLlevanUnaURL(t *testing.T) {
	m := manifiesto(t)
	for _, clave := range []string{"graph_base", "authority_base", "google_drive_base", "github_api_base", "token_url", "base_url"} {
		cfg, _ := json.Marshal(map[string]string{"service_url": "https://erp.acme.com/odata", clave: "https://evil.example"})
		if err := soloDestinosDeclarados(m, cfg); err == nil {
			t.Errorf("la config aceptó una URL en %q, que la plantilla no declara", clave)
		}
	}
	// Hay drivers que aceptan el servidor sin esquema: el nombre de la clave lo delata.
	sinEsquema, _ := json.Marshal(map[string]string{"service_url": "https://erp.acme.com/odata", "instance_url": "evil.example"})
	if err := soloDestinosDeclarados(m, sinEsquema); err == nil {
		t.Error("la config aceptó un servidor sin esquema en \"instance_url\", que la plantilla no declara")
	}
	bien, _ := json.Marshal(map[string]string{
		"service_url":      "https://erp.acme.com/odata", // declarado
		"_launch_url":      "https://erp.acme.com",       // clave interna de la pasarela
		"_oauth_client_id": "abc",
		"gcp_project":      "acme-prod", // no declarado, pero no es una dirección
	})
	if err := soloDestinosDeclarados(m, bien); err != nil {
		t.Errorf("una config legítima se rechazó: %v", err)
	}
}

// NEG-07: cambiar el servidor de una conexión invalida sus credenciales; cambiar lo
// que no dice adónde se sale, no.
func TestCambiarDeServidorInvalidaLasCredenciales(t *testing.T) {
	m := manifiesto(t)
	cfg := func(kv map[string]string) json.RawMessage { b, _ := json.Marshal(kv); return b }
	antes := cfg(map[string]string{"service_url": "https://erp.acme.com/odata"})

	if !cambiaDeServidor(m, antes, cfg(map[string]string{"service_url": "https://evil.example/odata"})) {
		t.Error("apuntar la conexión a otro servidor no se detecta")
	}
	for nombre, despues := range map[string]json.RawMessage{
		"otra ruta en el mismo servidor": cfg(map[string]string{"service_url": "https://erp.acme.com/odata/v4"}),
		"una clave interna":              cfg(map[string]string{"service_url": "https://erp.acme.com/odata", "_oauth_expires_at": "2026-10-06T12:00:00Z"}),
		"un dato que no es dirección":    cfg(map[string]string{"service_url": "https://erp.acme.com/odata", "gcp_project": "acme"}),
	} {
		if cambiaDeServidor(m, antes, despues) {
			t.Errorf("%s: se tomó por un cambio de servidor", nombre)
		}
	}
}

// NEG-09: el secreto de la app que Sytue registra ante un proveedor es de todas las
// organizaciones. Solo sale hacia una URL cuyo host escribe el manifiesto.
func TestLaCredencialDePlataformaSoloVaAUnHostFijo(t *testing.T) {
	plataforma := map[string]struct{ ClientID, ClientSecret string }{}
	for _, k := range []string{"salesforce", "servicenow", "databricks", "github", "google-workspace"} {
		plataforma[k] = struct{ ClientID, ClientSecret string }{"id-de-sytue", "secreto-de-sytue"}
	}
	h := &Handlers{oauthClients: newOAuthClientResolver(&loaderFalso{datos: plataforma}, nil)}
	ctx := context.Background()

	for plantilla, destino := range map[string]string{
		"databricks": "{workspace_url}/oidc/v1/token",           // la URL entera es de la instancia
		"github":     "https://{host}/login/oauth/access_token", // un GitHub Enterprise propio
	} {
		if c := h.clienteDePlataforma(ctx, plantilla, destino); c.ClientID != "" || c.ClientSecret != "" {
			t.Errorf("%s: la credencial de plataforma saldría hacia %q, que elige la instancia", plantilla, destino)
		}
	}
	for plantilla, destino := range map[string]string{
		"google-workspace": "https://oauth2.googleapis.com/token",
		"salesforce":       "https://{subdomain}.my.salesforce.com/services/oauth2/token",
		"servicenow":       "https://{instance}.service-now.com/oauth_token.do",
	} {
		if c := h.clienteDePlataforma(ctx, plantilla, destino); c.ClientSecret != "secreto-de-sytue" {
			t.Errorf("%s: con el host fijo (%q) tiene que usarse la app de Sytue", plantilla, destino)
		}
	}
}
