// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// pedidoTienda es el contrato privado Pages → SyERP. Los importes son siempre
// céntimos: usar float aquí convertiría un cobro de 29,10 € en una discrepancia
// fiscal según la representación binaria del cliente.
type pedidoTienda struct {
	Tenant    string `json:"tenant"`
	Origen    string `json:"origen"`
	Modo      string `json:"modo"`
	PedidoID  string `json:"pedido_id"`
	Numero    string `json:"numero"`
	Fecha     string `json:"fecha"`
	Divisa    string `json:"divisa"`
	Factura   string `json:"factura"`
	Comprador struct {
		Nombre    string `json:"nombre"`
		Correo    string `json:"correo"`
		NIF       string `json:"nif"`
		Direccion struct {
			Pais string `json:"pais"`
			CP   string `json:"cp"`
		} `json:"direccion"`
	} `json:"comprador"`
	Lineas        []lineaPedidoTienda `json:"lineas"`
	Envio         *lineaPedidoTienda  `json:"envio"`
	TotalCentimos int64               `json:"total_centimos"`
	Cobro         struct {
		Pasarela        string `json:"pasarela"`
		Referencia      string `json:"referencia"`
		Fecha           string `json:"fecha"`
		ImporteCentimos int64  `json:"importe_centimos"`
	} `json:"cobro"`
}

type lineaPedidoTienda struct {
	SKU                    string `json:"sku"`
	Nombre                 string `json:"nombre"`
	Cantidad               int64  `json:"cantidad"`
	PrecioUnitarioCentimos int64  `json:"precio_unitario_centimos"`
	// PrecioCentimos es el importe completo del envío. El contrato de Pages
	// no lo modela como cantidad × precio unitario, a diferencia de una línea.
	PrecioCentimos int64 `json:"precio_centimos"`
	TipoIVA        int   `json:"tipo_iva"`
}

// validarPedidoTienda rechaza antes de tocar la base los estados que nunca
// pueden producir una factura española válida.
func validarPedidoTienda(p pedidoTienda) error {
	if err := validarCabeceraPedido(p); err != nil {
		return err
	}
	if err := validarCompradorPedido(p); err != nil {
		return err
	}
	return validarLineasPedido(p)
}

func validarCabeceraPedido(p pedidoTienda) error {
	if p.Origen != "pages" || p.Modo != "live" || p.Tenant == "" || p.PedidoID == "" || p.Numero == "" {
		return fmt.Errorf("origen, modo, tenant, pedido_id y numero son obligatorios")
	}
	if p.Divisa != "EUR" || p.TotalCentimos <= 0 || p.TotalCentimos != p.Cobro.ImporteCentimos {
		return fmt.Errorf("el cobro debe coincidir exactamente con un total positivo en EUR")
	}
	if _, err := time.Parse("2006-01-02", p.Fecha); err != nil {
		return fmt.Errorf("fecha: %w", err)
	}
	if _, err := time.Parse(time.RFC3339, p.Cobro.Fecha); err != nil {
		return fmt.Errorf("fecha de cobro: %w", err)
	}
	if p.Factura != "simplificada" && p.Factura != "completa" {
		return fmt.Errorf("factura debe ser simplificada o completa")
	}
	return nil
}

func validarCompradorPedido(p pedidoTienda) error {
	if p.Factura == "completa" && !validation.NIF(p.Comprador.NIF) {
		return fmt.Errorf("NIF del comprador inválido")
	}
	if p.Factura == "simplificada" {
		if p.Comprador.Direccion.Pais != "ES" || esTerritorioNoPeninsular(p.Comprador.Direccion.CP) || p.TotalCentimos > 300000 {
			return fmt.Errorf("la factura simplificada solo admite España peninsular/Baleares y hasta 3.000 €")
		}
	}
	return nil
}

func validarLineasPedido(p pedidoTienda) error {
	total := int64(0)
	for _, l := range p.Lineas {
		if l.Nombre == "" || l.Cantidad <= 0 || l.PrecioUnitarioCentimos < 0 || !ivaAdmitido(l.TipoIVA) {
			return fmt.Errorf("línea de pedido inválida")
		}
		total += l.Cantidad * l.PrecioUnitarioCentimos
	}
	if p.Envio != nil {
		if p.Envio.Nombre == "" || p.Envio.PrecioCentimos < 0 || !ivaAdmitido(p.Envio.TipoIVA) {
			return fmt.Errorf("línea de envío inválida")
		}
		total += p.Envio.PrecioCentimos
	}
	if len(p.Lineas) == 0 || total != p.TotalCentimos {
		return fmt.Errorf("las líneas y el envío no suman el total")
	}
	return nil
}

