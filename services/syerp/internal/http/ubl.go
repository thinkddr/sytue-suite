// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"errors"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/efactura"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// UBL 2.1 (EN 16931, perfil Peppol BIS Billing 3.0). La generación es
// apps/nucleo/erp/efactura, la misma del ERP de Sytue Office.

// UBL — GET /api/v1/{modelo}/{id}/ubl — la factura como XML UBL 2.1.
// Solo para los modelos que cobran (facturas de venta): lo que se emite.
func (h *Handlers) UBL(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if m.Contable == nil || !m.Contable.Cobro {
		httpx.Fail(w, r, errNoContable.WithMessage("Only issued invoices can be exported as UBL."))
		return
	}
	reg, err := h.store.Obtener(r.Context(), m.Clave, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	if err := h.efacturaAdmitida(r.Context(), reg); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	org, err := h.store.Organizacion(r.Context())
	if errors.Is(err, store.ErrNoEncontrado) {
		httpx.Fail(w, r, errSinOrganizacion)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var cliente store.Registro
	if id, _ := reg.Data[m.Contable.CampoTercero].(string); id != "" {
		if cliente, err = h.store.Obtener(r.Context(), "contacto", id); err != nil && !errors.Is(err, store.ErrNoEncontrado) {
			httpx.Fail(w, r, err)
			return
		}
	}
	_, urlQR, err := h.registrosDeFactura(r.Context(), reg)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f := facturaDe(reg, m.Contable.CampoBase, m.Contable.CampoIVA, nil)
	f.QR = urlQR
	cuerpo, nombre, err := efactura.UBL(emisorDe(org, false), receptorDe(cliente), f)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": nombre}))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cuerpo)
}

func cadena(v any) string {
	s, _ := v.(string)
	return s
}
