// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package meethttp

import (
	"context"
	"io"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// maxWebhookBytes limita el tamaño del cuerpo de un webhook (defensa básica).
const maxWebhookBytes = 1 << 20 // 1 MiB

// webhookSink es la superficie mínima del servicio que consume el webhook. La
// satisface *service.Service; declararla como interfaz permite inyectar un fake
// en los tests HTTP.
type webhookSink interface {
	HandleWebhookEvent(ctx context.Context, ev livekit.WebhookEvent) error
}

// WebhookHandler recibe los webhooks de LiveKit. La autenticidad se verifica por
// la firma HMAC del cuerpo (cabecera Authorization: un JWT firmado con el
// api_secret), NO por OIDC: LiveKit no porta tokens de Sytue ID.
type WebhookHandler struct {
	svc    webhookSink
	signer *livekit.Signer
}

// Receive procesa un webhook de LiveKit: verifica firma + hash del cuerpo,
// decodifica el evento, resuelve el tenant y actualiza el estado en PostgreSQL.
func (h *WebhookHandler) Receive(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBytes))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The webhook body could not be read."))
		return
	}

	// La firma del webhook viaja en Authorization (un JWT HS256 cuyo claim
	// sha256 debe corresponder al hash del cuerpo). Esto autentica el origen.
	authToken := r.Header.Get("Authorization")
	ev, err := h.signer.ParseWebhook(authToken, body)
	if err != nil {
		log.From(r.Context()).Warn("meet: webhook de LiveKit rechazado", "error", err)
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Unauthenticated webhook.").Wrap(err))
		return
	}

	if err := h.svc.HandleWebhookEvent(r.Context(), ev); err != nil {
		// 500 para que LiveKit reintente la entrega del webhook.
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}
