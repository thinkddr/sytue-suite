// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	domain "git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/efactura"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/qr"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// La factura para el cliente, lista para imprimir o guardar en PDF desde el
// navegador. El impreso es apps/nucleo/erp/efactura, el mismo del ERP de Sytue
// Office; aquí se le añade el QR tributario de VERI*FACTU.

type (
	facturaImprimible = efactura.Imprimible
	parteImprimible   = efactura.ParteImprimible
)

// Imprimir — GET /api/v1/{modelo}/{id}/imprimir.
func (h *Handlers) Imprimir(w http.ResponseWriter, r *http.Request) {
	f, _, ok := h.facturaParaCliente(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := f.HTML(w); err != nil {
		httpx.Fail(w, r, err)
	}
}

// facturaParaCliente reúne lo que lleva el impreso de la factura de la ruta: lo
// comparten el HTML para imprimir, el PDF y el envío por correo, para que el
// cliente reciba exactamente lo que se ve al imprimir. Si devuelve false, ya ha
// respondido.
func (h *Handlers) facturaParaCliente(w http.ResponseWriter, r *http.Request) (facturaImprimible, store.Registro, bool) {
	m := h.modeloDe(w, r)
	if m == nil {
		return facturaImprimible{}, store.Registro{}, false
	}
	if m.Clave != "factura_venta" {
		httpx.Fail(w, r, errNoContable.WithMessage("Only customer invoices can be printed."))
		return facturaImprimible{}, store.Registro{}, false
	}
	ctx := r.Context()
	reg, err := h.store.Obtener(ctx, m.Clave, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return facturaImprimible{}, store.Registro{}, false
	}
	org, err := h.store.Organizacion(ctx)
	if errors.Is(err, store.ErrNoEncontrado) {
		httpx.Fail(w, r, errSinOrganizacion)
		return facturaImprimible{}, store.Registro{}, false
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return facturaImprimible{}, store.Registro{}, false
	}
	_, urlQR, err := h.registrosDeFactura(ctx, reg)
	if err != nil {
		httpx.Fail(w, r, err)
		return facturaImprimible{}, store.Registro{}, false
	}
	lineas, err := h.store.LineasDeFactura(ctx, reg.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return facturaImprimible{}, store.Registro{}, false
	}
	simplificada := cadena(reg.Data["tipo_factura"]) == "F2"
	idioma := httpx.LanguageFromRequest(r)
	rot := func(clave string) string { return h.cat.Rotulo(clave, idioma) }
	return h.construirFactura(ctx, reg, org, lineas, simplificada, idioma, rot, urlQR), reg, true
}

// construirFactura traduce el registro, la organización y el cliente a
// efactura.Imprimir y le pone la imagen del QR tributario.
func (h *Handlers) construirFactura(ctx context.Context, reg store.Registro, org store.Organizacion, lineas []domain.LineaFactura, simplificada bool, idioma string, rot func(string) string, urlQR string) facturaImprimible {
	f := facturaDe(reg, "base", "iva", lineas)
	f.Base, _ = domain.Centimos(reg.Data, "base")
	f.IVA, _ = domain.Centimos(reg.Data, "iva")
	if simplificada {
		f.Tipo = "F2"
	} else if f.Tipo == "F2" {
		f.Tipo = ""
	}
	f.QR = urlQR
	var cliente *efactura.Parte
	if id := cadena(reg.Data["cliente"]); id != "" && !simplificada {
		if c, err := h.store.Obtener(ctx, "contacto", id); err == nil {
			texto := func(k string) string { return strings.TrimSpace(cadena(c.Data[k])) }
			cliente = &efactura.Parte{
				Nombre: efactura.Primero(texto("nombre"), c.Etiqueta), NIF: texto("nif"), Direccion: texto("direccion"),
				CP: texto("cp"), Ciudad: texto("ciudad"), Pais: texto("pais"),
			}
		}
	}
	if id := cadena(reg.Data["rectifica"]); id != "" {
		if orig, err := h.store.Obtener(ctx, "factura_venta", id); err == nil {
			f.Rectifica = orig.Ref
		}
	}
	out := efactura.Imprimir(f, emisorDe(org, false), cliente, idioma, rot)
	if urlQR != "" {
		if png, err := qr.PNG(urlQR); err == nil {
			out.PonerQR(png)
		}
	}
	return out
}
