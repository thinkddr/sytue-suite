//go:build integration

// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

func TestExternalMailIsPersonalEvenForOrgAdmins(t *testing.T) {
	var handlers *Handlers
	a := montarAlmacen(t, func(h *Handlers, r chi.Router) {
		handlers = h
		h.oauthCipher, _ = keycrypto.New(make([]byte, 32))
		r.Get("/external-mail/accounts", h.ExternalMailAccounts)
		r.Post("/external-mail/accounts", h.ExternalMailConfigure)
		r.Get("/external-mail/{instance}/folders", h.ExternalMailFolders)
		r.Post("/external-mail/{instance}/send", h.ExternalMailSend)
		r.Post("/external-mail/{instance}/actions", h.ExternalMailAction)
		r.Delete("/external-mail/{instance}", h.ExternalMailDisconnect)
		r.Get("/instances/{id}", h.GetInstance)
		r.Patch("/instances/{id}", h.UpdateInstance)
		r.Put("/instances/{id}/secrets", h.PutSecrets)
		r.Patch("/instances/{id}/secrets", h.PatchSecrets)
		r.Delete("/instances/{id}", h.DeleteInstance)
		r.Post("/instances/{id}/activate", h.Activate)
		r.Post("/instances/{id}/deactivate", h.Deactivate)
	})
	ctx := tenancy.Into(authn.Into(context.Background(), authn.Claims{Subject: "ana", Tenant: "acme", Roles: []string{"member"}}), tenancy.Tenant{Slug: "acme"})
	if err := a.st.UpsertTemplate(ctx, "mail-imap", json.RawMessage(`{"name":"IMAP","description":"Fixture","color":"#123456","icon":"<svg/>","identity":{"kind":"none"},"launch":{"kind":"none"},"provisioning":"none"}`), "tier2"); err != nil {
		t.Fatal(err)
	}
	instance, err := a.st.CreateInstance(ctx, "mail-imap", "Private mailbox", json.RawMessage(`{"_mail_config":"{\"address\":\"fixture@example.eu\",\"incoming\":{\"host\":\"127.0.0.1\",\"port\":993,\"security\":\"tls\"}}"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.st.BindExternalChatActor(ctx, instance.ID, "ana"); err != nil {
		t.Fatal(err)
	}
	if err := a.st.MergeSecrets(ctx, instance.ID, map[string]string{mailPasswordKey: "fixture-password"}); err != nil {
		t.Fatal(err)
	}
	if err := a.st.SetStatus(ctx, instance.ID, domain.StatusActive); err != nil {
		t.Fatal(err)
	}
	for _, person := range []quien{ana, bea, carla} {
		status, raw := a.pedir(t, person, http.MethodGet, "/external-mail/accounts", nil)
		if status != 200 {
			t.Fatalf("accounts %s: %d %s", person.sub, status, raw)
		}
		visible := strings.Contains(string(raw), instance.ID)
		if visible != (person.sub == "ana") || strings.Contains(string(raw), "fixture-password") {
			t.Fatalf("accounts visibility/secret: %s", raw)
		}
		if person.sub == "ana" {
			continue
		}
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			path := "/external-mail/" + instance.ID
			if method == http.MethodGet {
				path += "/folders"
			}
			status, _ = a.pedir(t, person, method, path, nil)
			if status != http.StatusForbidden {
				t.Fatalf("foreign account %s %s: %d", person.sub, method, status)
			}
		}
		for _, operation := range []struct {
			method, path string
			body         any
			status       int
		}{
			{http.MethodGet, "/instances/" + instance.ID, nil, 404},
			{http.MethodPatch, "/instances/" + instance.ID, map[string]any{"config": map[string]string{"_mail_config": "replacement"}}, 403},
			{http.MethodPut, "/instances/" + instance.ID + "/secrets", map[string]any{"secrets": map[string]string{"mail_password": "replacement"}}, 403},
			{http.MethodPatch, "/instances/" + instance.ID + "/secrets", map[string]any{"secrets": map[string]string{"mail_password": "replacement"}}, 403},
			{http.MethodDelete, "/instances/" + instance.ID, nil, 403},
			{http.MethodPost, "/instances/" + instance.ID + "/activate", nil, 403},
			{http.MethodPost, "/instances/" + instance.ID + "/deactivate", nil, 403},
		} {
			got, raw := a.pedir(t, person, operation.method, operation.path, operation.body)
			if got != operation.status {
				t.Fatalf("generic private route %s %s: %d %s", operation.method, operation.path, got, raw)
			}
		}
		// A foreign owner must be rejected before testing supplied remote credentials.
		status, _ = a.pedir(t, person, http.MethodPost, "/external-mail/accounts", map[string]any{"provider": "imap", "instance_id": instance.ID, "name": "replacement", "address": "other@example.eu", "password": "fixture", "incoming": map[string]any{"host": "127.0.0.1", "port": 993, "security": "tls"}, "outgoing": map[string]any{"host": "127.0.0.1", "port": 465, "security": "tls"}})
		if status != 403 {
			t.Fatalf("foreign configure must precede network: %d", status)
		}
	}
	// A draft or message action opened before reconnect cannot use a different account.
	for _, path := range []string{"send", "actions"} {
		status, raw := a.pedir(t, ana, http.MethodPost, "/external-mail/"+instance.ID+"/"+path, map[string]any{"expected_address": "old-account@example.eu"})
		if status != 409 || !strings.Contains(string(raw), "external_reconnect_required") {
			t.Fatalf("stale account %s: %d %s", path, status, raw)
		}
	}
	// Owner gets an actionable SSRF error, rather than attempting loopback TLS.
	status, raw := a.pedir(t, ana, http.MethodGet, "/external-mail/"+instance.ID+"/folders", nil)
	if status != 422 || !strings.Contains(string(raw), "mail_blocked") {
		t.Fatalf("safe raw transport: %d %s", status, raw)
	}
	if err := a.st.SetIntegrationEnabled(ctx, "imap", false); err != nil {
		t.Fatal(err)
	}
	status, raw = a.pedir(t, ana, http.MethodGet, "/external-mail/"+instance.ID+"/folders", nil)
	if status != 403 || !strings.Contains(string(raw), "integration_disabled") {
		t.Fatalf("org disabled mailbox: %d %s", status, raw)
	}
	if handlers == nil {
		t.Fatal("handler not mounted")
	}
	// Disconnect stays available when the provider is offline or disabled.
	status, _ = a.pedir(t, ana, http.MethodDelete, "/external-mail/"+instance.ID, nil)
	if status != 204 {
		t.Fatalf("disconnect: %d", status)
	}
}

func TestExternalMailStaleRefreshStopsOriginalOperation(t *testing.T) {
	var h *Handlers
	a := montarAlmacen(t, func(handler *Handlers, _ chi.Router) { h = handler })
	ctx := tenancy.Into(authn.Into(context.Background(), authn.Claims{Subject: "ana", Tenant: "acme"}), tenancy.Tenant{Slug: "acme"})
	if err := a.st.UpsertTemplate(ctx, "gmail-mail", json.RawMessage(manifiestoConConsentimiento()), "tier2"); err != nil {
		t.Fatal(err)
	}
	cfgA := json.RawMessage(`{"_mail_config":"{\"address\":\"a@example.eu\"}","_oauth_expires_at":"2000-01-01T00:00:00Z"}`)
	inst, err := a.st.CreateInstance(ctx, "gmail-mail", "Account A", cfgA, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.st.BindExternalChatActor(ctx, inst.ID, "ana"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.st.CommitInstanceConnection(ctx, inst.ID, "Account A", cfgA, domain.StatusActive, nil, map[string]string{store.SecretKeyBearer: "bearer-a", secretKeyRefresh: "refresh-a"}); err != nil {
		t.Fatal(err)
	}
	old, err := a.st.LoadInstanceWithSecrets(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	entered, resume := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(entered)
		<-resume
		conn, _, err := h.saveMailOAuthRefresh(ctx, inst, old, tokenResponse{AccessToken: "renewed-a", ExpiresIn: 3600})
		if err == nil && conn.Secrets[store.SecretKeyBearer] != "" {
			result <- nil
			return
		}
		result <- err
	}()
	<-entered
	cfgB := json.RawMessage(`{"_mail_config":"{\"address\":\"b@example.eu\"}","_oauth_expires_at":"2000-01-01T00:00:00Z"}`)
	if _, err := a.st.CommitInstanceConnection(ctx, inst.ID, "Account B", cfgB, domain.StatusActive, nil, map[string]string{store.SecretKeyBearer: "bearer-b"}); err != nil {
		t.Fatal(err)
	}
	close(resume)
	if err := <-result; err == nil || httpx.AsError(err).Status != 409 || httpx.AsError(err).Code != "external_reconnect_required" {
		t.Fatalf("stale refresh must stop original send, got %v", err)
	}
	current, err := a.st.LoadInstanceWithSecrets(ctx, inst.ID)
	if err != nil || current.Secrets[store.SecretKeyBearer] != "bearer-b" {
		t.Fatal("stale refresh replaced reconnected account")
	}
	if _, expires, err := h.saveMailOAuthRefresh(ctx, inst, current, tokenResponse{AccessToken: "renewed-b"}); err != nil || !expires.IsZero() {
		t.Fatalf("non-expiring refresh preserved an expiry: %v %s", err, expires)
	}
	current, err = a.st.LoadInstanceWithSecrets(ctx, inst.ID)
	if err != nil || current.Config[configKeyExpires] != "" {
		t.Fatal("non-expiring refresh did not clear expiry")
	}
}
