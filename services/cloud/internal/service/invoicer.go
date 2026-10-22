// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// ---------------------------------------------------------------------------
// Emisor mensual de facturas
// ---------------------------------------------------------------------------
//
// La pestaña de facturación promete «emitimos una factura al cierre de cada
// periodo» y no había NADA que la emitiera: el motor (GenerateInvoice) estaba
// entregado y su único disparador era un endpoint que no llamaba nadie. Esto es
// el disparador.
//
// Va DENTRO de cloud-api y no en un CronJob del chart a propósito: el endpoint
// POST /billing/invoices:generate exige rol admin/owner y factura UN tenant —el
// del token—, así que un CronJob externo necesitaría una credencial por
// organización. La facturación es cross-tenant por naturaleza; el sitio donde
// eso es legítimo es el propio servicio, con la transacción de mantenimiento que
// ya usan el purgador y el evaluador de gasto.
//
// Corre en TODAS las réplicas sin elección de líder, como RunSpendAlerter: la
// exclusión la da el UNIQUE(tenant, period_from, period_to) de invoices, así que
// N réplicas emitiendo a la vez producen una sola factura (CreateInvoice
// devuelve la existente con created=false).
const (
	invoicerTick  = time.Hour
	invoicerBatch = 1000
	// invoiceGrace es lo que se espera desde que cierra el mes antes de facturarlo.
	// El medidor del controlador escribe el uso de forma continua, pero un reinicio
	// o un reintento puede dejar los últimos minutos del mes en camino, y la
	// factura NO se corrige sola: es idempotente, así que emitirla de más pronto
	// congela para siempre un uso incompleto. Es la única perilla de calibración
	// aquí; subirla retrasa la factura, bajarla arriesga facturar de menos.
	invoiceGrace = 24 * time.Hour
)

// RunInvoicer arranca el emisor mensual de facturas. Bloquea hasta que se
// cancele ctx. Lo arranca cmd/cloud-api.
func (s *Service) RunInvoicer(ctx context.Context) error {
	l := log.From(ctx).With("component", "invoicer")
	l.Info("emisor de facturas arrancado",
		"tick", invoicerTick.String(), "gracia", invoiceGrace.String())
	for {
		if err := s.issueClosedPeriod(ctx, time.Now()); err != nil && ctx.Err() == nil {
			l.Warn("ciclo de facturación con error", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(invoicerTick):
		}
	}
}

// issueClosedPeriod emite la factura del último mes natural cerrado a quien le
// falte. Un fallo con un tenant NO aborta el barrido: se registra y se sigue,
// porque el uso ilegible de una organización no puede dejar sin factura a las
// demás (y el siguiente ciclo lo reintenta: el filtro es «sin factura»).
//
// ponytail: solo mira el ÚLTIMO periodo cerrado. Una caída de más de un mes deja
// ese mes sin emitir; se recupera a mano con POST /billing/invoices:generate,
// que es también como se facturaría un periodo anterior al despliegue de esto.
func (s *Service) issueClosedPeriod(ctx context.Context, now time.Time) error {
	from, to := lastClosedMonth(now)
	if now.UTC().Before(to.Add(invoiceGrace)) {
		return nil
	}
	tenants, err := s.pstore.ListTenantsToInvoice(ctx, from, to, invoicerBatch)
	if err != nil {
		return err
	}
	for _, slug := range tenants {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // cancelación del contexto = apagado ordenado, no un fallo.
		}
		t, err := tenancy.New(slug)
		if err != nil {
			log.From(ctx).Warn("tenant no válido al facturar", "tenant", slug, "error", err)
			continue
		}
		inv, err := s.GenerateInvoice(tenancy.Into(ctx, t), from, to)
		if err != nil {
			log.From(ctx).Warn("emitir factura del tenant", "tenant", slug, "error", err)
			continue
		}
		log.From(ctx).Info("factura emitida",
			"tenant", slug, "factura", inv.ID, "total_eur", inv.TotalEur,
			"periodo_desde", from, "periodo_hasta", to)
	}
	return nil
}

// lastClosedMonth devuelve [from, to) del último mes natural CERRADO en UTC. El
// periodo es semiabierto por la derecha, igual que lo espera SummarizeUsage.
func lastClosedMonth(now time.Time) (from, to time.Time) {
	u := now.UTC()
	to = time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	return to.AddDate(0, -1, 0), to
}
