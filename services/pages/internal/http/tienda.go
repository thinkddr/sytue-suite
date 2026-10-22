// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// montarTienda registra los ajustes, el catálogo y las pasarelas de la tienda (§6.3, §10.2-10.3).
func montarTienda(g grupos, h *Handlers) {
	g.api.Route("/sites/{id}/store", func(r chi.Router) {
		// Abrir o cerrar la venta y sus condiciones (envíos, condiciones legales, a quién
		// responde el comprador y quién recibe los avisos) son de quien administra la
		// organización, como la pasarela y los pedidos (WEB-02). Un miembro lleva el catálogo.
		soloAdmin := h.Verifier.RequireRole("owner", "admin")
		r.Get("/settings", h.obtenerAjustesTienda)
		r.With(soloAdmin).Put("/settings", h.guardarAjustesTienda)
		r.With(soloAdmin).Post("/enable", h.activarTienda)
		r.With(soloAdmin).Post("/disable", h.desactivarTienda)

		r.Get("/products", h.listarProductosTienda)
		r.Post("/products", h.crearProductoTienda)
		r.Get("/products/{productId}", h.obtenerProductoTienda)
		r.Put("/products/{productId}", h.actualizarProductoTienda)
		r.Delete("/products/{productId}", h.eliminarProductoTienda)

		// La pasarela decide a qué cuenta va el dinero de la tienda.
		r.Get("/gateways", h.listarPasarelasTienda)
		r.With(soloAdmin).Post("/gateways", h.guardarPasarelaTienda)
		r.With(soloAdmin).Delete("/gateways/{provider}", h.desactivarPasarelaTienda)
		r.Get("/gateways/{provider}/webhook-url", h.webhookURLPasarelaTienda)
		r.Get("/orders", h.listarPedidosTienda)
		r.Get("/orders.csv", h.exportarPedidosCSV)
	})
}

func (h *Handlers) listarPedidosTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	t, _ := tenancy.From(r.Context())
	orders, err := h.Store.ListarPedidos(r.Context(), t.Slug, siteID, limit)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	if orders == nil {
		orders = []domain.Order{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"orders": orders})
}

// exportarPedidosCSV entrega un CSV UTF-8 deliberadamente pequeño: no exporta
// dirección ni documento fiscal para evitar que el fichero sea un volcado de PII.
func (h *Handlers) exportarPedidosCSV(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	t, _ := tenancy.From(r.Context())
	orders, err := h.Store.ListarPedidos(r.Context(), t.Slug, siteID, 1000)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=orders.csv")
	w.Header().Set("Cache-Control", "no-store")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"number", "status", "test", "created_at", "buyer_email", "currency", "total_cents", "erp_invoice_ref"})
	for _, order := range orders {
		_ = cw.Write(filaDePedidoCSV(order))
	}
	cw.Flush()
}

// filaDePedidoCSV es la fila de un pedido en orders.csv.
//
// Todo lo que es texto pasa por validation.CSVSafe. El correo de quien compra lo
// escribe una persona sin sesión en el checkout público y nadie lo valida; quien
// abre este fichero es un administrador, en una hoja de cálculo, que ejecuta como
// fórmula una celda que empiece por = + - o @. Los números y la fecha los formatea
// este código y se quedan como están: un importe no puede llevar el apóstrofo.
func filaDePedidoCSV(order domain.Order) []string {
	s := validation.CSVSafe
	return []string{
		s(order.Number), s(order.Status), strconv.FormatBool(order.Test),
		order.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		s(order.BuyerEmail), s(order.Currency), strconv.Itoa(order.TotalCents), s(order.ERPInvoiceRef),
	}
}

func idProducto(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "productId"))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithCode("invalid_product_id").Wrap(err)
	}
	return id, nil
}

func (h *Handlers) obtenerAjustesTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	settings, err := h.Store.ObtenerAjustesTienda(r.Context(), t.Slug, siteID)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, settings)
}

