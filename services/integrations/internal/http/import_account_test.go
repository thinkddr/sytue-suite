// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// TestImportAccountFromProvider: la cuenta sale de lo que dice el proveedor con el token
// recién concedido, y es la misma clave que daría el formulario con el dominio tecleado.
func TestImportAccountFromProvider(t *testing.T) {
	for _, c := range []struct {
		nombre, plantilla, url, cuerpo string
		clave, rotulo                  string
	}{
		{
			"workspace", "google-workspace", googleUserinfoURL,
			`{"sub":"1","email":"Admin@Acme.com","hd":"Acme.com"}`, "google:acme.com", "acme.com",
		},
		{
			"cuenta personal", "google-workspace", googleUserinfoURL,
			`{"sub":"1","email":"ana@gmail.com"}`, "google:ana@gmail.com", "ana@gmail.com",
		},
		{
			"inquilino", "microsoft365", m365OrgURL,
			`{"value":[{"id":"7f0c…","verifiedDomains":[{"name":"contoso.com","isDefault":true},{"name":"Contoso.onmicrosoft.com","isInitial":true}]}]}`,
			"m365:contoso.onmicrosoft.com", "contoso.com",
		},
		{
			"inquilino sin dominios", "microsoft365", m365OrgURL,
			`{"value":[{"id":"7F0C-ID","verifiedDomains":[]}]}`, "m365:7f0c-id", "7f0c-id",
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != c.url {
					t.Fatalf("preguntó a %s, quiero %s", r.URL, c.url)
				}
				return roomsTestResponse(http.StatusOK, c.cuerpo), nil
			})
			clave, rotulo, err := importAccountFromProvider(context.Background(), rt, c.plantilla)
			if err != nil || clave != c.clave || rotulo != c.rotulo {
				t.Fatalf("= %q, %q, %v; quiero %q, %q", clave, rotulo, err, c.clave, c.rotulo)
			}
		})
	}

	// Lo que no es un origen no pregunta nada a nadie.
	clave, _, err := importAccountFromProvider(context.Background(), roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("no debía preguntar al proveedor por %s", r.URL)
		return nil, errors.New("inalcanzable")
	}), "google-drive")
	if err != nil || clave != "" {
		t.Fatalf("google-drive no es un origen: %q, %v", clave, err)
	}
}

// TestApplyImportAccountNoTumbaElConsentimiento: si el proveedor no dice de quién es la
// cuenta, la conexión se guarda igual, sin cuenta; saberla es una mejora, no un requisito.
func TestApplyImportAccountNoTumbaElConsentimiento(t *testing.T) {
	h := &Handlers{}
	inst := &domain.AppInstance{ID: "i", TemplateKey: "google-workspace", Status: domain.StatusActive}
	cfg := map[string]string{}
	caido := roomsTestRuntime(func(*http.Request) (*http.Response, error) {
		return roomsTestResponse(http.StatusServiceUnavailable, `{}`), nil
	})
	if err := h.applyImportAccount(context.Background(), inst, func() connector.Runtime { return caido }, cfg); err != nil {
		t.Fatalf("un fallo al preguntar la cuenta tumbó el consentimiento: %v", err)
	}
	if cfg[configKeyImportAccount] != "" {
		t.Fatalf("no debía inventarse una cuenta: %v", cfg)
	}
	if _, err := time.Parse(time.RFC3339, cfg[configKeyAuthorizedAt]); err != nil {
		t.Fatalf("debía recordar cuándo se autorizó: %v", cfg)
	}
}

// TestSameImportAccount: una conexión nueva de una cuenta que ya está es la repetida; un
// borrador o una cuenta distinta, no.
func TestSameImportAccount(t *testing.T) {
	conexiones := []domain.AppInstance{
		conexion("borrador", "google-workspace", domain.StatusDraft, false, map[string]string{"domain": "acme.com"}, t0),
		conexion("otra", "google-workspace", domain.StatusActive, true, map[string]string{"domain": "beta.com"}, t0),
		conexion("m365", "microsoft365", domain.StatusActive, true, map[string]string{"tenant": "acme.com"}, t0),
		conexion("apagada", "google-workspace", domain.StatusDisabled, true,
			map[string]string{configKeyImportAccount: "google:acme.com"}, t0),
	}
	if got := sameImportAccount(conexiones, "nueva", "google-workspace", "google:acme.com"); got != "apagada" {
		t.Fatalf("la cuenta ya estaba (apagada): %q", got)
	}
	if got := sameImportAccount(conexiones, "apagada", "google-workspace", "google:acme.com"); got != "" {
		t.Fatalf("volver a autorizar la misma no es repetirla: %q", got)
	}
	if got := sameImportAccount(conexiones, "nueva", "google-workspace", "google:gamma.com"); got != "" {
		t.Fatalf("otra cuenta no es repetida: %q", got)
	}

	cfg, _ := json.Marshal(map[string]string{"domain": " ACME.com "})
	taken := declaredImportAccountTaken(conexiones, "google-workspace", cfg)
	if taken == nil || taken.Status != http.StatusConflict || taken.Code != "instance_exists" ||
		taken.Params["instance_id"] != "apagada" {
		t.Fatalf("crear otra con el mismo dominio debía dar 409 instance_exists: %+v", taken)
	}
	if declaredImportAccountTaken(conexiones, "google-workspace", json.RawMessage(`{}`)) != nil {
		t.Fatal("sin cuenta declarada lo decide el consentimiento, no el alta")
	}
	if declaredImportAccountTaken(conexiones, "jira", cfg) != nil {
		t.Fatal("solo se controla en los orígenes de importación")
	}
}
