// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

func pedidoTiendaValido() pedidoTienda {
	p := pedidoTienda{
		Tenant: "acme", Origen: "pages", Modo: "live", PedidoID: "order-1", Numero: "P-000001",
		Fecha: "2026-10-02", Divisa: "EUR", Factura: "simplificada", TotalCentimos: 2910,
	}
	p.Comprador.Nombre = "Ana"
	p.Comprador.Correo = "ana@example.test"
	p.Comprador.Direccion.Pais = "ES"
	p.Comprador.Direccion.CP = "47001"
	p.Lineas = []lineaPedidoTienda{{SKU: "pan", Nombre: "Pan", Cantidad: 2, PrecioUnitarioCentimos: 1210, TipoIVA: 21}}
	p.Envio = &lineaPedidoTienda{Nombre: "Envío", PrecioCentimos: 490, TipoIVA: 21}
	p.Cobro.Pasarela = "mollie"
	p.Cobro.Referencia = "tr_1"
	p.Cobro.Fecha = "2026-10-02T10:00:00Z"
	p.Cobro.ImporteCentimos = p.TotalCentimos
	return p
}

func TestValidarPedidoTienda(t *testing.T) {
	casos := []struct {
		nombre  string
		cambiar func(*pedidoTienda)
	}{
		{"test mode", func(p *pedidoTienda) { p.Modo = "test" }},
		{"sum mismatch", func(p *pedidoTienda) { p.TotalCentimos++ }},
		{"invalid VAT", func(p *pedidoTienda) { p.Lineas[0].TipoIVA = 7 }},
		{"F2 outside Spain", func(p *pedidoTienda) { p.Comprador.Direccion.Pais = "FR" }},
		{"F2 Canary Islands", func(p *pedidoTienda) { p.Comprador.Direccion.CP = "35001" }},
		{"F2 over limit", func(p *pedidoTienda) { p.TotalCentimos = 300001; p.Cobro.ImporteCentimos = 300001 }},
		{"F1 invalid NIF", func(p *pedidoTienda) { p.Factura = "completa"; p.Comprador.NIF = "not-a-nif" }},
	}
	for _, tc := range casos {
		t.Run(tc.nombre, func(t *testing.T) {
			p := pedidoTiendaValido()
			tc.cambiar(&p)
			if err := validarPedidoTienda(p); err == nil {
				t.Fatal("validarPedidoTienda accepted an invalid order")
			}
		})
	}
	if err := validarPedidoTienda(pedidoTiendaValido()); err != nil {
		t.Fatalf("validarPedidoTienda rejected a valid order: %v", err)
	}
}

func TestPedidosTiendaHTTP_SeguridadYValidacion(t *testing.T) {
	const secreto = "s-pedidos-tienda-test"
	h := &Handlers{}

	r := chi.NewRouter()
	r.With(httpx.SoloDesdeDentro(secreto)).
		Post("/internal/pedidos-tienda", h.handlePedidosTienda)
	r.With(httpx.SoloDesdeDentro(secreto)).
		Get("/internal/pedidos-tienda/{pedido_id}/factura", h.handlePedidosTiendaFactura)

	t.Run("sin_autorizacion_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/internal/pedidos-tienda", bytes.NewReader([]byte("{}")))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("código = %d; want 401", w.Code)
		}
	})

	t.Run("con_secreto_incorrecto_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/internal/pedidos-tienda", bytes.NewReader([]byte("{}")))
		req.Header.Set("Authorization", "Bearer otro-secreto")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("código = %d; want 401", w.Code)
		}
	})

	t.Run("cuerpo_invalido_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/internal/pedidos-tienda", bytes.NewReader([]byte("{malformed")))
		req.Header.Set("Authorization", "Bearer "+secreto)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("código = %d; want 400", w.Code)
		}
	})

	t.Run("contrato_invalido_422", func(t *testing.T) {
		p := pedidoTiendaValido()
		p.Modo = "test" // inválido: debe ser live
		cuerpo, _ := json.Marshal(p)
		req := httptest.NewRequest(http.MethodPost, "/internal/pedidos-tienda", bytes.NewReader(cuerpo))
		req.Header.Set("Authorization", "Bearer "+secreto)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("código = %d; want 422", w.Code)
		}
	})

	t.Run("factura_sin_tenant_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/internal/pedidos-tienda/order-1/factura", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+secreto)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("código = %d; want 400", w.Code)
		}
	})
}
