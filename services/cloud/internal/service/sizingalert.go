// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// ---------------------------------------------------------------------------
// Evaluador proactivo de tamaño
// ---------------------------------------------------------------------------
//
// La recomendación de tamaño existía desde la 0026, pero solo se veía ENTRANDO al
// proyecto: el ahorro dependía de que alguien abriera el panel, y el aviso de «se
// te está quedando corto» llegaba —si llegaba— después del OOMKill. Esto es la
// otra mitad de la promesa: avisamos nosotros.
//
// CADENCIA: una hora. El motor decide sobre una ventana de días (72 h de mínimo
// para bajar), así que la recomendación no puede cambiar entre dos ciclos cortos;
// sondear más a menudo solo repetiría el mismo barrido. Es el mismo criterio del
// evaluador de coste, con la constante que le toca a esta señal.
const (
	sizingAlertTick = time.Hour
	// sizingAlertCooldown es el silencio mínimo entre dos avisos del MISMO
	// proyecto. Sin él, una recomendación que oscila avisaría cada hora, que es la
	// forma más rápida de que el cliente silencie el canal entero.
	sizingAlertCooldown = 24 * time.Hour
	sizingAlertBatch    = 1000
)

// RunSizingAlerter arranca el evaluador proactivo de tamaño (sin estado,
// cross-réplica). Bloquea hasta que se cancele ctx. Lo arranca cmd/cloud-api.
//
// Corre en TODAS las réplicas a propósito, igual que RunSpendAlerter: la
// exclusión la da la clave primaria de sizing_alerts al reservar el aviso
// (store.PublishSizingAlert), así que N réplicas evaluando a la vez producen un
// solo aviso, y si una se cae aquí no se deja de avisar.
func (s *Service) RunSizingAlerter(ctx context.Context) error {
	l := log.From(ctx).With("component", "sizing-alerter")
	l.Info("evaluador proactivo de tamaño arrancado", "tick", sizingAlertTick.String())
	for {
		if err := s.evaluateSizingAlerts(ctx); err != nil && ctx.Err() == nil {
			l.Warn("ciclo de avisos de tamaño con error", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(sizingAlertTick):
		}
	}
}

// evaluateSizingAlerts recorre los proyectos con medición suficiente y avisa de
// los que dejaron de estar bien dimensionados. Un fallo con un proyecto NO aborta
// el barrido: un webhook roto de una organización no puede dejar sin aviso a las
// demás.
func (s *Service) evaluateSizingAlerts(ctx context.Context) error {
	cands, err := s.pstore.ListSizingCandidates(ctx, time.Now().Add(-sizingWindow), sizingAlertBatch)
	if err != nil {
		return err
	}
	if len(cands) == sizingAlertBatch {
		// Se dice en voz alta: el barrido cubrió un subconjunto ALEATORIO y hay
		// proyectos que este ciclo no ha mirado. Callarlo haría leer «evaluado
		// todo» donde no lo está (ver el ponytail de ListSizingCandidates).
		log.From(ctx).Info("barrido de tamaño al tope del lote: se evalúa una muestra este ciclo",
			"lote", sizingAlertBatch)
	}
	for _, c := range cands {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // cancelación del contexto = apagado ordenado, no un fallo.
		}
		if err := s.evaluateProjectSizing(ctx, c.Tenant, c.ProjectID); err != nil {
			log.From(ctx).Warn("evaluar tamaño del proyecto",
				"tenant", c.Tenant, "project_id", c.ProjectID, "error", err)
		}
	}
	return nil
}

