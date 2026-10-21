// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
)

// VerifyWhatsAppPlatformWebhook verifica el callback fijo para account_update.
// Los mensajes de cada número tienen su callback firmado en Rooms.
func (h *Handlers) VerifyWhatsAppPlatformWebhook(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	got := q.Get("hub.verify_token")
	if h.whatsAppWebhookVerifyToken == "" || q.Get("hub.mode") != "subscribe" || subtle.ConstantTimeCompare([]byte(got), []byte(h.whatsAppWebhookVerifyToken)) != 1 {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, q.Get("hub.challenge"))
}

// ReceiveWhatsAppPlatformWebhook acepta solo avisos de cuenta autenticados. Los
// mensajes nunca se descartan aquí: un callback de número incompleto pide reintento.
func (h *Handlers) ReceiveWhatsAppPlatformWebhook(w http.ResponseWriter, r *http.Request) {
	client, _, available := h.whatsAppPlatform(r.Context())
	if !available {
		httpx.Fail(w, r, whatsAppNotConfigured())
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	signature, ok := strings.CutPrefix(strings.TrimSpace(r.Header.Get("X-Hub-Signature-256")), "sha256=")
	if !ok || !keycrypto.VerifyHMACSHA256(client.ClientSecret, raw, signature) {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var event struct {
		Object string `json:"object"`
		Entry  []struct {
			Changes []struct {
				Field string `json:"field"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if json.Unmarshal(raw, &event) != nil || event.Object != "whatsapp_business_account" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	for _, entry := range event.Entry {
		for _, change := range entry.Changes {
			if change.Field != "account_update" {
				httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("The WhatsApp number callback is not ready yet."))
				return
			}
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]bool{"received": true})
}
