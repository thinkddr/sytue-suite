// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package manifest_test

import (
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// base es un manifiesto SAML válido mínimo; cada caso lo perturba en un punto.
const base = `{
  "name": "X", "description": "d", "color": "#123456", "icon": "<svg/>",
  "identity": { "kind": "saml", "entity_id": "https://{sub}.x.com", "acs_url": "https://{sub}.x.com" },
  "provisioning": "scim",
  "scim_base_url": "https://{sub}.x.com/scim/v2",
  "launch": { "kind": "initiate" },
  "fields": [ { "key": "sub", "label": "Sub", "required": true } ],
  "secret": { "label": "Token SCIM" }
}`

func TestParse_valid(t *testing.T) {
	m, err := manifest.Parse([]byte(base))
	if err != nil {
		t.Fatalf("Parse válido: %v", err)
	}
	if m.Identity.Kind != manifest.IdentitySAML || len(m.RequiredFieldKeys()) != 1 {
		t.Fatalf("manifiesto mal parseado: %+v", m)
	}
}

func TestParse_rejects(t *testing.T) {
	cases := map[string]string{
		"sin name":             `{"description":"d","color":"#111","icon":"<svg/>","identity":{"kind":"none"},"launch":{"kind":"url","url":"https://x"}}`,
		"color no CSS":         `{"name":"X","description":"d","color":"rojo","icon":"<svg/>","identity":{"kind":"none"},"launch":{"kind":"url","url":"https://x"}}`,
		"icon no SVG":          `{"name":"X","description":"d","color":"#111","icon":"http://evil/x.png","identity":{"kind":"none"},"launch":{"kind":"url","url":"https://x"}}`,
		"placeholder colgante": `{"name":"X","description":"d","color":"#111","icon":"<svg/>","identity":{"kind":"none"},"launch":{"kind":"url","url":"https://{nope}"}}`,
		"initiate sin saml":    `{"name":"X","description":"d","color":"#111","icon":"<svg/>","identity":{"kind":"none"},"launch":{"kind":"initiate"}}`,
		"saml sin acs":         `{"name":"X","description":"d","color":"#111","icon":"<svg/>","identity":{"kind":"saml","entity_id":"https://x"},"launch":{"kind":"initiate"}}`,
		"oidc sin redirect":    `{"name":"X","description":"d","color":"#111","icon":"<svg/>","identity":{"kind":"oidc"},"launch":{"kind":"url","url":"https://x"}}`,
		"campo con clave mala": `{"name":"X","description":"d","color":"#111","icon":"<svg/>","identity":{"kind":"none"},"launch":{"kind":"url","url":"https://x"},"fields":[{"key":"1bad","label":"L"}]}`,
		"campo desconocido":    `{"name":"X","description":"d","color":"#111","icon":"<svg/>","identity":{"kind":"none"},"launch":{"kind":"url","url":"https://x"},"bogus":1}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := manifest.Parse([]byte(raw)); err == nil {
				t.Fatalf("%s: se esperaba error", name)
			} else if name != "campo desconocido" && !errors.Is(err, manifest.ErrInvalid) {
				t.Fatalf("%s: error = %v, quiero ErrInvalid", name, err)
			}
		})
	}
}

func TestParse_maxThreeFields(t *testing.T) {
	raw := `{"name":"X","description":"d","color":"#111","icon":"<svg/>","identity":{"kind":"none"},"launch":{"kind":"url","url":"https://x"},
	 "fields":[{"key":"a","label":"A"},{"key":"b","label":"B"},{"key":"c","label":"C"},{"key":"d","label":"D"}]}`
	if _, err := manifest.Parse([]byte(raw)); err == nil || !strings.Contains(err.Error(), "máximo") {
		t.Fatalf("4 campos deben rechazarse: %v", err)
	}
}

// TestParse_scimNeedsBaseURLAndSecret: declarar aprovisionamiento SCIM sin decir CONTRA
// QUÉ (raíz SCIM) o SIN CON QUÉ (secreto) es lo que hacía que el catálogo prometiera
// altas y bajas automáticas que morían en el outbox. Ahora la semilla no carga.
func TestParse_scimNeedsBaseURLAndSecret(t *testing.T) {
	sinBase := strings.Replace(base, `"scim_base_url": "https://{sub}.x.com/scim/v2",`, "", 1)
	if _, err := manifest.Parse([]byte(sinBase)); err == nil {
		t.Error("provisioning scim sin scim_base_url debería ser inválido")
	}
	sinSecreto := strings.Replace(base, `,
  "secret": { "label": "Token SCIM" }`, "", 1)
	if _, err := manifest.Parse([]byte(sinSecreto)); err == nil {
		t.Error("provisioning scim sin secret NI oauth debería ser inválido")
	}
	// Pero con consentimiento SÍ vale sin secreto: el token de aprovisionamiento sale
	// del canje del código, no del portapapeles del admin. Es el camino preferente.
	conOAuth := strings.Replace(sinSecreto, `"launch": { "kind": "initiate" }`,
		`"launch": { "kind": "initiate" },
  "oauth": { "authorize_url": "https://{sub}.x.com/authorize", "token_url": "https://{sub}.x.com/token", "scopes": ["scim:write"] }`, 1)
	if _, err := manifest.Parse([]byte(conOAuth)); err != nil {
		t.Errorf("provisioning scim con oauth y sin secret debe valer: %v", err)
	}
	conPlaceholderColgante := strings.Replace(base,
		`"scim_base_url": "https://{sub}.x.com/scim/v2"`,
		`"scim_base_url": "https://{fantasma}.x.com/scim/v2"`, 1)
	if _, err := manifest.Parse([]byte(conPlaceholderColgante)); err == nil {
		t.Error("scim_base_url con {campo} inexistente debería ser inválido")
	}
}

// TestParse_oauthConsent cubre el flujo de consentimiento, que es el camino POR DEFECTO
// del producto para conectar un tercero (pegar un token es el último recurso). Lo que se
// exige es lo que hace falta para poder ejecutarlo y para poder CONTÁRSELO al admin antes
// de redirigirle: las dos URLs, en https, y los scopes que se van a pedir.
func TestParse_oauthConsent(t *testing.T) {
	con := func(oauth string) string {
		return strings.Replace(base, `"secret": { "label": "Token SCIM" }`, `"oauth": `+oauth, 1)
	}
	ok := `{ "authorize_url": "https://{sub}.x.com/authorize", "token_url": "https://x.com/token",
	          "scopes": ["users:read","users:write"], "write_scopes": ["files:write"], "revoke_url": "https://x.com/revoke" }`
	m, err := manifest.Parse([]byte(con(ok)))
	if err != nil {
		t.Fatalf("manifiesto con oauth válido: %v", err)
	}
	if m.OAuth == nil || len(m.OAuth.Scopes) != 2 || m.OAuth.RevokeURL == "" || len(m.OAuth.WriteScopes) != 1 {
		t.Fatalf("oauth mal parseado: %+v", m.OAuth)
	}

	malos := map[string]string{
		"sin authorize_url":    `{ "token_url": "https://x.com/token", "scopes": ["a"] }`,
		"sin token_url":        `{ "authorize_url": "https://x.com/a", "scopes": ["a"] }`,
		"sin scopes":           `{ "authorize_url": "https://x.com/a", "token_url": "https://x.com/token" }`,
		"authorize en claro":   `{ "authorize_url": "http://x.com/a", "token_url": "https://x.com/token", "scopes": ["a"] }`,
		"token en claro":       `{ "authorize_url": "https://x.com/a", "token_url": "http://x.com/token", "scopes": ["a"] }`,
		"revoke en claro":      `{ "authorize_url": "https://x.com/a", "token_url": "https://x.com/t", "scopes": ["a"], "revoke_url": "http://x.com/r" }`,
		"placeholder colgante": `{ "authorize_url": "https://{fantasma}.x.com/a", "token_url": "https://x.com/t", "scopes": ["a"] }`,
		"write_scope vacío":    `{ "authorize_url": "https://x.com/a", "token_url": "https://x.com/t", "scopes": ["a"], "write_scopes": [" "] }`,
	}
	for nombre, oauth := range malos {
		t.Run(nombre, func(t *testing.T) {
			if _, err := manifest.Parse([]byte(con(oauth))); err == nil {
				t.Fatalf("%s: se esperaba error", nombre)
			} else if !errors.Is(err, manifest.ErrInvalid) {
				t.Fatalf("%s: error = %v, quiero ErrInvalid", nombre, err)
			}
		})
	}
}

// TestDriverFor_perCapability: cada capacidad la sirve el driver que nombra el
// manifiesto, y son distintos. Es lo que permite que una misma conexión aprovisione con
// SCIM y traiga datos con el driver del SaaS, cada uno con su credencial.
func TestDriverFor_perCapability(t *testing.T) {
	conDatos := strings.Replace(base, `"provisioning": "scim",`, `"provisioning": "scim", "data": "jira",`, 1)
	m, err := manifest.Parse([]byte(conDatos))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := m.DriverFor(manifest.CapProvision); got != "scim" {
		t.Errorf("driver de aprovisionamiento = %q, quiero \"scim\"", got)
	}
	if got := m.DriverFor(manifest.CapData); got != "jira" {
		t.Errorf("driver de datos = %q, quiero \"jira\"", got)
	}
	if got := len(m.Capabilities()); got != 2 {
		t.Errorf("capacidades = %d, quiero 2", got)
	}

	cfg, err := m.CapabilityConfig(manifest.CapProvision, map[string]string{"sub": "acme"})
	if err != nil {
		t.Fatalf("CapabilityConfig: %v", err)
	}
	if cfg[manifest.ConfigSCIMBaseURL] != "https://acme.x.com/scim/v2" {
		t.Errorf("raíz SCIM = %q; debe resolver los {campo} con la config de la instancia", cfg[manifest.ConfigSCIMBaseURL])
	}
}

// TestParse_substitutes cubre la libertad de elección: qué producto de la suite dice una
// plantilla poder REEMPLAZAR. Lo que se valida es lo que evita prometer de más.
func TestParse_substitutes(t *testing.T) {
	// Se AÑADE al manifiesto base sin quitarle el secreto: la sustitución es ortogonal
	// a cómo se autentica la conexión.
	con := func(subs string) string {
		return strings.Replace(base, `"secret": { "label": "Token SCIM" }`,
			`"secret": { "label": "Token SCIM" }, "substitutes": `+subs, 1)
	}
	m, err := manifest.Parse([]byte(con(
		`[{"product":"sheets","open_url":"https://{sub}.x.com/sheets",
		   "document_url":"https://{sub}.x.com/d/{document_id}"}]`,
	)))
	if err != nil {
		t.Fatalf("sustitución válida: %v", err)
	}
	if s := m.SubstitutionFor("sheets"); s == nil || s.DocumentURL == "" {
		t.Fatalf("no se ha parseado la sustitución: %+v", m.Substitutes)
	}
	if m.SubstitutionFor("mail") != nil {
		t.Fatal("no debe decir que sustituye a mail")
	}

	malos := map[string]string{
		// Una errata haría que la sustitución no se ofreciera nunca y sin explicación.
		"producto inexistente": `[{"product":"sheet"}]`,
		"duplicado":            `[{"product":"docs"},{"product":"docs"}]`,
		"placeholder colgante": `[{"product":"docs","open_url":"https://{fantasma}.x.com"}]`,
		// Sin {document_id} el «abrir este fichero» llevaría a la portada del tercero.
		"document_url sin id": `[{"product":"docs","document_url":"https://x.com/docs"}]`,
	}
	for nombre, subs := range malos {
		t.Run(nombre, func(t *testing.T) {
			if _, err := manifest.Parse([]byte(con(subs))); err == nil {
				t.Fatalf("%s: se esperaba error", nombre)
			} else if !errors.Is(err, manifest.ErrInvalid) {
				t.Fatalf("%s: error = %v, quiero ErrInvalid", nombre, err)
			}
		})
	}
}

// TestRenderOAuth_exigeHTTPSResuelto: hay proveedores cuyo endpoint ES la raíz del
// workspace que teclea el admin (Databricks), así que el manifiesto no puede exigir el
// literal «https://» al principio. La garantía se comprueba sobre la URL YA RESUELTA,
// que es el único momento en que se sabe lo que se va a llamar de verdad. Sin esto, un
// admin que escribiera «http://…» mandaría el código de autorización en claro.
func TestRenderOAuth_exigeHTTPSResuelto(t *testing.T) {
	raw := strings.Replace(base, `"secret": { "label": "Token SCIM" }`,
		`"oauth": { "authorize_url": "{sub}/oauth/authorize", "token_url": "{sub}/oauth/token", "scopes": ["a"] }`, 1)
	m, err := manifest.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("una URL que empieza por {campo} debe aceptarse en el manifiesto: %v", err)
	}
	if _, err := m.RenderOAuth(map[string]string{"sub": "https://acme.example"}); err != nil {
		t.Fatalf("resuelta a https debe valer: %v", err)
	}
	if _, err := m.RenderOAuth(map[string]string{"sub": "http://acme.example"}); err == nil {
		t.Fatal("resuelta a http DEBE rechazarse: por ahí viaja el código de autorización")
	} else if !errors.Is(err, manifest.ErrInvalid) {
		t.Fatalf("err = %v, quiero ErrInvalid", err)
	}
}
