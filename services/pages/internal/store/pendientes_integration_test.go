// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// WEB-06. Un solo comprador ya no llena los 30 huecos de pedidos sin pagar de una
// tienda: con tres suyos pendientes, el cuarto se rechaza y los demás siguen
// comprando. Repetir un checkout ya hecho devuelve el pedido, no pide otro hueco.
func TestUnCompradorNoLlenaLaTiendaConPedidosSinPagar(t *testing.T) {
	dosRoles(t, func(t *testing.T, s *Store, super *pg.Pool) {
		ctx := context.Background()
		const org = "tienda"
		x := sembrarTienda(t, super, org, "tienda-llena")
		// sembrarTienda ya gastó el P-000001 a mano.
		if _, err := super.Exec(ctx, `UPDATE store_settings SET next_order_number = 2 WHERE site_id = $1`, x.sitio); err != nil {
			t.Fatal(err)
		}

		pedir := func(correo string, token uuid.UUID) (*domain.Order, error) {
			t.Helper()
			ahora := time.Now().UTC()
			o := &domain.Order{
				ID: uuid.New(), Tenant: org, SiteID: x.sitio, Test: true, CheckoutToken: token,
				AccessTokenHash: []byte{0}, Status: domain.OrderStatusPendingPayment, Currency: "EUR",
				SubtotalCents: 250, TotalCents: 250, BuyerEmail: correo, BuyerName: "Comprador",
				Language: "es", TermsVersion: 1, TermsAcceptedAt: ahora,
				ReservedUntil: ahora.Add(35 * time.Minute), ERPStatus: "none",
			}
			guardado, _, err := s.CrearPedidoYReservar(ctx, org, x.sitio, o, nil, nil)
			return guardado, err
		}

		primero := uuid.New()
		for i, correo := range []string{"Atacante@Example.test", "atacante@example.test", " atacante@EXAMPLE.test "} {
			token := uuid.New()
			if i == 0 {
				token = primero
			}
			if _, err := pedir(correo, token); err != nil {
				t.Fatalf("pedido %d del mismo comprador: %v", i+1, err)
			}
		}
		if _, err := pedir("atacante@example.test", uuid.New()); !errors.Is(err, domain.ErrDemasiadosPendientes) {
			t.Fatalf("cuarto pedido sin pagar del mismo correo: err=%v; quiero ErrDemasiadosPendientes", err)
		}
		// El doble clic de uno ya hecho no es otro pedido: devuelve el suyo.
		if o, err := pedir("atacante@example.test", primero); err != nil || o == nil || o.CheckoutToken != primero {
			t.Fatalf("checkout repetido: %v, %v; quiero el pedido que ya existía", o, err)
		}
		if _, err := pedir("otra@example.test", uuid.New()); err != nil {
			t.Fatalf("otra persona no puede comprar: %v", err)
		}

		// El tope de la tienda sigue ahí (1 de sembrarTienda + 3 + 1 = 5 pendientes).
		for i := 5; i < 30; i++ {
			if _, err := pedir(fmt.Sprintf("p%d@example.test", i), uuid.New()); err != nil {
				t.Fatalf("pedido %d: %v", i+1, err)
			}
		}
		if _, err := pedir("tarde@example.test", uuid.New()); !errors.Is(err, domain.ErrTiendaOcupada) {
			t.Fatalf("pedido 31: err=%v; quiero ErrTiendaOcupada", err)
		}
	})
}
