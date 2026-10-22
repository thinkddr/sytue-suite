// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package seeds_test

import (
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/db/seeds"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/drivers"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// TestLoad_validatesCatalog carga todas las semillas embebidas y afirma que cada
// manifiesto cumple el esquema y declara la identidad/aprovisionamiento/tier esperados.
// Es la red de seguridad de que un fichero de catálogo mal escrito no llega al arranque:
// tier1 = las plantillas insignia (W3, algunas con driver de datos propio); tier2 = el
// catálogo ampliado (OLA 9) que reusa la maquinaria genérica SAML/OIDC + SCIM 2.0.
func TestLoad_validatesCatalog(t *testing.T) {
	list, err := seeds.Load()
	if err != nil {
		t.Fatalf("seeds.Load: %v", err)
	}
	byKey := map[string]manifest.Manifest{}
	tierByKey := map[string]string{}
	for _, s := range list {
		m, err := manifest.Parse(s.Manifest) // re-valida (Load ya validó; esto documenta el contrato).
		if err != nil {
			t.Fatalf("semilla %q inválida: %v", s.Key, err)
		}
		tierByKey[s.Key] = s.Tier
		byKey[s.Key] = *m
	}

	want := map[string]struct {
		identity     string
		provisioning string
		data         string
		tier         string
	}{
		"salesforce":        {manifest.IdentitySAML, manifest.ProvisioningNone, "salesforce", "tier1"},
		"jira-cloud":        {manifest.IdentitySAML, manifest.ProvisioningNone, "jira", "tier1"},
		"github-enterprise": {manifest.IdentityOIDC, manifest.ProvisioningNone, "", "tier1"},
		"odoo":              {manifest.IdentityOIDC, manifest.ProvisioningNone, "odoo", "tier1"},

		// Conectores de solo DATOS: sin identidad (el login lo gobierna el IdP del
		// proveedor) y con su driver propio. Existían compilados y sin plantilla, es
		// decir, inalcanzables: no había forma de crear una instancia de ninguno.
		"databricks": {manifest.IdentityNone, manifest.ProvisioningNone, "databricks", "tier1"},
		"powerbi":    {manifest.IdentityNone, manifest.ProvisioningNone, "powerbi", "tier1"},
		"odata":      {manifest.IdentityNone, manifest.ProvisioningNone, "odata", "tier1"},
		// ERP de pymes que Sytue ERP importa: Holded con su driver, Business Central
		// por el OData de siempre con el consentimiento de Microsoft.
		"holded":           {manifest.IdentityNone, manifest.ProvisioningNone, "holded", "tier2"},
		"business-central": {manifest.IdentityNone, manifest.ProvisioningNone, "odata", "tier2"},
		// La PROPIA suite como origen: el MISMO driver `graph` apuntado a los endpoints
		// /compat de Sytue Drive. Ya tiene plantilla porque ya existe la pieza que
		// faltaba —la inyección de configuración del SERVICIO al despachar
		// (integrationshttp.loadInstance)—; sin ella creaba instancias que no funcionaban.
		// El admin solo aporta la persona: el destino y el client delegable los pone el
		// servicio, que es lo que permite tratar el destino como interno.
		"sytue": {manifest.IdentityNone, manifest.ProvisioningNone, "graph", "tier1"},

		// OLA 9 — catálogo ampliado (SAML). El aprovisionamiento SCIM quedó en "none" en casi
		// todas: declararlo exige una raíz SCIM 2.0 verificada contra la documentación del
		// fabricante (ver el validador del manifiesto), y prometerlo sin ella es lo que hacía
		// que «Sincronizar» fallara siempre. Se rehabilitan de una en una, con su URL.
		"slack":                     {manifest.IdentitySAML, manifest.ProvisioningSCIM, "", "tier2"},
		"zoom":                      {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"snowflake":                 {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"aws-iam-identity-center":   {manifest.IdentitySAML, manifest.ProvisioningSCIM, "", "tier2"},
		"tableau":                   {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"confluence":                {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"notion":                    {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"servicenow":                {manifest.IdentitySAML, manifest.ProvisioningNone, "servicenow", "tier2"},
		"box":                       {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"gmail-mail":                {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"microsoft-mail":            {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"yahoo-mail":                {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"mail-imap":                 {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"mail-pop3":                 {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"google-drive":              {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"onedrive":                  {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"dropbox-storage":           {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"slack-rooms":               {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"slack-chat":                {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"microsoft-teams-chat":      {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"google-chat-conversations": {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"microsoft-teams-rooms":     {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"google-chat":               {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"google-meet":               {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"dropbox":                   {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"asana":                     {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"miro":                      {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		// OLA 6 — datos de las suites de productividad (Directory/Graph) en Sheets/Hub.
		// Google Workspace: SSO SAML de terceros ✓ + datos del Directory (driver gws);
		// aprovisionamiento saliente (Directory API, no SCIM) → ola posterior.
		"google-workspace": {manifest.IdentitySAML, manifest.ProvisioningNone, "gws", "tier2"},
		// Microsoft 365: datos vía Graph (driver graph). Identity none: Entra es el IdP
		// de la organización (upstream), Sytue no actúa aquí de IdP.
		"microsoft365": {manifest.IdentityNone, manifest.ProvisioningNone, "graph", "tier2"},
		// Libertad de elección: sustituyen a un producto de la suite. Identity none —el
		// IdP de la organización es Entra, no Sytue— y sin capacidad saliente todavía:
		// hoy la sustitución significa que el lanzador abre el sustituto (ver
		// handlers_substitution.go), no que la suite lea su dato.
		"microsoft-teams": {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier1"},
		// GitHub.com como ORIGEN de migración: se conecta por consentimiento y el
		// asistente trae sus repositorios a Sytue Git y sus gists a Drive. Identity
		// none: aquí Sytue no actúa de IdP de github.com (eso es github-enterprise).
		"github": {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier1"},
		"azure":  {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier1"},

		// OLA 10 — el catálogo llega a los SaaS que más se piden fuera de la ingeniería.
		// Todas con provisioning "none" por la misma regla de arriba: sin raíz SCIM 2.0
		// verificada no se promete «Sincronizar».
		//
		// Okta va por OIDC y no por SAML porque aquí Sytue ID es el proveedor de
		// identidad DE ARRIBA de una organización de Okta, y eso Okta lo consume como
		// «external identity provider» OIDC, con una URL de vuelta que sí es pública y
		// estable (https://DOMINIO/oauth2/v1/authorize/callback).
		//
		// Trello va con identity "none" a propósito: su SSO no se configura por
		// producto sino para toda la organización de Atlassian, así que una plantilla
		// SAML propia prometería una configuración que no existe. Con Jira Cloud o
		// Confluence conectados, Trello ya entra por el mismo SSO.
		"okta":     {manifest.IdentityOIDC, manifest.ProvisioningNone, "", "tier2"},
		"zendesk":  {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"hubspot":  {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"docusign": {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"trello":   {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
		"airtable": {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		"monday":   {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},

		// La plantilla GENÉRICA: cualquier servicio con SAML 2.0 entra con Sytue ID
		// pegando su Entity ID y su ACS, sin esperar a que exista su ficha. Es lo que
		// convierte «Sytue ID como cuenta de la organización» en algo que vale para
		// el SaaS número doscientos y no solo para los treinta del catálogo.
		"saml-generica": {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		// GitLab.com por grupo (SAML SSO for groups). NameID persistente: GitLab liga
		// la identidad al valor y un correo que cambia dejaría a la persona fuera.
		"gitlab": {manifest.IdentitySAML, manifest.ProvisioningNone, "", "tier2"},
		// WhatsApp Business: canal de mensajería externa para Rooms.
		"whatsapp-business": {manifest.IdentityNone, manifest.ProvisioningNone, "", "tier2"},
	}
	if len(byKey) != len(want) {
		t.Fatalf("claves cargadas = %v, quiero %d semillas", keysOf(byKey), len(want))
	}
	for key, exp := range want {
		m, ok := byKey[key]
		if !ok {
			t.Fatalf("falta la semilla %q", key)
		}
		if m.Identity.Kind != exp.identity {
			t.Errorf("%q: identity = %q, quiero %q", key, m.Identity.Kind, exp.identity)
		}
		if got := normalizedProvisioning(m); got != exp.provisioning {
			t.Errorf("%q: provisioning = %q, quiero %q", key, got, exp.provisioning)
		}
		if m.Data != exp.data {
			t.Errorf("%q: data = %q, quiero %q", key, m.Data, exp.data)
		}
		if tierByKey[key] != exp.tier {
			t.Errorf("%q: tier = %q, quiero %q", key, tierByKey[key], exp.tier)
		}
	}
}

// normalizedProvisioning refleja el default "" ⇒ "none" del manifiesto.
func normalizedProvisioning(m manifest.Manifest) string {
	if m.Provisioning == "" {
		return manifest.ProvisioningNone
	}
	return m.Provisioning
}

func keysOf(m map[string]manifest.Manifest) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestLoad_everyDeclaredCapabilityResolvesToADriver es la comprobación que faltaba y que
// habría evitado el defecto de fondo: una plantilla podía declarar `provisioning: scim`
// o `data: loquesea` sin que existiera ningún driver que lo sirviera. El catálogo
// prometía altas y bajas automáticas en 14 aplicaciones y no funcionaban en NINGUNA
// —el botón «Sincronizar» devolvía 400— porque el despacho buscaba el driver por la
// clave de la plantilla en vez de por la capacidad.
//
// Se comprueba contra el registro REAL del servicio (internal/drivers), no contra una
// lista copiada aquí: si diverge, diverge para todos.
func TestLoad_everyDeclaredCapabilityResolvesToADriver(t *testing.T) {
	list, err := seeds.Load()
	if err != nil {
		t.Fatalf("seeds.Load: %v", err)
	}
	reg := drivers.Registry(nil, nil)

	for _, s := range list {
		m, err := manifest.Parse(s.Manifest)
		if err != nil {
			t.Fatalf("semilla %q inválida: %v", s.Key, err)
		}
		for _, capab := range m.Capabilities() {
			kind := m.DriverFor(capab)
			drv, ok := reg.Get(kind)
			if !ok {
				t.Errorf("%q declara la capacidad %q servida por el driver %q, que no está registrado: "+
					"el catálogo promete algo que el servicio no puede hacer", s.Key, capab, kind)
				continue
			}
			switch capab {
			case manifest.CapProvision:
				if _, ok := drv.(connector.Provisioner); !ok {
					t.Errorf("%q declara aprovisionamiento con el driver %q, que no implementa Provisioner", s.Key, kind)
				}
			case manifest.CapData:
				if _, ok := drv.(connector.TabularSource); !ok {
					t.Errorf("%q declara datos con el driver %q, que no implementa TabularSource", s.Key, kind)
				}
			}
		}
	}
}

// TestLoad_laCredencialDePlataformaNoSaleDelProveedor (NEG-09): en cada plantilla del
// catálogo con consentimiento, o el host del token_url y del revoke_url lo fija el
// manifiesto —y entonces ningún valor de un campo consigue sacar la URL de ahí—, o no
// es fijo y la app de plataforma no se usa (manifest.HostFijo, que es lo que mira la
// capa HTTP). Con sustitución a pelo, «evil.example/x?» en el subdominio de Salesforce
// mandaba el secreto de la app de Sytue a evil.example.
func TestLoad_laCredencialDePlataformaNoSaleDelProveedor(t *testing.T) {
	semillas, err := seeds.Load()
	if err != nil {
		t.Fatal(err)
	}
	hostiles := []string{"evil.example/x?", "evil.example#", "a@evil.example", "evil.example:443/x", "evil.example"}
	fijos := map[string]bool{}
	for _, s := range semillas {
		m, err := manifest.Parse(s.Manifest)
		if err != nil || m.OAuth == nil {
			continue
		}
		fijos[s.Key] = manifest.HostFijo(m.OAuth.TokenURL)
		for _, tmpl := range []string{m.OAuth.TokenURL, m.OAuth.RevokeURL} {
			if tmpl == "" || !manifest.HostFijo(tmpl) {
				continue
			}
			// El dominio que escribe el manifiesto: lo que queda del host tras el último campo.
			host := strings.SplitN(strings.TrimPrefix(tmpl, "https://"), "/", 2)[0]
			sufijo := host[strings.LastIndex(host, "}")+1:]
			for _, hostil := range hostiles {
				valores := map[string]string{}
				for _, f := range m.Fields {
					valores[f.Key] = hostil
				}
				out, err := manifest.RenderURL(tmpl, valores)
				if err != nil {
					continue // rechazado: bien.
				}
				u, perr := url.Parse(out)
				if perr != nil || !strings.HasSuffix(u.Host, sufijo) {
					t.Errorf("%s: con %q en sus campos, %q resuelve a %q, fuera de %q", s.Key, hostil, tmpl, out, sufijo)
				}
			}
		}
	}
	// Las que cuelgan de un servidor que elige la instancia no usan la app de plataforma;
	// las de proveedor único, sí.
	for clave, quiero := range map[string]bool{
		"databricks": false, "salesforce": true, "servicenow": true,
		"google-workspace": true, "microsoft365": true, "github": true,
	} {
		if tengo, hay := fijos[clave]; !hay || tengo != quiero {
			t.Errorf("%s: HostFijo(token_url) = %v (en el catálogo: %v), quiero %v", clave, tengo, hay, quiero)
		}
	}
}

// El almacenamiento usa consentimiento de archivos con renovación, sin sustituir
// la plantilla SAML de Dropbox Business que ya pueden tener otras organizaciones.
func TestDropboxStoragePideConsentimientoRenovable(t *testing.T) {
	list, err := seeds.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, seed := range list {
		if seed.Key != "dropbox-storage" {
			continue
		}
		m, err := manifest.Parse(seed.Manifest)
		if err != nil {
			t.Fatal(err)
		}
		if m.OAuth == nil || m.OAuth.AuthorizeParams["token_access_type"] != "offline" ||
			m.OAuth.TokenURL != "https://api.dropboxapi.com/oauth2/token" ||
			strings.Join(m.OAuth.Scopes, " ") != "files.metadata.read files.content.read files.content.write" {
			t.Fatalf("Dropbox debe pedir solo acceso a archivos y un refresh token: %+v", m.OAuth)
		}
		return
	}
	t.Fatal("falta el conector OAuth de almacenamiento Dropbox")
}

func TestRoomsPideSoloLosPermisosQueConsume(t *testing.T) {
	list, err := seeds.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"slack-rooms":           "incoming-webhook",
		"microsoft-teams-rooms": "offline_access User.Read Team.ReadBasic.All Channel.ReadBasic.All ChannelMessage.Send",
		"microsoft-teams-chat":  "offline_access User.Read Team.ReadBasic.All Channel.ReadBasic.All ChannelMessage.Send Chat.ReadWrite",
		"google-chat":           "https://www.googleapis.com/auth/chat.spaces.readonly https://www.googleapis.com/auth/chat.messages.create",
		"google-meet":           "https://www.googleapis.com/auth/meetings.space.created",
	}
	for _, seed := range list {
		scopes, ok := want[seed.Key]
		if !ok {
			continue
		}
		m, err := manifest.Parse(seed.Manifest)
		if err != nil {
			t.Fatal(err)
		}
		if m.OAuth == nil || strings.Join(m.OAuth.Scopes, " ") != scopes || m.Identity.Kind != manifest.IdentityNone || normalizedProvisioning(*m) != manifest.ProvisioningNone || len(m.Fields) != 0 {
			t.Fatalf("%s mezcla OAuth de Rooms con SAML, SCIM o datos ajenos: %+v", seed.Key, m)
		}
		delete(want, seed.Key)
	}
	if len(want) > 0 {
		t.Fatalf("faltan plantillas de Rooms: %v", want)
	}
}
