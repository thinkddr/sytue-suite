// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// DeployFreeze devuelve la congelación de producción del proyecto y si la hay. El
// proyecto se resuelve primero bajo RLS: preguntar por un id ajeno contesta «no
// existe», no «no está congelado».
func (s *Service) DeployFreeze(ctx context.Context, projectID uuid.UUID) (domain.DeployFreeze, bool, error) {
	if _, err := s.store.GetProject(ctx, projectID); err != nil {
		return domain.DeployFreeze{}, false, err
	}
	return s.store.GetDeployFreeze(ctx, projectID)
}

// FreezeDeploys congela los despliegues de PRODUCCIÓN del proyecto con un motivo.
//
// Sobre un proyecto ya congelado solo cambia el motivo: la fecha y el autor
// siguen diciendo desde cuándo y quién lo paró.
func (s *Service) FreezeDeploys(ctx context.Context, projectID uuid.UUID, reason string) (domain.DeployFreeze, error) {
	r, err := domain.NormalizeFreezeReason(reason)
	if err != nil {
		return domain.DeployFreeze{}, err
	}
	f, err := s.store.SetDeployFreeze(ctx, projectID, r, actorSub(ctx))
	if err != nil {
		return domain.DeployFreeze{}, err
	}
	s.audit(ctx, "deploy.freeze", projectID.String(), map[string]any{"reason": r})
	return f, nil
}

// UnfreezeDeploys vuelve a dejar que producción reciba despliegues. Idempotente:
// descongelar lo que no estaba congelado no es un error, pero tampoco deja una
// entrada de auditoría que diga que algo cambió.
func (s *Service) UnfreezeDeploys(ctx context.Context, projectID uuid.UUID) error {
	if _, err := s.store.GetProject(ctx, projectID); err != nil {
		return err
	}
	cambio, err := s.store.ClearDeployFreeze(ctx, projectID)
	if err != nil {
		return err
	}
	if cambio {
		s.audit(ctx, "deploy.unfreeze", projectID.String(), nil)
	}
	return nil
}

// deployOrigin dice en la auditoría quién intentó publicar: el autor del push, el
// disparador de un CMS («deploy-hook») o la persona. Sin él, «bloqueado» no dice
// si fue el robot o un compañero, que es lo primero que se pregunta.
func deployOrigin(in DeployInput) string {
	if in.CreatedBy != "" {
		return in.CreatedBy
	}
	return "api"
}

// checkProductionFreeze es la verja de todo lo que publica algo NUEVO en
// producción (Deploy y PromotePreview). Lo que se bloquea queda en la auditoría:
// un push que no llegó a producción tiene que poder encontrarse después, o el
// equipo creerá que el webhook se perdió.
func (s *Service) checkProductionFreeze(ctx context.Context, projectID uuid.UUID, origen string) error {
	f, congelado, err := s.store.GetDeployFreeze(ctx, projectID)
	if err != nil {
		return err
	}
	if !congelado {
		return nil
	}
	s.audit(ctx, "deploy.blocked_frozen", projectID.String(), map[string]any{
		"via": origen, "reason": f.Reason,
	})
	return domain.ErrDeployFrozen
}
