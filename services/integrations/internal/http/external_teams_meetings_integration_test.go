//go:build integration

// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestProtectedTeamsPersonalConsentAndOwnership(t *testing.T) {
	c := montarConsentimiento(t, func(h *Handlers, r chi.Router) {
		h.oauthClients = newOAuthClientResolver(h.store, map[string]config.OAuthClient{tmplTeamsChat: {ClientID: "suite-fixture", ClientSecret: "fixture-secret"}})
		r.Post("/external-conversations/connect", h.ExternalChatConnect)
		r.Get("/external-conversations/accounts", h.ExternalChatAccounts)
		r.Post("/external-conversations/meeting", h.ExternalChatMeeting)
		r.Patch("/external-conversations/meeting", h.ExternalChatMeeting)
	})
	ctx := tenancy.Into(authn.Into(context.Background(), authn.Claims{Subject: "ana", Tenant: "acme"}), tenancy.Tenant{Slug: "acme"})
	if err := c.st.UpsertTemplate(ctx, tmplTeamsChat, json.RawMessage(manifiestoConConsentimiento()), "standard"); err != nil {
		t.Fatal(err)
	}
	res := c.pedir(t, http.MethodPost, "/external-conversations/connect", "ana", "acme", map[string]string{"provider": "microsoft-teams", "purpose": "meeting"})
	var start oauthStartResp
	if err := json.NewDecoder(res.Body).Decode(&start); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	u, _ := url.Parse(start.AuthorizeURL)
	scopes := strings.Fields(u.Query().Get("scope"))
	if res.StatusCode != http.StatusOK || !slices.Equal(scopes, []string{"offline_access", "User.Read", teamsMeetScope}) {
		t.Fatalf("new account requested unrelated grants: %d %v", res.StatusCode, scopes)
	}
	rd, err := consent.Callback(c.cif, stateDe(t, start.AuthorizeURL), "fixture-code", "", time.Now())
	if err != nil || rd.Section != teamsMeetingSection {
		t.Fatalf("meeting callback lost its sealed purpose: %+v %v", rd, err)
	}
	instances, err := c.st.ListExternalChatInstances(ctx, "ana")
	if err != nil || len(instances) != 1 {
		t.Fatalf("empty-to-consent: %d %v", len(instances), err)
	}
	id := instances[0].ID
	cfg := map[string]string{"_external_user_id": teamsMeetingActor, teamsServicesKey: `["meeting"]`, teamsScopesKey: "offline_access User.Read OnlineMeetings.ReadWrite"}
	encoded, _ := json.Marshal(cfg)
	if _, err := c.st.CommitInstanceConnection(ctx, id, "Teams", encoded, domain.StatusActive, nil, map[string]string{store.SecretKeyBearer: "fixture-token"}); err != nil {
		t.Fatal(err)
	}
	inv := c.pedir(t, http.MethodGet, "/external-conversations/accounts", "ana", "acme", nil)
	var inventory struct {
		Items    []map[string]any `json:"items"`
		Meetings []map[string]any `json:"meetings"`
	}
	if err := json.NewDecoder(inv.Body).Decode(&inventory); err != nil {
		t.Fatal(err)
	}
	_ = inv.Body.Close()
	if len(inventory.Items) != 0 {
		t.Fatal("meeting-only consent enabled chat reading")
	}
	found := false
	for _, meeting := range inventory.Meetings {
		if meeting["instance_id"] == id {
			found = meeting["connected"] == true && meeting["account_id"] == teamsMeetingActor && meeting["e2ee_required"] == true
		}
	}
	if !found {
		t.Fatal("verified purpose/account missing from inventory")
	}
	for _, tc := range []struct {
		sub, org, method, instance, account string
		status                              int
	}{
		{"bea", "acme", http.MethodPost, id, teamsMeetingActor, 403},
		{"ana", "otra", http.MethodPost, id, teamsMeetingActor, 403},
		{"ana", "acme", http.MethodPost, id, "different-account", 409},
		{"ana", "acme", http.MethodPost, "", teamsMeetingActor, 400},
		{"ana", "acme", http.MethodPatch, id, teamsMeetingActor, 400},
	} {
		response := c.pedir(t, tc.method, "/external-conversations/meeting", tc.sub, tc.org, map[string]any{"provider": "microsoft-teams", "instance_id": tc.instance, "expected_account_id": tc.account, "e2ee": true})
		var raw map[string]any
		_ = json.NewDecoder(response.Body).Decode(&raw)
		_ = response.Body.Close()
		if response.StatusCode != tc.status || raw["url"] != nil {
			t.Fatalf("ownership/method guard: %+v got %d", tc, response.StatusCode)
		}
	}
	if err := c.st.SetIntegrationEnabled(ctx, "microsoft-teams", false); err != nil {
		t.Fatal(err)
	}
	response := c.pedir(t, http.MethodPost, "/external-conversations/meeting", "ana", "acme", map[string]any{"provider": "microsoft-teams", "instance_id": id, "expected_account_id": teamsMeetingActor, "e2ee": true})
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatal("organisation opt-out bypassed by existing meeting grant")
	}
}
