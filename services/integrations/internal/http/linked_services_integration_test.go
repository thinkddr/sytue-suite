//go:build integration

// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestLinkedServicesKeepPersonalScopeIdentityAndOrgPolicy(t *testing.T) {
	a := montarAlmacen(t, func(h *Handlers, r chi.Router) {
		h.linkedServicesRoutes(r)
		r.Get("/external-mail/{instance}/folders", h.ExternalMailFolders)
	})
	ctx := tenancy.Into(authn.Into(context.Background(), authn.Claims{Subject: "ana", Tenant: "acme"}), tenancy.Tenant{Slug: "acme"})
	if err := a.st.UpsertTemplate(ctx, "gmail-mail", json.RawMessage(manifiestoConConsentimiento()), "tier2"); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]string{"_mail_config": `{"address":"original@example.eu"}`, mailServicesKey: `["contacts"]`}
	raw, _ := json.Marshal(cfg)
	instance, err := a.st.CreateInstance(ctx, "gmail-mail", "Fixture", raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.st.BindExternalChatActor(ctx, instance.ID, "ana"); err != nil {
		t.Fatal(err)
	}
	if err = a.st.MergeSecrets(ctx, instance.ID, map[string]string{store.SecretKeyBearer: "synthetic-access"}); err != nil {
		t.Fatal(err)
	}
	if err = a.st.SetStatus(ctx, instance.ID, domain.StatusActive); err != nil {
		t.Fatal(err)
	}
	path := "/linked-services/" + instance.ID
	for _, person := range []quien{bea, carla} {
		status, _ := a.pedir(t, person, http.MethodGet, path+"/contacts/collections", nil)
		if status != 403 {
			t.Fatalf("foreign member/admin accessed account: %s %d", person.sub, status)
		}
	}
	status, body := a.pedir(t, ana, http.MethodGet, path+"/contacts/collections?expected_address=original%40example.eu", nil)
	if status != 200 || !strings.Contains(string(body), "Google Contacts") || strings.Contains(string(body), "synthetic-access") {
		t.Fatalf("granted read without network: %d %s", status, body)
	}
	mailStatus, _ := a.pedir(t, ana, http.MethodGet, "/external-mail/"+instance.ID+"/folders", nil)
	if mailStatus != 422 {
		t.Fatalf("service-only account opened mailbox: %d", mailStatus)
	}
	status, body = a.pedir(t, ana, http.MethodGet, path+"/calendar/collections", nil)
	if status != 403 || !strings.Contains(string(body), "linked_service_not_granted") {
		t.Fatalf("ungranted service: %d %s", status, body)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		target := path + "/contacts/collections?expected_address=previous%40example.eu"
		var request any
		if method == http.MethodPost {
			target = path + "/contacts/save"
			request = map[string]any{"expected_address": "previous@example.eu", "collection": "contacts", "id": "people/c1", "etag": "v1", "patch": map[string]string{"name": "Updated"}}
		}
		status, body = a.pedir(t, ana, method, target, request)
		if status != 409 || !strings.Contains(string(body), "external_reconnect_required") {
			t.Fatalf("stale account %s: %d %s", method, status, body)
		}
	}
	if err = a.st.SetIntegrationEnabled(ctx, "gmail", false); err != nil {
		t.Fatal(err)
	}
	status, body = a.pedir(t, ana, http.MethodGet, path+"/contacts/collections", nil)
	if status != 403 || !strings.Contains(string(body), "integration_disabled") {
		t.Fatalf("disabled provider bypass: %d %s", status, body)
	}
}

func TestLinkedSourceConsentStartsWithoutMailboxScopes(t *testing.T) {
	c := montarConsentimiento(t, func(h *Handlers, r chi.Router) {
		h.oauthClients = newOAuthClientResolver(h.store, map[string]config.OAuthClient{
			"gmail-mail": {ClientID: "fixture", ClientSecret: "fixture"}, "microsoft-mail": {ClientID: "fixture", ClientSecret: "fixture"}, "yahoo-mail": {ClientID: "fixture", ClientSecret: "fixture"},
		})
		r.Post("/external-mail/connect", h.ExternalMailConnect)
	})
	ctx := tenancy.Into(authn.Into(t.Context(), authn.Claims{Subject: "ana", Tenant: "acme", Roles: []string{"owner"}}), tenancy.Tenant{Slug: "acme"})
	for _, provider := range []string{"gmail", "microsoft", "yahoo"} {
		if err := c.st.UpsertTemplate(ctx, mailProviderTemplates[provider], json.RawMessage(manifiestoConConsentimiento()), "tier2"); err != nil {
			t.Fatal(err)
		}
		for _, service := range []string{"calendar", "contacts", "notes"} {
			if service == "notes" && provider != "microsoft" {
				continue
			}
			for _, prefix := range []string{"native-", "external-"} {
				res := c.pedir(t, http.MethodPost, "/external-mail/connect", "ana", "acme", map[string]any{"provider": provider, "return_to": prefix + service, "services": []string{service}})
				var start struct {
					Scopes []string `json:"scopes"`
				}
				err := json.NewDecoder(res.Body).Decode(&start)
				_ = res.Body.Close()
				if err != nil || res.StatusCode != http.StatusOK {
					t.Fatalf("origin-only start: %s %s %d %v", provider, service, res.StatusCode, err)
				}
				for _, scope := range []string{"https://www.googleapis.com/auth/gmail.modify", "Mail.ReadWrite", "Mail.Send", "mail-r", "mail-w"} {
					if slices.Contains(start.Scopes, scope) {
						t.Errorf("%s %s required unselected mailbox: %s", provider, service, scope)
					}
				}
			}
		}
		res := c.pedir(t, http.MethodPost, "/external-mail/connect", "ana", "acme", map[string]any{"provider": provider, "return_to": "native-mail", "services": []string{"calendar"}})
		_ = res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("Mail origin omitted its required permission: %d", res.StatusCode)
		}
	}
}