func (h *Handlers) guardarAjustesTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in domain.StoreSettings
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json").Wrap(err))
		return
	}
	in.SiteID = siteID
	t, _ := tenancy.From(r.Context())
	in.Tenant = t.Slug

	if err := h.Store.GuardarAjustesTienda(r.Context(), t.Slug, in); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handlers) activarTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	if err := h.Store.ActivarTienda(r.Context(), t.Slug, siteID); err != nil {
		if errors.Is(err, domain.ErrTiendaNoLista) {
			httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("store_not_ready"))
			return
		}
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]bool{"enabled": true})
}

func (h *Handlers) desactivarTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	if err := h.Store.DesactivarTienda(r.Context(), t.Slug, siteID); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]bool{"enabled": false})
}

func (h *Handlers) listarProductosTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	prods, err := h.Store.ListarProductos(r.Context(), t.Slug, siteID)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	if prods == nil {
		prods = []domain.Product{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"products": prods})
}

func (h *Handlers) crearProductoTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var p domain.Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json").Wrap(err))
		return
	}
	p.SiteID = siteID
	t, _ := tenancy.From(r.Context())
	p.Tenant = t.Slug

	if err := h.Store.CrearProducto(r.Context(), t.Slug, &p); err != nil {
		if errors.Is(err, domain.ErrProductoInvalido) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_product").Wrap(err))
			return
		}
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, p)
}

func (h *Handlers) obtenerProductoTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	prodID, err := idProducto(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	p, err := h.Store.ObtenerProducto(r.Context(), t.Slug, siteID, prodID)
	if err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			httpx.Fail(w, r, httpx.ErrNotFound.WithCode("product_not_found"))
			return
		}
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

func (h *Handlers) actualizarProductoTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	prodID, err := idProducto(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var p domain.Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json").Wrap(err))
		return
	}
	p.ID = prodID
	p.SiteID = siteID
	t, _ := tenancy.From(r.Context())
	p.Tenant = t.Slug

	if err := h.Store.ActualizarProducto(r.Context(), t.Slug, &p); err != nil {
		if errors.Is(err, domain.ErrProductoInvalido) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_product").Wrap(err))
			return
		}
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

func (h *Handlers) eliminarProductoTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	prodID, err := idProducto(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	if err := h.Store.EliminarProducto(r.Context(), t.Slug, siteID, prodID); err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			httpx.Fail(w, r, httpx.ErrNotFound.WithCode("product_not_found"))
			return
		}
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *Handlers) listarPasarelasTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	gateways, err := h.Store.ListarPasarelas(r.Context(), t.Slug, siteID)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	if gateways == nil {
		gateways = []domain.Gateway{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"gateways": gateways})
}

type peticionGuardarPasarela struct {
	Provider       string            `json:"provider"`
	Mode           string            `json:"mode"`
	Credentials    map[string]string `json:"credentials"`
	PublicConfig   map[string]any    `json:"public_config"`
	NoticeAccepted bool              `json:"notice_accepted"`
}

func (h *Handlers) guardarPasarelaTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req peticionGuardarPasarela
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json").Wrap(err))
		return
	}

	t, _ := tenancy.From(r.Context())
	usuario := sub(r)

	g, err := h.Tienda.ProbarYGuardarPasarela(
		r.Context(),
		t.Slug,
		siteID,
		usuario,
		req.Provider,
		req.Mode,
		req.Credentials,
		req.PublicConfig,
		req.NoticeAccepted,
	)
	if err != nil {
		if errors.Is(err, domain.ErrCredencialesPasarela) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_gateway_credentials").Wrap(err))
			return
		}
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	httpx.JSON(w, r, http.StatusOK, g)
}

func (h *Handlers) desactivarPasarelaTienda(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	provider := chi.URLParam(r, "provider")
	t, _ := tenancy.From(r.Context())
	if err := h.Store.DesactivarPasarela(r.Context(), t.Slug, siteID, provider); err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			httpx.Fail(w, r, httpx.ErrNotFound.WithCode("gateway_not_found"))
			return
		}
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]bool{"active": false})
}

func (h *Handlers) webhookURLPasarelaTienda(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	url := h.Tienda.WebhookURL(provider)
	httpx.JSON(w, r, http.StatusOK, map[string]string{"webhook_url": url})
}
