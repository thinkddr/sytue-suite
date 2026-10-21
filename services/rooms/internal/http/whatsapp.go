// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// maxWhatsAppBody acota el aviso de Meta antes de leerlo: trae texto, no ficheros.
const maxWhatsAppBody = 1 << 20

// VerifyWhatsApp — GET /webhooks/whatsapp/{token} — la comprobación de Meta al dar
// de alta el aviso: devuelve hub.challenge si el token de verificación es el de la URL.
func (ih *intHandlers) VerifyWhatsApp(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if !ih.svc.VerifyWhatsAppSubscription(r.Context(), chi.URLParam(r, "token"), q.Get("hub.mode"), q.Get("hub.verify_token")) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, q.Get("hub.challenge"))
}

// ReceiveWhatsApp — POST /webhooks/whatsapp/{token} — un aviso de mensajes de Meta.
// Sin JWT: el token de la URL encuentra la cuenta y la firma prueba que viene de Meta.
func (ih *intHandlers) ReceiveWhatsApp(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWhatsAppBody))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Body too large."))
		return
	}
	err = ih.svc.ReceiveWhatsApp(r.Context(), chi.URLParam(r, "token"), body, r.Header.Get("X-Hub-Signature-256"))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.Fail(w, r, httpx.ErrNotFound)
	case errors.Is(err, domain.ErrForbidden):
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Invalid signature."))
	case errors.Is(err, domain.ErrInvalidInput):
		httpx.Fail(w, r, httpx.ErrBadRequest)
	case err != nil:
		// 5xx a propósito: Meta reintenta, y el mensaje no se pierde.
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
	default:
		httpx.JSON(w, r, http.StatusOK, map[string]any{})
	}
}

// ListWhatsApp — GET /whatsapp — números conectados (admin del workspace).
func (ih *intHandlers) ListWhatsApp(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	cuentas, err := ih.svc.ListWhatsApp(r.Context(), actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if cuentas == nil {
		cuentas = []domain.WhatsAppAccount{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"accounts": cuentas})
}

// DisconnectWhatsApp — DELETE /whatsapp/{id} — desconecta un número.
func (ih *intHandlers) DisconnectWhatsApp(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := ih.svc.DisconnectWhatsApp(r.Context(), id, actor); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}
