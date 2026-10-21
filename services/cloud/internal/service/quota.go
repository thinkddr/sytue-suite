// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// ---------------------------------------------------------------------------
// Cuánto queda del plan
// ---------------------------------------------------------------------------
//
// Los límites existían y se aplicaban —crear el proyecto número once devuelve
// 402—, pero no se podían MIRAR: el tope aparecía de golpe al chocar contra él.
// DigitalOcean y Vercel enseñan el consumo frente a la cuota justamente para que
// eso no pase.

// QuotaUsage es el uso de un límite concreto.
type QuotaUsage struct {
	Used int `json:"used"`
	// Max 0 = sin límite. Es un valor legítimo, no «no se sabe»: la interfaz lo
	// enseña como ilimitado en vez de dibujar una barra al 100 %.
	Max int `json:"max"`
}

// QuotaReport es lo que la organización tiene frente a lo que su plan permite.
type QuotaReport struct {
	Plan     domain.Plan `json:"plan"`
	Projects QuotaUsage  `json:"projects"`
	// Services es el total de servicios gestionados de TODOS los proyectos: el
	// límite del plan es por organización, y contarlo por proyecto daría un número
	// que no se corresponde con el que decide el 402.
	Services QuotaUsage `json:"services"`
	// Pods, CPU y memoria son los topes del namespace del tenant (ResourceQuota).
	// Van como texto porque así los declara el plan y así los entiende Kubernetes;
	// convertirlos aquí a un número inventaría precisión que no hay.
	MaxPods       int    `json:"max_pods"`
	RequestCPU    string `json:"request_cpu"`
	RequestMemory string `json:"request_memory"`
	LimitCPU      string `json:"limit_cpu"`
	LimitMemory   string `json:"limit_memory"`
	// PostgresPVCSize y BackupRetentionDays son las dos cifras del plan que el
	// cliente nota en su base de datos.
	PostgresPVCSize     string `json:"postgres_pvc_size"`
	BackupRetentionDays int    `json:"backup_retention_days"`
}

// QuotaStatus devuelve el uso de la organización frente a los límites de su plan.
//
// Los conteos salen de las MISMAS listas que ve el panel (bajo RLS), no de un
// contador aparte: un contador desincronizado enseñaría «te quedan 3» y el alta
// devolvería 402, que es peor que no enseñar nada.
func (s *Service) QuotaStatus(ctx context.Context) (QuotaReport, error) {
	org, err := s.store.GetOrganization(ctx)
	if err != nil {
		return QuotaReport{}, err
	}
	q := domain.QuotasFor(org.Plan)

	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		return QuotaReport{}, err
	}
	servicios := 0
	for _, p := range projects {
		list, err := s.store.ListServices(ctx, p.ID)
		if err != nil {
			return QuotaReport{}, err
		}
		servicios += len(list)
	}

	return QuotaReport{
		Plan:                org.Plan,
		Projects:            QuotaUsage{Used: len(projects), Max: q.MaxProjects},
		Services:            QuotaUsage{Used: servicios, Max: q.MaxManagedServices},
		MaxPods:             q.MaxPods,
		RequestCPU:          q.RequestCPU,
		RequestMemory:       q.RequestMemory,
		LimitCPU:            q.LimitCPU,
		LimitMemory:         q.LimitMemory,
		PostgresPVCSize:     q.PostgresPVCSize,
		BackupRetentionDays: q.BackupRetentionDays,
	}, nil
}
