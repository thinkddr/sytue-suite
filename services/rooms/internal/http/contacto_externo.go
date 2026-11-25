// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

// La política de la organización para el contacto con gente de fuera (mig 0083). La
// guarda quien administra (owner o admin, por el grupo de rutas) desde Sytue Admin.

// PoliticaContactoExterno — GET /api/v1/admin/external-contact — { mode, trusted }.
func (h *Handlers) PoliticaContactoExterno(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.PoliticaContactoExterno(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

// GuardarPoliticaContactoExterno — PUT /api/v1/admin/external-contact — la guarda y la
// devuelve como queda (la lista de confianza, limpia).
func (h *Handlers) GuardarPoliticaContactoExterno(w http.ResponseWriter, r *http.Request) {
	var in domain.PoliticaContactoExterno
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	p, err := h.svc.GuardarPoliticaContactoExterno(r.Context(), in, sub)
	var mala *service.OrganizacionNoValida
	switch {
	case errors.As(err, &mala):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("rooms_external_policy_org").
			WithMessage("That is not an organisation name: "+mala.Org).WithDetails(map[string]string{"org": mala.Org}))
		return
	case errors.Is(err, domain.ErrInvalidInput):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(`"mode" must be "abierta", "cerrada" or "confianza", with up to 100 trusted organisations.`))
		return
	case err != nil:
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}
