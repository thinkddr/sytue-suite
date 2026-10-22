// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
)

func TestWhatsAppWebhookPlataformaExigeFirmaYNoDescartaMensajes(t *testing.T) {
	cipher, err := keycrypto.New(make([]byte, keycrypto.KeyLen))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{whatsAppConfigID: "config", whatsAppWebhookVerifyToken: "verify", oauthCipher: cipher, oauthClients: &oauthClientResolver{cachedAt: time.Now(), cached: map[string]config.OAuthClient{tmplWhatsAppBusiness: {ClientID: "app", ClientSecret: "secreto"}}}}
	for _, tc := range []struct {
		field, signature string
		status           int
	}{{"account_update", "valid", http.StatusOK}, {"messages", "valid", http.StatusServiceUnavailable}, {"account_update", "invalid", http.StatusUnauthorized}} {
		body := `{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"` + tc.field + `"}]}]}`
		mac := hmac.New(sha256.New, []byte("secreto"))
		_, _ = mac.Write([]byte(body))
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if tc.signature != "valid" {
			signature = "sha256=" + strings.Repeat("0", 64)
		}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/whatsapp/webhook", strings.NewReader(body))
		r.Header.Set("X-Hub-Signature-256", signature)
		w := httptest.NewRecorder()
		h.ReceiveWhatsAppPlatformWebhook(w, r)
		if w.Code != tc.status {
			t.Fatalf("field=%s firma=%s status=%d body=%s", tc.field, tc.signature, w.Code, w.Body)
		}
	}
	for _, token := range []string{"incorrecto", "verify"} {
		w := httptest.NewRecorder()
		h.VerifyWhatsAppPlatformWebhook(w, httptest.NewRequest(http.MethodGet, "/api/v1/whatsapp/webhook?hub.mode=subscribe&hub.challenge=123&hub.verify_token="+token, http.NoBody))
		if (token == "verify") != (w.Code == http.StatusOK) {
			t.Fatalf("verificación incorrecta: %d", w.Code)
		}
	}
}
