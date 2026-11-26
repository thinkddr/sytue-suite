// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	domain "git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/efactura"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// Facturae 3.2.2 sin firmar. La generación es apps/nucleo/erp/efactura, la misma
// del ERP de Sytue Office; aquí solo se traducen el registro, la organización y el
// contacto del almacén a sus tipos, y se añade el QR de VERI*FACTU.

// Facturae — GET /api/v1/{modelo}/{id}/facturae — la factura emitida como
// Facturae 3.2.2 sin firmar.
func (h *Handlers) Facturae(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if m.Contable == nil || !m.Contable.Cobro {
		httpx.Fail(w, r, errNoContable.WithMessage("Only issued invoices can be exported as Facturae."))
		return
	}
	reg, err := h.store.Obtener(r.Context(), m.Clave, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	if cadena(reg.Data["tipo_factura"]) == "F2" {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage(efactura.ErrSimplificada.Error()))
		return
	}
	lineas, err := h.store.LineasDeFactura(r.Context(), reg.ID)
	if err != nil {
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
	personaFisica := false
	if pais, ok := h.obligaciones.Pais(org.Pais); ok {
		if forma, ok := pais.Forma(org.Forma); ok {
			personaFisica = forma.Tipo == "persona_fisica"
		}
	}
	_, urlQR, err := h.registrosDeFactura(r.Context(), reg)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f := facturaDe(reg, m.Contable.CampoBase, m.Contable.CampoIVA, lineas)
	f.QR = urlQR
	doc, err := efactura.Facturae(emisorDe(org, personaFisica), receptorDe(cliente), f)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage(err.Error()))
		return
	}
	cuerpo, err := doc.Bytes()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": doc.Nombre()}))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cuerpo)
}

// efacturaAdmitida dice si la factura puede salir como factura electrónica
// (Facturae o UBL): efactura.Admitida con las líneas del almacén.
func (h *Handlers) efacturaAdmitida(ctx context.Context, reg store.Registro) error {
	lineas, err := h.store.LineasDeFactura(ctx, reg.ID)
	if err != nil {
		return err
	}
	if err := efactura.Admitida(efactura.Factura{Tipo: cadena(reg.Data["tipo_factura"]), Lineas: lineas}); err != nil {
		return httpx.ErrUnprocessable.WithMessage(err.Error())
	}
	return nil
}

// facturaDe traduce un registro de factura a efactura.Factura. Base e IVA salen de
// los campos contables del modelo.
func facturaDe(reg store.Registro, campoBase, campoIVA string, lineas []domain.LineaFactura) efactura.Factura {
	base, _ := centimos(reg.Data[campoBase])
	iva, _ := centimos(reg.Data[campoIVA])
	return efactura.Factura{
		ID: reg.ID, Numero: reg.Ref, Fecha: cadena(reg.Data["fecha"]), Vencimiento: cadena(reg.Data["vencimiento"]),
		Notas: cadena(reg.Data["notas"]), Estado: reg.Estado, Tipo: cadena(reg.Data["tipo_factura"]),
		RegimenIVA: cadena(reg.Data["regimen_iva"]), Base: base, IVA: iva, Lineas: lineas,
		Rectificativa: cadena(reg.Data["rectifica"]) != "",
	}
}

// emisorDe es la organización como emisor.
func emisorDe(org store.Organizacion, fisica bool) efactura.Parte {
	return efactura.Parte{Nombre: org.Nombre, NIF: org.NIF, Direccion: org.Direccion, CP: org.CP, Ciudad: org.Ciudad, Pais: org.Pais, Fisica: fisica}
}

// receptorDe es el contacto como receptor de una factura electrónica: con su
// etiqueta por nombre. nil si la factura no tiene cliente.
func receptorDe(c store.Registro) *efactura.Parte {
	if c.ID == "" {
		return nil
	}
	texto := func(k string) string { return strings.TrimSpace(cadena(c.Data[k])) }
	return &efactura.Parte{
		Nombre: c.Etiqueta, NIF: texto("nif"), Direccion: texto("direccion"), CP: texto("cp"), Ciudad: texto("ciudad"),
		Pais: texto("pais"), Correo: texto("correo"), Fisica: texto("tipo") == "persona",
		DIR3: [3]string{texto("dir3_contable"), texto("dir3_gestor"), texto("dir3_tramitadora")},
	}
}

// facturaeDe arma el documento con los tipos del almacén, de una persona jurídica y con
// los campos «base» e «iva» (lo usan las pruebas).
func facturaeDe(org store.Organizacion, reg, cliente store.Registro, lineas ...domain.LineaFactura) (efactura.FacturaeDoc, error) {
	return efactura.Facturae(emisorDe(org, false), receptorDe(cliente), facturaDe(reg, "base", "iva", lineas))
}
