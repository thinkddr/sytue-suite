// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

type ctxKeyPublico int

const (
	ctxKeyTenant ctxKeyPublico = iota
	ctxKeySiteID
	ctxKeySitio
)

var reCustomScheme = regexp.MustCompile(`^[a-z][a-z0-9+.-]{2,30}$`)

var esquemasProhibidos = map[string]bool{
	"http":       true,
	"https":      true,
	"javascript": true,
	"data":       true,
	"file":       true,
}

// montarPublicoTienda registra lo público de la tienda en /_pages: ALTCHA, catálogo, checkout, pago y pedidos (§6.4, §9.5).
func montarPublicoTienda(g grupos, h *Handlers) {
	// Middleware de origen y CORS exacto para /_pages/* (§9.5)
	g.publico.Use(h.origenYCorsMiddleware)

	g.publico.Options("/*", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	g.publico.Get("/altcha", h.publicoAltcha)
	g.publico.Get("/catalog", h.publicoCatalogo)
	g.publico.Get("/products", h.publicoProductos)
	g.publico.Get("/forms/{form_id}", h.publicoDefinicionFormulario)
	g.publico.Post("/forms/{form_id}", h.publicoEnviarFormulario)
	// Cinco checkouts por visitante y sitio, y uno más cada 7 minutos: lo que dura una
	// reserva (35 min) para cinco pedidos (WEB-06).
	var checkouts httpx.RateLimiter
	if h.Store != nil {
		checkouts = pg.NewRateLimiter(h.Store.DB(), pg.RateLimiterConfig{PerSecond: 5.0 / (35 * 60), Burst: 5})
	}
	g.publico.With(porVisitante(checkouts, "checkout")).Post("/checkout", h.publicoCheckout)
	g.publico.Post("/pay/{payment_id}", h.publicoPagar)
	g.publico.Get("/orders/{id}", h.publicoConsultarPedido)
	g.publico.Post("/orders/{id}/invoice", h.publicoSolicitarFactura)
	g.publico.Post("/withdrawals", h.publicoDesistimiento)
	g.publico.Post("/orders/{id}/withdrawal", h.publicoDesistimientoPedido)
}

func (h *Handlers) origenYCorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		host := r.Host
		if colon := strings.IndexByte(host, ':'); colon != -1 {
			host = host[:colon]
		}

		if h.Store == nil {
			next.ServeHTTP(w, r)
			return
		}

		tenant, siteID, err := h.Store.ResolverHostPublico(ctx, host, h.Cfg.SitesDomain)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrNotFound.WithCode("site_not_found"))
			return
		}

		sitio, err := h.Store.ObtenerSitioPublico(ctx, tenant, siteID)
		if err != nil || sitio == nil {
			httpx.Fail(w, r, httpx.ErrNotFound.WithCode("site_not_found"))
			return
		}

		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			if !validarOrigenPermitido(origin, host, sitio) {
				httpx.Fail(w, r, httpx.ErrForbidden.WithCode("origin_invalid"))
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Sytue-Pedido")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		ctx = context.WithValue(ctx, ctxKeyTenant, tenant)
		ctx = context.WithValue(ctx, ctxKeySiteID, siteID)
		ctx = context.WithValue(ctx, ctxKeySitio, sitio)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// porVisitante limita una ruta de /_pages por (sitio, IP) con un cubo compartido entre
// réplicas, pero solo cuando la IP es de verdad la de la persona. Detrás del túnel del
// piloto todas las peticiones traen la de un proxy nuestro, y el cubo sería un tope para
// todo el mundo a la vez (PAGES-PLAN A14): ahí solo quedan los topes por comprador.
func porVisitante(l httpx.RateLimiter, ambito string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limitado := httpx.RateLimit(httpx.RateLimitConfig{Limiter: l, KeyFunc: func(r *http.Request) string {
			siteID, _ := r.Context().Value(ctxKeySiteID).(uuid.UUID)
			return ambito + ":" + siteID.String() + ":" + ipDeVisitante(r)
		}})(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if l == nil || !clientip.TrustedProxiesFrom(r.Context()).Identifica(ipDeVisitante(r)) {
				next.ServeHTTP(w, r)
				return
			}
			limitado.ServeHTTP(w, r)
		})
	}
}

func ipDeVisitante(r *http.Request) string {
	return clientip.TrustedProxiesFrom(r.Context()).ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
}

