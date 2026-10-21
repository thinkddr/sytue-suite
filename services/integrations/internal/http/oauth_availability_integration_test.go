// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package integrationshttp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

func TestConsentInventoryReadinessUsesOnlyThisTenantsVault(t *testing.T) {
	c := montarConsentimiento(t, func(h *Handlers, r chi.Router) {
		r.Get("/templates", h.ListTemplates)
		r.Get("/instances", h.ListInstances)
	})
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})
	read := func(org string) (bool, string) {
		t.Helper()
		response := c.pedir(t, http.MethodGet, "/instances", "ana", org, nil)
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		var body struct {
			Instances []struct {
				ID        string `json:"id"`
				Available bool   `json:"oauth_available"`
			} `json:"instances"`
		}
		if response.StatusCode != http.StatusOK || json.Unmarshal(raw, &body) != nil {
			t.Fatalf("inventory status=%d", response.StatusCode)
		}
		for _, item := range body.Instances {
			if item.ID == c.inst {
				return item.Available, string(raw)
			}
		}
		return false, string(raw)
	}
	if ready, _ := read("acme"); ready {
		t.Fatal("a client_id without its registered secret is offered as consent-ready")
	}
	if err := c.st.PutSecrets(ctx, c.inst, map[string]string{secretKeyClientSc: "private-application-secret"}); err != nil {
		t.Fatal(err)
	}
	if ready, raw := read("acme"); !ready || strings.Contains(raw, "private-application-secret") || strings.Contains(raw, "ConsentManifest") {
		t.Fatal("own registered app is unavailable or exposes private vault metadata")
	}
	if ready, raw := read("otra"); ready || strings.Contains(raw, c.inst) || strings.Contains(raw, "app-de-acme") {
		t.Fatal("readiness leaked another tenant's connection")
	}
	response := c.pedir(t, http.MethodGet, "/templates", "ana", "acme", nil)
	defer response.Body.Close()
	var catalog struct {
		Templates []struct {
			Key       string `json:"key"`
			Available bool   `json:"oauth_available"`
		} `json:"templates"`
	}
	if json.NewDecoder(response.Body).Decode(&catalog) != nil {
		t.Fatal("invalid catalogue")
	}
	for _, tpl := range catalog.Templates {
		if tpl.Key == "prueba-oauth" && tpl.Available {
			t.Fatal("a tenant-owned application became platform availability")
		}
	}
}

func TestStorageReadinessIncludesOnlyRegisteredTenantAccounts(t *testing.T) {
	c := montarConsentimiento(t, func(h *Handlers, r chi.Router) { r.Get("/storage", h.InternalStorageInstances) })
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})
	if err := c.st.UpsertTemplate(ctx, tmplGoogleDrive, []byte(manifiestoConConsentimiento()), "standard"); err != nil {
		t.Fatal(err)
	}
	inst, err := c.st.CreateInstance(ctx, tmplGoogleDrive, "Own Drive", []byte(`{"_oauth_client_id":"own-drive"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.st.PutSecrets(ctx, inst.ID, map[string]string{secretKeyClientSc: "private-drive-client", "bearer": "private-drive-token"}); err != nil {
		t.Fatal(err)
	}
	if err = c.st.SetStatus(ctx, inst.ID, domain.StatusActive); err != nil {
		t.Fatal(err)
	}
	for _, org := range []string{"acme", "otra"} {
		response := c.pedir(t, http.MethodGet, "/storage?tenant="+org, "ana", org, nil)
		raw, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		var body struct {
			Items []struct {
				ID        string `json:"id"`
				Available bool   `json:"oauth_available"`
				Connected bool   `json:"connected"`
			} `json:"items"`
			Providers map[string]bool `json:"providers"`
		}
		if response.StatusCode != http.StatusOK || json.Unmarshal(raw, &body) != nil {
			t.Fatalf("storage status=%d", response.StatusCode)
		}
		if strings.Contains(string(raw), "private-drive-") {
			t.Fatal("storage readiness exposed vault values")
		}
		if org == "acme" {
			if !body.Providers["gdrive"] || len(body.Items) != 1 || !body.Items[0].Available || !body.Items[0].Connected {
				t.Fatal("registered storage account unavailable")
			}
		} else if body.Providers["gdrive"] || len(body.Items) != 0 {
			t.Fatal("storage availability leaked another tenant's registered app")
		}
	}
}
