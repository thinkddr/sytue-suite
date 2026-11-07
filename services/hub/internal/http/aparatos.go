// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/apns"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/hub/internal/despertar"
)

// # El registro de aparatos de iOS (ADR 0039 y 0078, enmiendas del 2026-11-06)
//
// Las apps nativas de iOS registran aquí el token que les da APNs, con su bundle, para
// que el Hub las despierte cuando cambia algo de su producto (internal/despertar). Es
// el ÚNICO registro de la suite: Rooms tuvo el suyo (`push_apns`) y sus rutas
// redirigen aquí (308).
//
// La app solo se registra con el interruptor «Actualizar los widgets con la app
// cerrada» encendido y un widget puesto, y se olvida al salir, al poner el bloqueo, al
// apagar el interruptor y al quitar el último widget.

// registroAparatos es lo que se usa del store (store.Store).
type registroAparatos interface {
	RegistrarAparato(ctx context.Context, subject, bundle, token string) error
	OlvidarAparatos(ctx context.Context, subject, token string) (int64, error)
}

// despertadorCambios recibe la señal de cambios (despertar.Despertador).
type despertadorCambios interface {
	Cambio(ctx context.Context, tenant, subject, producto string)
}

type aparatoRequest struct {
	Bundle string `json:"bundle"`
	Token  string `json:"token"`
}

// RegistrarAparato atiende POST /api/v1/me/push/apns {bundle, token}: 204.
//
// Un token DELEGADO (`act`) no registra: quien actúa en nombre de otra persona no
// puede hacer que su aparato se despierte con los cambios de ella.
func (h *Handlers) RegistrarAparato(w http.ResponseWriter, r *http.Request) {
	if h.aparatos == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if c, ok := authn.FromContext(r.Context()); !ok || c.Delegado {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	var req aparatoRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req.Bundle, req.Token = strings.TrimSpace(req.Bundle), strings.TrimSpace(req.Token)
	if !despertar.BundleConocido(req.Bundle) || !apns.TokenValido(req.Token) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A known bundle and a device token are required."))
		return
	}
	if err := h.aparatos.RegistrarAparato(r.Context(), subFromCtx(r), req.Bundle, req.Token); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// OlvidarAparatos atiende POST /api/v1/me/push/apns/forget: con {token}, borra ese
// aparato de la persona; SIN CUERPO (o sin token), todos sus aparatos en esta
// organización. 204 aunque no hubiera ninguno. Es la ruta que promete el ADR 0039.
//
// Un token DELEGADO tampoco olvida: sin cuerpo borraría todos los iPhone de la persona
// representada, y quien la representa no registra ninguno (RegistrarAparato).
func (h *Handlers) OlvidarAparatos(w http.ResponseWriter, r *http.Request) {
	if h.aparatos == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if c, ok := authn.FromContext(r.Context()); !ok || c.Delegado {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	crudo, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Body too large."))
		return
	}
	if len(bytes.TrimSpace(crudo)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(crudo))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid JSON."))
			return
		}
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token != "" && !apns.TokenValido(req.Token) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid device token."))
		return
	}
	if _, err := h.aparatos.OlvidarAparatos(r.Context(), subFromCtx(r), req.Token); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}
