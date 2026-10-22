// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// ---------------------------------------------------------------------------
// Recomendación de tamaño de un proyecto
// ---------------------------------------------------------------------------
//
// Une las dos mitades: las muestras de uso REAL que recoge el controlador
// (internal/controller/rightsizing.go) y el criterio puro que decide qué hacer
// con ellas (domain.RecommendInstanceSize).
//
// NO hay endpoint de «aplicar»: aplicar la recomendación es cambiar el tamaño
// del proyecto, y para eso ya existe UpdateProject, que valida, audita y respeta
// la cuota del plan. Un endpoint propio duplicaría esas tres cosas y sería el
// único camino de cambio de tamaño que se saltaría alguna. Lo de «1 clic» es la
// interfaz, no un verbo nuevo en la API.

// sizingWindow es cuánto histórico se le da al motor. Más que el mínimo que
// exige para bajar (72 h), para que la decisión no dependa de haber muestreado
// justo en el borde.
const sizingWindow = 7 * 24 * time.Hour

// samplesStore es el seam de lectura de muestras. Por aserción sobre s.store
// para no tocar service.go, que lleva otra sesión.
type samplesStore interface {
	ListResourceSamples(ctx context.Context, projectID uuid.UUID, since time.Time) ([]domain.ResourceSample, error)
	ListServices(ctx context.Context, projectID uuid.UUID) ([]domain.ManagedService, error)
	GetCohort(ctx context.Context, key domain.CohortKey) (domain.Cohort, bool, error)
}

// La aserción en compilación evita que una deriva de firma se convierta en un
// 500 silencioso en caliente.
var _ samplesStore = (*store.Store)(nil)

// ProjectSizingAdvice devuelve qué hacer con el tamaño de un proyecto según su
// consumo real. Sin muestras bastantes devuelve la recomendación explícita de
// «sin datos suficientes», que NO es un error: es la respuesta correcta para un
// proyecto recién creado, y la interfaz tiene que poder decirlo con esas
// palabras en vez de callar o inventar.
func (s *Service) ProjectSizingAdvice(ctx context.Context, id uuid.UUID) (domain.SizingAdvice, error) {
	proj, err := s.store.GetProject(ctx, id)
	if err != nil {
		return domain.SizingAdvice{}, err
	}
	ss, ok := s.store.(samplesStore)
	if !ok {
		// Un store sin muestras (dobles de test) no es un fallo del endpoint.
		return domain.RecommendInstanceSize(proj.EffectiveInstanceSize().Name, proj.EffectiveInstanceCount(), nil), nil
	}
	samples, err := ss.ListResourceSamples(ctx, id, time.Now().Add(-sizingWindow))
	if err != nil {
		return domain.SizingAdvice{}, err
	}
	adv := domain.RecommendInstanceSize(
		proj.EffectiveInstanceSize().Name, proj.EffectiveInstanceCount(), samples,
	)
	if adv.Reason != domain.ReasonInsufficientData {
		// Con datos propios no se consulta la cohorte siquiera: además de ser la
		// decisión correcta, ahorra una consulta en el caso normal.
		return adv, nil
	}
	return s.withCohort(ctx, ss, proj, adv), nil
}

// withCohort completa el consejo con el agregado anónimo de las apps
// comparables. Un fallo aquí NO tumba la respuesta: la recomendación propia ya
// está calculada, y quedarse sin el «las apps como la tuya…» es infinitamente
// mejor que devolver un 500 en la pantalla principal del proyecto.
func (s *Service) withCohort(ctx context.Context, ss samplesStore, proj domain.Project, adv domain.SizingAdvice) domain.SizingAdvice {
	svcs, err := ss.ListServices(ctx, proj.ID)
	if err != nil {
		return adv
	}
	kinds := make([]domain.ServiceKind, 0, len(svcs))
	for _, sv := range svcs {
		kinds = append(kinds, sv.Kind)
	}
	key := domain.CohortKey{
		Framework: string(proj.Framework),
		Services:  domain.ServicesKey(kinds),
		Size:      proj.EffectiveInstanceSize().Name,
	}
	c, found, err := ss.GetCohort(ctx, key)
	if err != nil || !found {
		return adv
	}
	return domain.ApplyCohort(adv, proj.EffectiveInstanceCount(), c)
}