// evaluateProjectSizing calcula la recomendación de UN proyecto y emite el aviso
// si es accionable y aún no se avisó.
//
// Reusa ProjectSizingAdvice —el MISMO camino que responde a la API y pinta la
// tarjeta— en vez de replicar el criterio aquí: dos motores acabarían avisando de
// una cosa y proponiendo otra en la pantalla, que es peor que no avisar.
func (s *Service) evaluateProjectSizing(ctx context.Context, tenantSlug string, projectID uuid.UUID) error {
	// El motor lee proyecto y muestras bajo RLS: el barrido es cross-tenant, pero
	// cada evaluación va acotada al inquilino del proyecto.
	t, err := tenancy.New(tenantSlug)
	if err != nil {
		return err
	}
	adv, err := s.ProjectSizingAdvice(tenancy.Into(ctx, t), projectID)
	if err != nil {
		return err
	}
	// El consumo al límite se mira SIEMPRE, incluso cuando el tamaño está bien:
	// son dos preguntas distintas. «Deberías cambiarte de plan» puede esperar a
	// mañana; «llevas horas al 95 % de memoria» es lo que avisa antes del reinicio
	// por falta de memoria, y pasa igual en un proyecto perfectamente dimensionado
	// al que le entró tráfico. Va aquí porque el advice ya trae las dos cifras
	// medidas: evaluarlo cuesta cero consultas.
	s.evaluateMetricThresholds(ctx, tenantSlug, projectID, adv)

	// `keep` incluye «bien dimensionado» y «el pico impide bajar»: los dos son
	// información de panel, no algo de lo que despertar a nadie.
	//
	// Un consejo apoyado SOLO en la cohorte tampoco se emite: aquí no llega
	// ninguno —el prefiltro exige muestras propias— y es deliberado. Avisar a
	// alguien de que baje el tamaño porque «las apps parecidas van sobradas»,
	// sin haber medido la suya, es exactamente la recomendación que acaba en
	// OOMKill y en no volver a fiarse de la siguiente.
	if adv.Action == domain.SizingKeep {
		return nil
	}
	sent, err := s.pstore.PublishSizingAlert(ctx, tenantSlug, projectID, sizingAlertCooldown,
		domain.SizingAlertPayload{
			ProjectID:        projectID.String(),
			Action:           adv.Action,
			Reason:           adv.Reason,
			FromSize:         adv.From,
			ToSize:           adv.To,
			MonthlyDeltaEur:  adv.MonthlyDeltaEur,
			P95MemoryPercent: adv.P95MemoryPercent,
		})
	if err != nil {
		return err
	}
	if sent {
		log.From(ctx).Info("aviso de tamaño emitido",
			"tenant", tenantSlug, "project_id", projectID,
			"accion", adv.Action, "de", adv.From, "a", adv.To)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Consumo al límite (CPU y memoria)
// ---------------------------------------------------------------------------

// metricAlertCooldown es el silencio mínimo entre dos avisos del mismo recurso y
// proyecto. Más corto que el de tamaño (24 h) porque esto es urgente y menos que
// el tick para no repetirlo cada hora: si sigue al límite pasadas seis horas, es
// que nadie lo vio y merece insistir.
const metricAlertCooldown = 6 * time.Hour

// evaluateMetricThresholds avisa de los recursos que llevan la ventana entera
// pegados al techo, y OLVIDA los que han vuelto a la normalidad.
//
// Se apoya en el p95 y no en el pico: un pico de un minuto no es un problema —lo
// absorbe cualquier app— mientras que un p95 alto significa que el 95 % del
// tiempo se está rozando el límite, que es cuando llegan los reinicios por
// memoria y el estrangulamiento de CPU.
//
// Los fallos se registran y no se propagan: este aviso viaja de acompañante del
// de tamaño, y un canal roto no puede hacer que el otro tampoco salga.
func (s *Service) evaluateMetricThresholds(ctx context.Context, tenantSlug string, projectID uuid.UUID, adv domain.SizingAdvice) {
	// Quién supera el umbral lo decide el DOMINIO (domain.MetricUsageOf): aquí solo
	// se publica o se olvida. Así la división que decide si alguien recibe el aviso
	// a tiempo se puede probar sin base de datos.
	for _, u := range domain.MetricUsageOf(adv) {
		if !u.Over {
			// Vuelto a la normalidad: se olvida, para que una recaída vuelva a
			// avisar en vez de quedarse callada porque «ya se avisó».
			if err := s.pstore.ClearMetricAlert(ctx, projectID, u.Resource); err != nil {
				log.From(ctx).Warn("limpiar aviso de consumo", "project_id", projectID, "recurso", u.Resource, "error", err)
			}
			continue
		}
		busType := store.EventMetricCPUHigh
		if u.Resource == domain.MetricResourceMemory {
			busType = store.EventMetricMemoryHigh
		}
		sent, err := s.pstore.PublishMetricAlert(ctx, tenantSlug, projectID, u.Resource, u.Percent,
			metricAlertCooldown, busType, store.MetricAlertPayload{
				ProjectID:   projectID.String(),
				Resource:    u.Resource,
				Percent:     u.Percent,
				Threshold:   domain.MetricAlertPercent,
				InstanceSze: adv.From,
				WindowHours: adv.WindowHours,
			})
		if err != nil {
			log.From(ctx).Warn("emitir aviso de consumo", "project_id", projectID, "recurso", u.Resource, "error", err)
			continue
		}
		if sent {
			log.From(ctx).Info("aviso de consumo emitido",
				"tenant", tenantSlug, "project_id", projectID, "recurso", u.Resource, "porcentaje", u.Percent)
		}
	}
}
