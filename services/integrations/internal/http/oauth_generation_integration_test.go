// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// Exercise the real start/finish routes and TLS egress. An older exchange is
// deliberately held at the provider until a newer consent has fully committed.
func TestSlowOAuthExchangeCannotReplaceTheLatestUserChoice(t *testing.T) {
	entered, resume := make(chan struct{}), make(chan struct{})
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		code := r.PostForm.Get("code")
		if code == "slow-a" {
			close(entered)
			<-resume
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "token-" + code, "expires_in": 3600})
	}))
	defer provider.Close()
	resumed := false
	defer func() {
		if !resumed {
			close(resume)
		}
	}()
	c := montarConsentimiento(t, func(h *Handlers, r chi.Router) { r.Post("/oauth/finish", h.OAuthFinish) })
	var manifest map[string]any
	if err := json.Unmarshal([]byte(manifiestoConConsentimiento()), &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["oauth"].(map[string]any)["token_url"] = provider.URL + "/token"
	encoded, _ := json.Marshal(manifest)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	if err := c.st.UpsertTemplate(ctx, "prueba-oauth", encoded, "standard"); err != nil {
		t.Fatal(err)
	}
	stateA := c.arrancar(t, "ana")
	result := make(chan oauthFinishResp, 1)
	go func() { result <- c.terminar(t, "ana", "acme", stateA, "slow-a") }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("older exchange did not reach the TLS provider")
	}
	stateB := c.arrancar(t, "ana")
	if out := c.terminar(t, "ana", "acme", stateB, "chosen-b"); out.Error != "" {
		t.Fatalf("newer consent failed: %+v", out)
	}
	close(resume)
	resumed = true
	select {
	case out := <-result:
		if out.Error != "exchange" {
			t.Fatalf("older exchange was accepted: %+v", out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("older exchange did not finish")
	}
	conn, err := c.st.LoadInstanceWithSecrets(ctx, c.inst)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Secrets[store.SecretKeyBearer] != "token-chosen-b" || conn.Config[store.ConfigKeyConsentimiento] != "" {
		t.Fatal("older exchange replaced latest account or restored consumed state")
	}
}