func validarOrigenPermitido(origin, host string, sitio *domain.SitioCompleto) bool {
	canonicalWebHTTPS := "https://" + host
	canonicalWebHTTP := "http://" + host
	if strings.EqualFold(origin, canonicalWebHTTPS) || strings.EqualFold(origin, canonicalWebHTTP) {
		return true
	}

	for _, o := range sitio.ExportOrigins {
		o = strings.TrimSpace(o)
		if o != "" && strings.EqualFold(origin, o) {
			return true
		}
	}

	// Admitir http://localhost:<puerto>
	if strings.HasPrefix(origin, "http://localhost:") || origin == "http://localhost" {
		return true
	}

	return false
}

func (h *Handlers) publicoAltcha(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}
	ch, err := h.Tienda.EmitirAltcha()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("altcha_failed"))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ch)
}

func (h *Handlers) publicoCatalogo(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}
	tenant, _ := r.Context().Value(ctxKeyTenant).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(uuid.UUID)

	rawSKUs := r.URL.Query().Get("skus")
	skus := strings.Split(rawSKUs, ",")

	items, err := h.Tienda.ObtenerCatalogoPublico(r.Context(), tenant, siteID, skus)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("catalog_failed"))
		return
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) publicoProductos(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}
	tenant, _ := r.Context().Value(ctxKeyTenant).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(uuid.UUID)
	sitio, _ := r.Context().Value(ctxKeySitio).(*domain.SitioCompleto)

	lang := r.URL.Query().Get("lang")
	cursor := r.URL.Query().Get("cursor")

	canonicalHost := r.Host
	if sitio != nil && sitio.CanonicalHost != nil && *sitio.CanonicalHost != "" {
		canonicalHost = *sitio.CanonicalHost
	}

	items, nextCursor, err := h.Tienda.ListarProductosPublicos(r.Context(), tenant, siteID, canonicalHost, lang, cursor)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("products_failed"))
		return
	}

	var nc any
	if nextCursor != "" {
		nc = nextCursor
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": nc,
	})
}

func (h *Handlers) publicoDefinicionFormulario(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}
	tenant, _ := r.Context().Value(ctxKeyTenant).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(uuid.UUID)
	formID := chi.URLParam(r, "form_id")

	dto, err := h.Tienda.ObtenerFormularioPublico(r.Context(), tenant, siteID, formID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("form_not_found"))
		return
	}

	httpx.JSON(w, r, http.StatusOK, dto)
}

