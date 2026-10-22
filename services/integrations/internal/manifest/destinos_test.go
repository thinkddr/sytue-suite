// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package manifest_test

import (
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// Un campo que cae en el nombre del servidor no puede sacar la URL del dominio que
// escribe el manifiesto. Con sustitución a pelo, «evil.example/x?» en {sub} dejaba
// «https://evil.example/x?.proveedor.com/token»: el host era evil.example.
func TestElCampoDelHostNoSaleDelDominio(t *testing.T) {
	const tmpl = "https://{sub}.proveedor.com/oauth/token"
	for _, hostil := range []string{"evil.example/x?", "evil.example#", "a@evil.example", "evil.example:443", "evil.example\\", " evil"} {
		out, err := manifest.RenderURL(tmpl, map[string]string{"sub": hostil})
		if err == nil {
			t.Errorf("{sub}=%q se aceptó: %q", hostil, out)
		} else if !errors.Is(err, manifest.ErrInvalid) {
			t.Errorf("{sub}=%q: %v, quiero ErrInvalid", hostil, err)
		}
	}
	for _, bueno := range []string{"acme", "acme-eu", "acme.sandbox"} {
		out, err := manifest.RenderURL(tmpl, map[string]string{"sub": bueno})
		if err != nil {
			t.Fatalf("{sub}=%q: %v", bueno, err)
		}
		if u, _ := url.Parse(out); !strings.HasSuffix(u.Host, ".proveedor.com") {
			t.Errorf("{sub}=%q deja el host en %q", bueno, u.Host)
		}
	}
	// El campo que CIERRA el host admite puerto (un servidor propio en otro puerto);
	// lo que va en la ruta no se toca.
	if out, err := manifest.RenderURL("https://{host}/saml", map[string]string{"host": "git.acme.com:8443"}); err != nil || out != "https://git.acme.com:8443/saml" {
		t.Errorf("host con puerto: %q %v", out, err)
	}
	if _, err := manifest.RenderURL("https://{host}/saml", map[string]string{"host": "git.acme.com/x"}); err == nil {
		t.Error("un host con ruta se aceptó")
	}
	if out, err := manifest.RenderURL("https://git.example/{grupo}/x", map[string]string{"grupo": "a/b"}); err != nil || out != "https://git.example/a/b/x" {
		t.Errorf("campo en la ruta: %q %v", out, err)
	}
	// Quien aporta la URL entera la aporta entera: ahí no hay host que proteger.
	if out, err := manifest.RenderURL("{raiz}/oidc/v1/token", map[string]string{"raiz": "https://dbc-1.example.com"}); err != nil || out != "https://dbc-1.example.com/oidc/v1/token" {
		t.Errorf("url entera: %q %v", out, err)
	}
}

func TestHostFijo(t *testing.T) {
	casos := map[string]bool{
		"https://accounts.google.com/o/oauth2/token":                   true,
		"https://login.microsoftonline.com/{tenant}/oauth2/v2.0/token": true,
		"https://{subdomain}.my.salesforce.com/services/oauth2/token":  true,
		"https://{instance}.service-now.com/oauth_token.do":            true,
		"{workspace_url}/oidc/v1/token":                                false,
		"https://{host}/login/oauth/access_token":                      false,
		"https://{domain}":                 false,
		"https://{a}.{b}/token":            false,
		"https://{sub}.com/token":          false,
		"http://accounts.google.com/token": false,
		"":                                 false,
	}
	for tmpl, quiero := range casos {
		if tengo := manifest.HostFijo(tmpl); tengo != quiero {
			t.Errorf("HostFijo(%q) = %v, quiero %v", tmpl, tengo, quiero)
		}
	}
}

func TestDestinos(t *testing.T) {
	m, err := manifest.Parse([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	antes := m.Destinos(map[string]string{"sub": "acme"})
	if !slices.Equal(antes, []string{"acme.x.com"}) {
		t.Fatalf("destinos = %v", antes)
	}
	if slices.Equal(antes, m.Destinos(map[string]string{"sub": "otra"})) {
		t.Error("cambiar el campo del host no cambia los destinos")
	}
	// Lo que no dice adónde se sale no cuenta: claves internas y valores que no son URL.
	mismo := m.Destinos(map[string]string{"sub": "acme", "_launch_url": "https://portal.example/x", "nota": "hola"})
	if !slices.Equal(antes, mismo) {
		t.Errorf("destinos = %v, quiero %v", mismo, antes)
	}
	// Una URL en cualquier otra clave sí es un destino.
	con := m.Destinos(map[string]string{"sub": "acme", "graph_base": "https://Evil.Example/x"})
	if !slices.Equal(con, []string{"acme.x.com", "evil.example"}) {
		t.Errorf("destinos = %v", con)
	}
	if !m.Declara("sub") || m.Declara("graph_base") {
		t.Error("Declara no distingue el campo de la plantilla")
	}
}