func ivaAdmitido(v int) bool { return v == 0 || v == 4 || v == 5 || v == 10 || v == 21 }
func esTerritorioNoPeninsular(cp string) bool {
	return strings.HasPrefix(cp, "35") || strings.HasPrefix(cp, "38") || strings.HasPrefix(cp, "51") || strings.HasPrefix(cp, "52")
}

func (h *Handlers) handlePedidosTienda(w http.ResponseWriter, r *http.Request) {
	var in pedidoTienda
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := validarPedidoTienda(in); err != nil {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("order_invalid").WithMessage(err.Error()))
		return
	}

	t, err := tenancy.New(in.Tenant)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_tenant").Wrap(err))
		return
	}
	ctx := tenancy.Into(r.Context(), t)

	sub := "pages"
	if c, ok := authn.FromContext(r.Context()); ok && c.Subject != "" {
		sub = c.Subject
	}

	var input store.PedidoTiendaInput
	input.Tenant = in.Tenant
	input.Origen = in.Origen
	input.Modo = in.Modo
	input.PedidoID = in.PedidoID
	input.Numero = in.Numero
	input.Fecha = in.Fecha
	input.Divisa = in.Divisa
	input.Factura = in.Factura
	input.Comprador.Nombre = in.Comprador.Nombre
	input.Comprador.Correo = in.Comprador.Correo
	input.Comprador.NIF = in.Comprador.NIF
	input.Comprador.Direccion.Pais = in.Comprador.Direccion.Pais
	input.Comprador.Direccion.CP = in.Comprador.Direccion.CP

	input.TotalCentimos = in.TotalCentimos
	input.Cobro.Pasarela = in.Cobro.Pasarela
	input.Cobro.Referencia = in.Cobro.Referencia
	input.Cobro.Fecha = in.Cobro.Fecha
	input.Cobro.ImporteCentimos = in.Cobro.ImporteCentimos
	for _, l := range in.Lineas {
		input.Lineas = append(input.Lineas, struct {
			SKU                    string
			Nombre                 string
			Cantidad               int64
			PrecioUnitarioCentimos int64
			TipoIVA                int
		}{
			SKU:                    l.SKU,
			Nombre:                 l.Nombre,
			Cantidad:               l.Cantidad,
			PrecioUnitarioCentimos: l.PrecioUnitarioCentimos,
			TipoIVA:                l.TipoIVA,
		})
	}
	if in.Envio != nil {
		input.Envio = &struct {
			Nombre         string
			PrecioCentimos int64
			TipoIVA        int
		}{
			Nombre:         in.Envio.Nombre,
			PrecioCentimos: in.Envio.PrecioCentimos,
			TipoIVA:        in.Envio.TipoIVA,
		}
	}

	res, err := h.store.ProcesarPedidoTienda(ctx, input, sub)
	if err != nil {
		if errors.Is(err, store.ErrModulosDesactivados) {
			httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "modules_disabled", "Ventas and facturacion modules are disabled"))
			return
		}
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

func (h *Handlers) handlePedidosTiendaFactura(w http.ResponseWriter, r *http.Request) {
	pedidoID := chi.URLParam(r, "pedido_id")
	if pedidoID == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("missing_order_id"))
		return
	}
	tenantSlug := r.URL.Query().Get("tenant")
	if tenantSlug == "" {
		tenantSlug = r.Header.Get("X-Sytue-Tenant")
	}
	if tenantSlug == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("missing_tenant"))
		return
	}
	t, err := tenancy.New(tenantSlug)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_tenant").Wrap(err))
		return
	}
	ctx := tenancy.Into(r.Context(), t)

	reg, lineas, simplificada, err := h.store.FacturaPorPedidoTienda(ctx, pedidoID)
	if err != nil {
		if errors.Is(err, store.ErrNoEncontrado) {
			httpx.Fail(w, r, httpx.ErrNotFound.WithCode("order_not_found"))
			return
		}
		httpx.Fail(w, r, err)
		return
	}

	org, err := h.store.Organizacion(ctx)
	if errors.Is(err, store.ErrNoEncontrado) {
		httpx.Fail(w, r, errSinOrganizacion)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	_, urlQR, err := h.registrosDeFactura(ctx, reg)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	idioma := httpx.LanguageFromRequest(r)
	rot := func(clave string) string { return h.cat.Rotulo(clave, idioma) }

	f := h.construirFactura(ctx, reg, org, lineas, simplificada, idioma, rot, urlQR)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := plantillaFactura.Execute(w, f); err != nil {
		httpx.Fail(w, r, err)
	}
}
