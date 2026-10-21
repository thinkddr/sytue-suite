//go:build integration

// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestOrganizationOptOutBlocksAllManagedConsentEntryPoints(t *testing.T) {
	c := montarConsentimiento(t, func(h *Handlers, r chi.Router) {
		h.oauthClients = newOAuthClientResolver(h.store, map[string]config.OAuthClient{
			"google-drive":    {ClientID: "suite-fixture", ClientSecret: "suite-fixture-secret"},
			"onedrive":        {ClientID: "suite-fixture", ClientSecret: "suite-fixture-secret"},
			"slack":           {ClientID: "suite-fixture", ClientSecret: "suite-fixture-secret"},
			"dropbox-storage": {ClientID: "suite-fixture", ClientSecret: "suite-fixture-secret"},
			"yahoo-mail":      {ClientID: "suite-fixture", ClientSecret: "suite-fixture-secret"},
		})
		r.Post("/oauth/finish", h.OAuthFinish)
		r.Get("/instances", h.ListInstances)
		r.Post("/personal-storage/connect", h.PersonalStorageConnect)
		r.Post("/external-conversations/connect", h.ExternalChatConnect)
		r.Post("/external-mail/connect", h.ExternalMailConnect)
		r.Get("/storage-token", h.InternalStorageToken)
		r.Get("/rooms-destinations", h.InternalRoomsDestinations)
	})
	ctx := tenancy.Into(authn.Into(context.Background(), authn.Claims{Subject: "ana", Tenant: "acme", Roles: []string{"owner"}}), tenancy.Tenant{Slug: "acme"})
	for _, tc := range []struct{ template, provider, personalPath, operation string }{
		{tmplGoogleDrive, "google-drive", "/personal-storage/connect", "/storage-token"},
		{tmplOneDrive, "onedrive", "/personal-storage/connect", "/storage-token"},
		{tmplDropbox, "dropbox", "/personal-storage/connect", "/storage-token"},
		{tmplSlackRooms, "slack", "/external-conversations/connect", "/rooms-destinations"},
		{tmplTeamsRooms, "microsoft-teams", "/external-conversations/connect", "/rooms-destinations"},
		{tmplGoogleChat, "google-chat", "/external-conversations/connect", "/rooms-destinations"},
		{tmplGoogleMeet, "google-meet", "/external-conversations/connect", "/rooms-destinations"},
		{tmplSlackChat, "slack", "/external-conversations/connect", ""},
		{tmplTeamsChat, "microsoft-teams", "/external-conversations/connect", ""},
		{tmplGoogleConversations, "google-chat", "/external-conversations/connect", ""},
		{"gmail-mail", "gmail", "/external-mail/connect", ""},
		{"microsoft-mail", "microsoft", "/external-mail/connect", ""},
		{"yahoo-mail", "yahoo", "/external-mail/connect", ""},
	} {
		t.Run(tc.template, func(t *testing.T) {
			if err := c.st.UpsertTemplate(ctx, tc.template, json.RawMessage(manifiestoConConsentimiento()), "standard"); err != nil {
				t.Fatal(err)
			}
			instance, err := c.st.CreateInstance(ctx, tc.template, "Managed account", json.RawMessage(`{}`), nil)
			if err != nil {
				t.Fatal(err)
			}
			if personalConsentTemplate(tc.template) {
				if err := c.st.BindExternalChatActor(ctx, instance.ID, "ana"); err != nil {
					t.Fatal(err)
				}
			}
			if err := c.st.PutSecrets(ctx, instance.ID, map[string]string{store.SecretKeyBearer: "previous-fixture-grant"}); err != nil {
				t.Fatal(err)
			}
			if err := c.st.SetStatus(ctx, instance.ID, domain.StatusActive); err != nil {
				t.Fatal(err)
			}
			c.inst = instance.ID
			started := c.pedir(t, http.MethodPost, "/instances/"+instance.ID+"/oauth/start", "ana", "acme", map[string]string{})
			var start oauthStartResp
			if err := json.NewDecoder(started.Body).Decode(&start); err != nil {
				t.Fatal(err)
			}
			_ = started.Body.Close()
			if started.StatusCode != http.StatusOK || start.AuthorizeURL == "" {
				t.Fatalf("central consent unavailable: %d", started.StatusCode)
			}
			if err := c.st.SetIntegrationEnabled(ctx, tc.provider, false); err != nil {
				t.Fatal(err)
			}
			for _, route := range []string{"/instances/" + instance.ID + "/oauth/start", tc.personalPath} {
				res := c.pedir(t, http.MethodPost, route, "ana", "acme", map[string]string{"provider": tc.provider})
				_ = res.Body.Close()
				if res.StatusCode != http.StatusForbidden {
					t.Fatalf("disabled provider accepted %s: %d", route, res.StatusCode)
				}
			}
			if tc.operation != "" {
				res := c.pedir(t, http.MethodGet, tc.operation+"?tenant=acme&instance="+instance.ID, "ana", "acme", nil)
				_ = res.Body.Close()
				if res.StatusCode != http.StatusForbidden {
					t.Fatalf("existing grant bypassed opt-out: %d", res.StatusCode)
				}
			}
			before := len(canjesDelProveedor())
			if result := c.terminar(t, "ana", "acme", stateDe(t, start.AuthorizeURL), "disabled-grant"); result.Error != "exchange" {
				t.Fatalf("pending consent bypassed opt-out: %+v", result)
			}
			if len(canjesDelProveedor()) != before {
				t.Fatal("disabled provider received a token exchange")
			}
			grant, err := c.st.LoadInstanceWithSecrets(ctx, instance.ID)
			if err != nil || grant.Secrets[store.SecretKeyBearer] != "previous-fixture-grant" {
				t.Fatal("disabled consent replaced the previous connection")
			}
			if err := c.st.SetIntegrationEnabled(ctx, tc.provider, true); err != nil {
				t.Fatal(err)
			}
		})
	}
}
