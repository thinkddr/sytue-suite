// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// ---------------------------------------------------------------------------
// Evaluador de alertas de coste
// ---------------------------------------------------------------------------
//
// El formulario del límite de gasto prometía «Te avisaremos al acercarte a X €» y
// no había nada detrás. Esto lo cumple: cada ciclo compara el gasto del mes con el
// límite que el propio cliente configuró y, al cruzar un umbral, emite un evento
// que el notificador de alertas ya sabe entregar (webhook, con su anti-SSRF).
//
// CADENCIA: minutos, no segundos. El gasto se mueve despacio (se acumula por
// despliegues y horas de cómputo) y cada ciclo hace una consulta de uso POR
// TENANT con límite, así que sondear rápido solo añadiría carga a la BD sin
// adelantar el aviso de forma perceptible. El tope duro de gasto sigue siendo el
// choke point de Deploy, que es inmediato; esto es el aviso PREVIO.
const (
	spendAlertTick  = 5 * time.Minute
	spendAlertBatch = 1000
)

// RunSpendAlerter arranca el evaluador de alertas de coste (sin estado,
// cross-réplica). Bloquea hasta que se cancele ctx. Lo arranca cmd/cloud-api.
//
// Corre en TODAS las réplicas a propósito: no hay elección de líder ni lease. La
// exclusión la da la clave primaria de billing_spend_alerts al reservar el envío
// (store.PublishSpendAlert), así que N réplicas evaluando a la vez producen un
// solo aviso. Es más simple y más resistente que designar una réplica: si esa se
// cae, aquí no se deja de avisar.
func (s *Service) RunSpendAlerter(ctx context.Context) error {
	l := log.From(ctx).With("component", "spend-alerter")
	l.Info("evaluador de alertas de coste arrancado", "tick", spendAlertTick.String())
	for {
		if err := s.evaluateSpendAlerts(ctx); err != nil && ctx.Err() == nil {
			l.Warn("ciclo de alertas de coste con error", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(spendAlertTick):
		}
	}
}

// evaluateSpendAlerts recorre los tenants con límite y avisa de los umbrales
// recién cruzados. Un fallo con un tenant NO aborta el barrido: se registra y se
// sigue, porque un webhook mal configurado de una organización no puede dejar sin
// aviso a las demás.
func (s *Service) evaluateSpendAlerts(ctx context.Context) error {
	tenants, err := s.pstore.ListSpendLimitedTenants(ctx, spendAlertBatch)
	if err != nil {
		return err
	}
	period := domain.SpendAlertPeriod(time.Now())
	for _, t := range tenants {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // cancelación del contexto = apagado ordenado, no un fallo.
		}
		if err := s.evaluateTenantSpend(ctx, t.Tenant, t.Plan, t.LimitEur, period); err != nil {
			log.From(ctx).Warn("evaluar gasto del tenant", "tenant", t.Tenant, "error", err)
		}
	}
	return nil
}

// evaluateTenantSpend calcula el gasto del mes de UN tenant y emite los avisos de
// los umbrales cruzados que aún no se hubieran enviado en este periodo.
func (s *Service) evaluateTenantSpend(
	ctx context.Context, tenantSlug, plan string, limitEur float64, period time.Time,
) error {
	// El cálculo de gasto lee `usage` bajo RLS, así que necesita el tenant en el
	// contexto: el barrido es cross-tenant, pero cada consulta va acotada a uno.
	t, err := tenancy.New(tenantSlug)
	if err != nil {
		return err
	}
	spend, err := s.spendMonthToDate(tenancy.Into(ctx, t), domain.Plan(plan))
	if err != nil {
		return err
	}
	for _, pct := range domain.CrossedSpendThresholds(spend, limitEur) {
		sent, err := s.pstore.PublishSpendAlert(ctx, tenantSlug, period, pct, spend, limitEur)
		if err != nil {
			return err
		}
		if sent {
			log.From(ctx).Info("aviso de coste emitido",
				"tenant", tenantSlug, "umbral_pct", pct, "gasto_eur", spend, "limite_eur", limitEur)
		}
	}
	return nil
}
