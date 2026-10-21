// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// WEB-03. Un pago cobrado no vuelve a abierto ni a fallido, no entra en el barrido, y
// una notificación repetida de ese mismo pago no es un «duplicado» (que se reembolsa
// solo): duplicado es otro pago del mismo pedido ya cobrado.
func TestUnPagoCobradoNoSeReabreNiSeReembolsaSolo(t *testing.T) {
	dosRoles(t, func(t *testing.T, s *Store, super *pg.Pool) {
		ctx := context.Background()
		const org = "tienda"
		x := sembrarTienda(t, super, org, "tienda-pagada")

		if _, _, dup, err := s.MarcarPedidoPagado(ctx, org, x.pedido, x.pago, "tr_1", 250); err != nil || dup {
			t.Fatalf("primer cobro: dup=%v err=%v", dup, err)
		}

		// El comprador repite «pagar» y una notificación tardía dice «fallido».
		for _, estado := range []string{domain.PaymentStatusOpen, domain.PaymentStatusFailed} {
			if err := s.ActualizarEstadoPago(ctx, org, x.pago, "", estado); err != nil {
				t.Fatal(err)
			}
			if p, err := s.ObtenerPago(ctx, org, x.pago); err != nil || p.Status != domain.PaymentStatusPaid {
				t.Fatalf("tras pedir %q el pago cobrado quedó en %v (%v)", estado, p, err)
			}
		}
		pendientes, err := s.ObtenerPagosPendientesBarrido(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range pendientes {
			if p.PaymentID == x.pago {
				t.Fatal("el pago cobrado vuelve a estar en el barrido")
			}
		}

		// La misma notificación «pagado» otra vez: ni duplicado ni nada que repetir.
		if _, _, dup, err := s.MarcarPedidoPagado(ctx, org, x.pedido, x.pago, "tr_1", 250); dup || !errors.Is(err, domain.ErrPagoYaTratado) {
			t.Fatalf("notificación repetida: dup=%v err=%v; quiero ErrPagoYaTratado y sin duplicado", dup, err)
		}

		// Otro intento del mismo pedido que también se cobra: ese SÍ es duplicado.
		otro := uuid.New()
		if _, err := super.Exec(ctx, `INSERT INTO payments (id, tenant, order_id, gateway_id, attempt, provider, provider_ref, status, amount_cents, idempotency_key)
			VALUES ($1, $2, $3, $4, 2, 'mollie', 'tr_2', 'open', 250, 'k2')`, otro, org, x.pedido, x.pasarela); err != nil {
			t.Fatal(err)
		}
		if _, _, dup, err := s.MarcarPedidoPagado(ctx, org, x.pedido, otro, "tr_2", 250); err != nil || !dup {
			t.Fatalf("segundo cobro del mismo pedido: dup=%v err=%v; quiero duplicado", dup, err)
		}
	})
}
