// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tienda

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pagos"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// IniciarBarrido ejecuta el bucle de barrido de pedidos y pagos pendientes cada 5 minutos (§10.5).
func (s *Servicio) IniciarBarrido(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.EjecutarRondaBarrido(ctx)
		}
	}
}

// EjecutarRondaBarrido ejecuta una pasada de sincronización de pagos y liberación de
// pedidos vencidos, y purga el outbox (WEB-10).
func (s *Servicio) EjecutarRondaBarrido(ctx context.Context) error {
	s.liberarPedidosVencidos(ctx)
	s.sincronizarPagosPendientes(ctx)
	_, _ = s.store.PurgarOutbox(ctx, time.Now())
	return nil
}

func (s *Servicio) liberarPedidosVencidos(ctx context.Context) {
	vencidos, err := s.store.ObtenerPedidosVencidos(ctx)
	if err != nil {
		return
	}
	for _, v := range vencidos {
		if p, err := s.store.ObtenerUltimoPagoPorOrden(ctx, v.Tenant, v.OrderID); err == nil && p != nil {
			if gw, err := s.store.ObtenerPasarelaConCredenciales(ctx, v.Tenant, v.SiteID, p.Provider); err == nil {
				if pasarela, err := s.CrearPasarelaCliente(gw); err == nil {
					_ = pasarela.Caducar(ctx, p.ProviderRef)
				}
			}
		}
		_ = s.store.LiberarReservaPedidoVencido(ctx, v.Tenant, v.OrderID)
	}
}

func (s *Servicio) sincronizarPagosPendientes(ctx context.Context) {
	pendientes, err := s.store.ObtenerPagosPendientesBarrido(ctx)
	if err != nil {
		return
	}
	for _, p := range pendientes {
		gw, err := s.store.ObtenerPasarelaConCredenciales(ctx, p.Tenant, p.SiteID, p.Provider)
		if err != nil {
			continue
		}
		pasarela, err := s.CrearPasarelaCliente(gw)
		if err != nil {
			continue
		}

		estado, err := pasarela.ConsultarPago(ctx, p.ProviderRef)
		if err != nil {
			continue
		}

		pagoModel := &domain.Payment{
			ID:          p.PaymentID,
			OrderID:     p.OrderID,
			GatewayID:   p.GatewayID,
			Provider:    p.Provider,
			ProviderRef: p.ProviderRef,
			AmountCents: p.AmountCents,
		}
		if err := s.verificarSiActua(pagoModel, gw, estado); err != nil {
			continue
		}

		switch estado.Estado {
		case pagos.EstadoPagado:
			_ = s.tratarPagoCompletado(ctx, p.Tenant, pagoModel, gw, estado.Importe)
		case pagos.EstadoFallido:
			_ = s.store.ActualizarEstadoPago(ctx, p.Tenant, p.PaymentID, p.ProviderRef, domain.PaymentStatusFailed)
		case pagos.EstadoCancelado:
			_ = s.store.ActualizarEstadoPago(ctx, p.Tenant, p.PaymentID, p.ProviderRef, domain.PaymentStatusCanceled)
		case pagos.EstadoCaducado:
			_ = s.store.ActualizarEstadoPago(ctx, p.Tenant, p.PaymentID, p.ProviderRef, domain.PaymentStatusExpired)
		}
	}
}
