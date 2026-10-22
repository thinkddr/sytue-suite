// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestWhatsAppEmbeddedVinculaLaSesionYConsumeElNonceSinRestaurarlo(t *testing.T) {
	var h *Handlers
	c := montarConsentimiento(t, func(handlers *Handlers, r chi.Router) {
		h = handlers
		h.whatsAppConfigID = "123"
		h.whatsAppWebhookVerifyToken = "verificacion-plataforma"
		h.whatsAppGraphVersion = "v26.0"
		h.oauthClients = newOAuthClientResolver(h.store, map[string]config.OAuthClient{tmplWhatsAppBusiness: {ClientID: "456", ClientSecret: "secreto-plataforma"}})
		r.Get("/whatsapp/signup", h.WhatsAppSignupInfo)
		r.Post("/whatsapp/signup/start", h.WhatsAppSignupStart)
	})
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	if err := c.st.UpsertTemplate(ctx, tmplWhatsAppBusiness, json.RawMessage(manifiestoConConsentimiento()), "tier2"); err != nil {
		t.Fatal(err)
	}
	res := c.pedir(t, http.MethodPost, "/whatsapp/signup/start", "ana", "acme", map[string]string{})
	var start struct {
		Instance string `json:"instance_id"`
		State    string `json:"state"`
		AppID    string `json:"app_id"`
		ConfigID string `json:"config_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&start); err != nil || res.StatusCode != http.StatusOK || start.Instance == "" || start.State == "" || start.AppID != "456" || start.ConfigID != "123" {
		t.Fatalf("inicio incorrecto: status=%d %+v err=%v", res.StatusCode, start, err)
	}
	actor := authn.Into(ctx, authn.Claims{Subject: "ana", Tenant: "acme"})
	if err := h.previewWhatsAppState(actor, start.State, start.Instance); err != nil {
		t.Fatal(err)
	}
	if err := h.previewWhatsAppState(authn.Into(ctx, authn.Claims{Subject: "bea", Tenant: "acme"}), start.State, start.Instance); err == nil {
		t.Fatal("otra persona pudo preparar el SDK")
	}
	rd, err := consent.Callback(c.cif, start.State, "codigo-meta", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	inst, err := c.st.GetInstance(ctx, start.Instance)
	if err != nil {
		t.Fatal(err)
	}
	if inst.ConfigMap()[store.ConfigKeyConsentimiento] != rd.Challenge() {
		t.Fatal("preview gastó el nonce")
	}
	inst, err = h.consumeWhatsAppState(actor, whatsAppFinishRequest{State: start.State, Code: "codigo-meta"})
	if err != nil {
		t.Fatal(err)
	}
	conn := connector.Instance{ID: inst.ID, OrgID: inst.OrgID, TemplateKey: tmplWhatsAppBusiness, Secrets: map[string]string{store.SecretKeyBearer: "token-meta", secretWAAppSecret: "secreto-plataforma", secretWAAccounts: `["789"]`}}
	if err := h.saveWhatsAppGrant(actor, inst, conn, tokenResponse{AccessToken: "token-meta", ExpiresIn: 3600}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	saved, err := c.st.GetInstance(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ConfigMap()[store.ConfigKeyConsentimiento] != "" {
		t.Fatal("guardar el grant restauró un nonce ya gastado")
	}
	if _, err := h.consumeWhatsAppState(actor, whatsAppFinishRequest{State: start.State, Code: "otro-codigo"}); err == nil {
		t.Fatal("el state se consumió dos veces")
	}
	if err := h.previewWhatsAppState(actor, start.State, start.Instance); err == nil {
		t.Fatal("preview acepta un state ya consumido")
	}
	if string(saved.Config) == "" || !saved.HasSecrets {
		t.Fatal("se perdió la cuenta conectada")
	}
	var public map[string]any
	if err := json.Unmarshal(saved.Config, &public); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"access_token", "app_secret", secretWAAccounts, secretWAAppSecret} {
		if _, ok := public[key]; ok {
			t.Fatalf("un secreto llegó a config pública: %s", key)
		}
	}
}
