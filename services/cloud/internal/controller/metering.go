// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"math"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// Parámetros del metering de cómputo (facturación). Constantes: cadencia y cotas
// operativas estables, no configurables por despliegue.
const (
	// computeMeterInterval es la ventana mínima entre muestras de una misma
	// producción: acota el crecimiento de usage_records (≈1 fila/prod por intervalo;
	// el rollup mensual las agrega) sin perder precisión de facturación.
	computeMeterInterval = 5 * time.Minute
	// computeMeterMaxWindow acota la ventana de una sola muestra (clamp): evita
	// facturar retroactivamente una producción medida por primera vez (metered_at
	// NULL con ready_at antiguo) o el hueco tras una caída larga del controller.
	computeMeterMaxWindow = 2 * time.Hour
	// computeMeterBatch acota las producciones medidas por ciclo (backlog FIFO por
	// antigüedad de medición; se drena en varios ciclos, como el teardown).
	// ponytail: global batch; si producciones ≫ batch×(interval/reconcile), sube el
	// batch o dedica un bucle de metering aparte.
	computeMeterBatch = 1000
)

// reconcileCompute emite compute_gb_hours de las producciones ready cuyo intervalo
// de medición ha vencido: reclama su ventana (avanzando metered_at atómicamente) y
// registra réplicas × memoria(GiB) × horas con RecordUsage. Idempotente y sin N+1
// (una sola pasada reclama todo el lote; el cálculo es local).
//
// storage_gb ya lo emite Ola F (service/storage.go, best-effort al listar objetos):
// no se duplica aquí. egress_gb/requests quedan DIFERIDOS: necesitan un pipeline de
// métricas de red/tráfico (ingress) que aún no existe; sin serie no hay qué medir.
func (c *Controller) reconcileCompute(ctx context.Context) error {
	now := time.Now().UTC()
	windows, err := c.store.ClaimComputeUsage(ctx, now, computeMeterInterval, computeMeterMaxWindow, computeMeterBatch)
	if err != nil {
		return err
	}
	for _, wnd := range windows {
		// La memoria facturable sale del TAMAÑO DE INSTANCIA del proyecto. Antes era
		// una constante de 0,5 GiB para todo el mundo: quien corría una instancia
		// grande pagaba lo mismo que quien corría la mínima. Un tamaño desconocido
		// (fila antigua, migración a medias) degrada al básico, que es justo lo que
		// se facturaba antes, así que nadie ve un salto de factura inexplicable.
		qty := computeGBHours(wnd.Replicas, domain.InstanceSizeOrDefault(wnd.Size).MemoryGiB,
			now.Sub(wnd.Since))
		if qty <= 0 {
			continue
		}
		pid := wnd.ProjectID
		if rerr := c.store.RecordUsage(ctx, store.UsageInput{
			Tenant: wnd.Tenant, ProjectID: &pid, Metric: "compute_gb_hours",
			Quantity: qty, Unit: "gb_hours", PeriodFrom: wnd.Since, PeriodTo: now,
		}); rerr != nil {
			// Best-effort (como el resto del metering): la ventana ya se avanzó, se
			// pierde esta muestra pero NO se re-cobra. Se registra para diagnóstico.
			log.From(ctx).Warn("registrar uso de cómputo", "error", rerr, "tenant", wnd.Tenant)
		}
	}
	return nil
}

// computeGBHours = réplicas × memoria(GiB) × horas de la ventana, redondeado a 6
// decimales (elimina ruido de coma flotante manteniendo precisión de gb-hora).
// Devuelve 0 ante entradas no positivas (ventana vacía, sin réplicas).
func computeGBHours(replicas int, memGiB float64, window time.Duration) float64 {
	if replicas <= 0 || memGiB <= 0 || window <= 0 {
		return 0
	}
	v := float64(replicas) * memGiB * window.Hours()
	return math.Round(v*1e6) / 1e6
}
