// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pagos/adyen"
)

// montarWebhooks registra los webhooks de las pasarelas (§6.5, §10.5).
func montarWebhooks(g grupos, h *Handlers) {
	g.webhooks.Post("/mollie", h.webhookMollie)
	g.webhooks.Post("/mollie/{payment_id}", h.webhookMollie)

	g.webhooks.Post("/redsys", h.webhookRedsys)
	g.webhooks.Post("/redsys/{payment_id}", h.webhookRedsys)

	g.webhooks.Post("/stripe", h.webhookStripe)
	g.webhooks.Post("/adyen", h.webhookAdyen)
}

func (h *Handlers) webhookMollie(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}

	// Mollie acota cuerpo a 8 KiB (§6.1, §10.5)
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_form"))
		return
	}

	idOrRef := strings.TrimSpace(r.FormValue("id"))
	if idOrRef == "" {
		idOrRef = strings.TrimSpace(chi.URLParam(r, "payment_id"))
	}
	if idOrRef == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("missing_id"))
		return
	}

	if err := h.Tienda.ProcesarWebhookMollie(r.Context(), idOrRef); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("webhook_failed").WithMessage(err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (h *Handlers) webhookRedsys(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}

	if err := r.ParseForm(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_form"))
		return
	}

	params := r.FormValue("Ds_MerchantParameters")
	sig := r.FormValue("Ds_Signature")
	if params == "" || sig == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("missing_signature_or_params"))
		return
	}

	if err := h.Tienda.ProcesarWebhookRedsys(r.Context(), params, sig); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("webhook_failed").WithMessage(err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (h *Handlers) webhookStripe(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}

	sigHeader := r.Header.Get("Stripe-Signature")
	if sigHeader == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("missing_stripe_signature"))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("read_body_failed"))
		return
	}

	if err := h.Tienda.ProcesarWebhookStripe(r.Context(), body, sigHeader); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("webhook_failed").WithMessage(err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (h *Handlers) webhookAdyen(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}

	var root struct {
		NotificationItems []struct {
			NotificationRequestItem adyen.Notificacion `json:"NotificationRequestItem"` //nolint:tagliatelle // Contrato Standard webhook de Adyen
		} `json:"notificationItems"` //nolint:tagliatelle // Contrato Standard webhook de Adyen
	}

	if err := json.NewDecoder(r.Body).Decode(&root); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json"))
		return
	}

	for _, item := range root.NotificationItems {
		if err := h.Tienda.ProcesarWebhookAdyen(r.Context(), item.NotificationRequestItem); err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("webhook_failed").WithMessage(err.Error()))
			return
		}
	}

	// Respuesta obligatoria exigida por el protocolo Standard webhook de Adyen
	httpx.JSON(w, r, http.StatusOK, map[string]string{
		"notificationResponse": "[accepted]",
	})
}
