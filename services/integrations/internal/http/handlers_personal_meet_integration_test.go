//go:build integration

// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestPersonalMeetInventoryAndSelectionStayWithTheirOwner(t *testing.T) {
	c := montarConsentimiento(t, func(h *Handlers, r chi.Router) {
		r.Get("/external-conversations/accounts", h.ExternalChatAccounts)
		r.Post("/external-conversations/meeting", h.ExternalChatMeeting)
	})
	ctx := tenancy.Into(authn.Into(context.Background(), authn.Claims{Subject: "ana", Tenant: "acme"}), tenancy.Tenant{Slug: "acme"})
	if err := c.st.UpsertTemplate(ctx, tmplGoogleMeet, json.RawMessage(manifiestoConConsentimiento()), "standard"); err != nil {
		t.Fatal(err)
	}
	create := func(owner, name string, active bool) string {
		ownerCtx := authn.Into(ctx, authn.Claims{Subject: owner, Tenant: "acme"})
		inst, err := c.st.CreateInstance(ownerCtx, tmplGoogleMeet, name, json.RawMessage(`{}`), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.st.BindExternalChatActor(ownerCtx, inst.ID, owner); err != nil {
			t.Fatal(err)
		}
		if active {
			if err := c.st.PutSecrets(ownerCtx, inst.ID, map[string]string{store.SecretKeyBearer: "fixture-" + name}); err != nil {
				t.Fatal(err)
			}
			if err := c.st.SetStatus(ownerCtx, inst.ID, domain.StatusActive); err != nil {
				t.Fatal(err)
			}
		}
		return inst.ID
	}
	a, b, draft, foreign := create("ana", "First", true), create("ana", "Second", true), create("ana", "Pending", false), create("bea", "Private", true)
	response := c.pedir(t, http.MethodGet, "/external-conversations/accounts", "ana", "acme", nil)
	defer response.Body.Close()
	var raw struct {
		Meetings []map[string]any `json:"meetings"`
	}
	if err := json.NewDecoder(response.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	google := raw.Meetings[:0]
	for _, meeting := range raw.Meetings {
		if meeting["provider"] == "google-meet" {
			google = append(google, meeting)
		}
	}
	raw.Meetings = google
	if response.StatusCode != http.StatusOK || len(raw.Meetings) != 3 {
		t.Fatalf("inventory: %d %+v", response.StatusCode, raw)
	}
	visible := map[string]bool{}
	for _, item := range raw.Meetings {
		id, _ := item["instance_id"].(string)
		visible[id] = true
		if item["name"] == "" || item["name"] == nil {
			t.Fatal("missing account label")
		}
		if id == draft && item["connected"] != false {
			t.Fatal("pending account presented as connected")
		}
	}
	if !visible[a] || !visible[b] || !visible[draft] || visible[foreign] {
		t.Fatal("inventory lost or disclosed an account")
	}
	for _, tc := range []struct {
		instance string
		status   int
		code     string
	}{
		{"", 409, "meeting_account_required"},
		{draft, 409, "meeting_not_connected"},
		{foreign, 403, "forbidden"},
		{"not-a-uuid", 400, "bad_request"},
	} {
		res := c.pedir(t, http.MethodPost, "/external-conversations/meeting", "ana", "acme", map[string]string{"provider": "google-meet", "instance_id": tc.instance})
		var out struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != tc.status || !strings.Contains(out.Error.Code, tc.code) {
			t.Fatalf("choice %q: %d %s", tc.instance, res.StatusCode, out.Error.Code)
		}
	}
	res := c.pedir(t, http.MethodPost, "/external-conversations/meeting", "ana", "otra", map[string]string{"instance_id": a})
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("another tenant selected a personal account: %d", res.StatusCode)
	}
}

func TestConsentCanRetryAnExistingNullConfigDraft(t *testing.T) {
	var handler *Handlers
	c := montarConsentimiento(t, func(h *Handlers, _ chi.Router) { handler = h })
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	inst, err := c.st.CreateInstance(ctx, "prueba-oauth", "Old native draft", json.RawMessage(`null`), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, config, err := handler.loadInstanceManifest(ctx, inst.ID)
	if err != nil || config == nil {
		t.Fatalf("unusable old draft: %v", err)
	}
	config[configKeyExpires] = "a renewed consent can write metadata"
}