func (h *Handlers) publicoEnviarFormulario(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}
	tenant, _ := r.Context().Value(ctxKeyTenant).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(uuid.UUID)
	formID := chi.URLParam(r, "form_id")

	var req struct {
		Fields   map[string]any `json:"fields"`
		Consent  bool           `json:"consent"`
		Language string         `json:"language"`
		Altcha   string         `json:"altcha"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json"))
		return
	}

	if err := h.Tienda.VerificarAltcha(r.Context(), req.Altcha); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("altcha_invalid"))
		return
	}

	if err := h.Tienda.EnviarFormularioPublico(r.Context(), tenant, siteID, formID, req.Fields, req.Consent, req.Language); err != nil {
		if errors.Is(err, domain.ErrConsentimientoRequerido) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("consent_required"))
			return
		}
		if errors.Is(err, domain.ErrFormularioNoExiste) {
			httpx.Fail(w, r, httpx.ErrNotFound.WithCode("form_not_found"))
			return
		}
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("submission_invalid").WithMessage(err.Error()))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) publicoCheckout(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}
	tenant, _ := r.Context().Value(ctxKeyTenant).(string)
	sitio, _ := r.Context().Value(ctxKeySitio).(*domain.SitioCompleto)

	var req domain.CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json"))
		return
	}

	if err := h.Tienda.VerificarAltcha(r.Context(), req.Altcha); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("altcha_invalid"))
		return
	}

	canonicalHost := r.Host
	if sitio != nil && sitio.CanonicalHost != nil && *sitio.CanonicalHost != "" {
		canonicalHost = *sitio.CanonicalHost
	}

	// Validar ReturnURL contra redirección abierta (§9.5)
	req.ReturnURL = sanitizarReturnURL(req.ReturnURL, req.Language, sitio, canonicalHost)

	resp, err := h.Tienda.Checkout(r.Context(), tenant, sitio, &req, canonicalHost)
	if err != nil {
		traducirErrorCheckout(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusOK, resp)
}

func sanitizarReturnURL(rawURL, lang string, sitio *domain.SitioCompleto, canonicalHost string) string {
	defaultURL := fmt.Sprintf("https://%s/%s/order/", canonicalHost, lang)
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return defaultURL
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return defaultURL
	}

	if u.Scheme == "https" || u.Scheme == "http" {
		origin := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		if validarOrigenPermitido(origin, canonicalHost, sitio) {
			return rawURL
		}
		return defaultURL
	}

	// Comprobar esquema nativo de app (ej. myapp://...)
	scheme := strings.ToLower(u.Scheme)
	if !esquemasProhibidos[scheme] && reCustomScheme.MatchString(scheme) && sitio != nil {
		for _, s := range sitio.ExportAppSchemes {
			if strings.EqualFold(scheme, strings.TrimSpace(s)) {
				return rawURL
			}
		}
	}

	return defaultURL
}

func traducirErrorCheckout(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, domain.ErrTiendaOcupada) {
		httpx.Fail(w, r, httpx.ErrTooManyRequests.WithCode("store_busy"))
		return
	}
	if errors.Is(err, domain.ErrDemasiadosPendientes) {
		httpx.Fail(w, r, httpx.ErrTooManyRequests.WithCode("too_many_pending_orders"))
		return
	}
	if errors.Is(err, domain.ErrExistenciasAgotadas) {
		httpx.Fail(w, r, httpx.ErrConflict.WithCode("out_of_stock").WithMessage(err.Error()))
		return
	}
	if errors.Is(err, domain.ErrFacturaRequerida) || errors.Is(err, domain.ErrNIFInvalido) {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("tax_id_invalid").WithMessage(err.Error()))
		return
	}
	if errors.Is(err, domain.ErrPagoNoDisponible) {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("gateway_unavailable"))
		return
	}
	httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("checkout_failed").WithMessage(err.Error()))
}

func (h *Handlers) publicoPagar(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}
	tenant, _ := r.Context().Value(ctxKeyTenant).(string)
	sitio, _ := r.Context().Value(ctxKeySitio).(*domain.SitioCompleto)
	paymentIDStr := chi.URLParam(r, "payment_id")
	paymentID, err := uuid.Parse(paymentIDStr)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("payment_not_found"))
		return
	}

	token := strings.TrimSpace(r.FormValue("token"))
	if token == "" {
		var body struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		token = strings.TrimSpace(body.Token)
	}

	canonicalHost := r.Host
	if sitio != nil && sitio.CanonicalHost != nil && *sitio.CanonicalHost != "" {
		canonicalHost = *sitio.CanonicalHost
	}

	pay, err := h.Tienda.Pagar(r.Context(), tenant, sitio, paymentID, token, canonicalHost)
	if err != nil {
		if errors.Is(err, domain.ErrTokenPedidoInvalido) {
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithCode("invalid_order_token"))
			return
		}
		if errors.Is(err, domain.ErrDemasiadosIntentos) {
			httpx.Fail(w, r, httpx.ErrConflict.WithCode("too_many_payment_attempts"))
			return
		}
		if errors.Is(err, domain.ErrPedidoYaPagado) {
			httpx.Fail(w, r, httpx.ErrConflict.WithCode("order_not_payable"))
			return
		}
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("payment_not_found"))
		return
	}

	if pay.Formulario != nil {
		// Redsys: Formulario autoenviado
		accion, _ := pay.Formulario["accion"].(string)
		campos, _ := pay.Formulario["campos"].(map[string]string)
		paginaRedsys(w, r, accion, campos)
		return
	}

	if pay.URL != "" {
		http.Redirect(w, r, pay.URL, http.StatusSeeOther)
		return
	}

	httpx.JSON(w, r, http.StatusOK, pay)
}

func (h *Handlers) publicoConsultarPedido(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}
	tenant, _ := r.Context().Value(ctxKeyTenant).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(uuid.UUID)
	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("order_not_found"))
		return
	}

	orderToken := strings.TrimSpace(r.Header.Get("X-Sytue-Pedido"))
	if orderToken == "" {
		orderToken = strings.TrimSpace(r.URL.Query().Get("token"))
	}

	order, err := h.Tienda.ConsultarPedido(r.Context(), tenant, siteID, orderID, orderToken)
	if err != nil {
		if errors.Is(err, domain.ErrTokenPedidoInvalido) {
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithCode("invalid_order_token"))
			return
		}
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("order_not_found"))
		return
	}

	httpx.JSON(w, r, http.StatusOK, order)
}

func (h *Handlers) publicoSolicitarFactura(w http.ResponseWriter, r *http.Request) {
	tenant, _ := r.Context().Value(ctxKeyTenant).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(uuid.UUID)
	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("order_not_found"))
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	token := req.Token
	if token == "" {
		token = r.Header.Get("X-Sytue-Pedido")
	}

	order, err := h.Tienda.ConsultarPedido(r.Context(), tenant, siteID, orderID, token)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithCode("invalid_order_token"))
		return
	}

	if !order.HasInvoice {
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("invoice_not_ready"))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = paginaFactura.Execute(w, order)
}

// Las dos páginas HTML de /_pages, con html/template y no con fmt: cada valor se escapa
// en su contexto (WEB-12). Hoy los pone el servidor o son base64, pero no tiene que
// depender de eso.
var (
	paginaFactura = template.Must(template.New("factura").Parse(
		`<!DOCTYPE html><html><body><h1>Factura {{.ERPInvoiceRef}}</h1><p>Pedido: {{.Number}}</p></body></html>`))

	formularioRedsys = template.Must(template.New("redsys").Parse(`<!DOCTYPE html><html><head><meta charset="utf-8"></head>` +
		`<body><form method="post" action="{{.Accion}}">{{range $k, $v := .Campos}}<input type="hidden" name="{{$k}}" value="{{$v}}">{{end}}` +
		`<noscript><button type="submit">Continuar</button></noscript></form><script>` + envioRedsys + `</script></body></html>`))
)

// envioRedsys es el ÚNICO script de la página de Redsys, y su CSP lo deja correr por hash.
const envioRedsys = "document.forms[0].submit()"

var hashEnvioRedsys = func() string {
	suma := sha256.Sum256([]byte(envioRedsys))
	return base64.StdEncoding.EncodeToString(suma[:])
}()

// paginaRedsys reenvía al comprador a Redsys con el formulario firmado. La CSP de la
// página solo deja correr el script que lo envía y solo deja enviarlo a la pasarela.
func paginaRedsys(w http.ResponseWriter, r *http.Request, accion string, campos map[string]string) {
	destino, err := url.Parse(accion)
	if err != nil || destino.Scheme != "https" || destino.Host == "" {
		httpx.Fail(w, r, httpx.ErrInternal.WithCode("gateway_form_invalid"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'sha256-"+hashEnvioRedsys+
		"'; form-action https://"+destino.Host+"; base-uri 'none'; frame-ancestors 'none'")
	w.WriteHeader(http.StatusOK)
	_ = formularioRedsys.Execute(w, struct {
		Accion string
		Campos map[string]string
	}{accion, campos})
}

func (h *Handlers) publicoDesistimiento(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}
	tenant, _ := r.Context().Value(ctxKeyTenant).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(uuid.UUID)

	var req struct {
		OrderNumber string `json:"order_number"`
		Email       string `json:"email"`
		Name        string `json:"name"`
		Reason      string `json:"reason"`
		Altcha      string `json:"altcha"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json"))
		return
	}

	if err := h.Tienda.VerificarAltcha(r.Context(), req.Altcha); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("altcha_invalid"))
		return
	}

	_ = h.Tienda.RegistrarDesistimientoPublico(r.Context(), tenant, siteID, req.OrderNumber, req.Email, req.Name, req.Reason, nil)

	// Siempre 204 sin oráculo de existencia (§10.7)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) publicoDesistimientoPedido(w http.ResponseWriter, r *http.Request) {
	if h.Tienda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("tienda_not_configured"))
		return
	}
	tenant, _ := r.Context().Value(ctxKeyTenant).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(uuid.UUID)
	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("order_not_found"))
		return
	}

	orderToken := strings.TrimSpace(r.Header.Get("X-Sytue-Pedido"))
	if _, err := h.Tienda.ConsultarPedido(r.Context(), tenant, siteID, orderID, orderToken); err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithCode("invalid_order_token"))
		return
	}

	var req struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
		Altcha string `json:"altcha"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json"))
		return
	}

	if err := h.Tienda.VerificarAltcha(r.Context(), req.Altcha); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("altcha_invalid"))
		return
	}

	_ = h.Tienda.RegistrarDesistimientoPublico(r.Context(), tenant, siteID, "", "", req.Name, req.Reason, &orderID)
	w.WriteHeader(http.StatusNoContent)
}
