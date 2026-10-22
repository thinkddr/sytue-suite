// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package syerphttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func (h *Handlers) CobrarTPV(w http.ResponseWriter, r *http.Request) {
	for _, modulo := range []string{"tpv", "facturacion"} {
		if !h.puede(r.Context(), modulo, permisos.Usuario) || !h.puedeVerbo(r.Context(), modulo, permisos.Crear) {
			httpx.Fail(w, r, errSinPermiso)
			return
		}
	}
	for _, modulo := range []string{"productos", "contactos"} {
		if !h.puede(r.Context(), modulo, permisos.Lectura) {
			httpx.Fail(w, r, errSinPermiso)
			return
		}
	}
	if !h.puedeVerbo(r.Context(), "facturacion", permisos.Pagar) {
		httpx.Fail(w, r, errSinVerbo(permisos.Pagar))
		return
	}
	if !h.puedeVerbo(r.Context(), "facturacion", permisos.Asentar) {
		httpx.Fail(w, r, errSinVerbo(permisos.Asentar))
		return
	}
	var in store.CobroTPV
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := h.store.CobrarTPV(r.Context(), h.cat, in, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

func (h *Handlers) ConvertirVenta(w http.ResponseWriter, r *http.Request) {
	modelo := chi.URLParam(r, "modelo")
	destino := "ventas"
	if modelo == "pedido_venta" {
		destino = "facturacion"
	} else if modelo != "presupuesto" {
		httpx.Fail(w, r, errAccionInvalida)
		return
	}
	if !h.puedeVerbo(r.Context(), "ventas", permisos.Crear) || !h.puede(r.Context(), destino, permisos.Usuario) || !h.puedeVerbo(r.Context(), destino, permisos.Crear) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	out, err := h.store.ConvertirVenta(r.Context(), h.cat, modelo, chi.URLParam(r, "id"), sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

func (h *Handlers) PresupuestoTPV(w http.ResponseWriter, r *http.Request) {
	if !h.puede(r.Context(), "ventas", permisos.Usuario) || !h.puedeVerbo(r.Context(), "ventas", permisos.Crear) || !h.puede(r.Context(), "productos", permisos.Lectura) || !h.puede(r.Context(), "contactos", permisos.Lectura) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	var in store.CobroTPV
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := h.store.PresupuestoTPV(r.Context(), h.cat, in, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
