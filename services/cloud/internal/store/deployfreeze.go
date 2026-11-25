// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// GetDeployFreeze devuelve la congelación de producción del proyecto y si la
// hay. «No hay fila» es el estado normal, no un error.
func (s *Store) GetDeployFreeze(ctx context.Context, projectID uuid.UUID) (domain.DeployFreeze, bool, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.DeployFreeze
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT project_id, reason, frozen_by, frozen_at
			  FROM deploy_freezes
			 WHERE tenant=$1 AND project_id=$2`, t.Slug, projectID,
		).Scan(&f.ProjectID, &f.Reason, &f.FrozenBy, &f.FrozenAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeployFreeze{}, false, nil
	}
	if err != nil {
		return domain.DeployFreeze{}, false, fmtErr("leer congelación", err)
	}
	return f, true, nil
}

// SetDeployFreeze congela la producción del proyecto, o actualiza el motivo si ya
// lo estaba. Volver a congelar NO reinicia `frozen_at` ni el autor: la pregunta
// que contesta esa fecha es «desde cuándo está parado», y un cambio de motivo no
// la cambia.
//
// El proyecto se comprueba en la misma transacción y bajo RLS: un id de otra
// organización no existe para esta, y la clave foránea sola no lo distinguiría.
func (s *Store) SetDeployFreeze(ctx context.Context, projectID uuid.UUID, reason, by string) (domain.DeployFreeze, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.DeployFreeze
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO deploy_freezes (project_id, tenant, reason, frozen_by)
			SELECT p.id, p.tenant, $3, $4
			  FROM projects p
			 WHERE p.tenant=$1 AND p.id=$2
			ON CONFLICT (project_id) DO UPDATE SET reason = EXCLUDED.reason
			RETURNING project_id, reason, frozen_by, frozen_at`,
			t.Slug, projectID, reason, by)
		if err := row.Scan(&f.ProjectID, &f.Reason, &f.FrozenBy, &f.FrozenAt); err != nil {
			return normalizeNoRows(err, domain.ErrNotFound)
		}
		return nil
	})
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.DeployFreeze{}, fmtErr("congelar despliegues", err)
	}
	return f, err
}

// ClearDeployFreeze descongela. Devuelve si había algo que descongelar: la
// auditoría solo apunta lo que de verdad cambió.
func (s *Store) ClearDeployFreeze(ctx context.Context, projectID uuid.UUID) (bool, error) {
	t := tenancy.MustFrom(ctx)
	var borradas int64
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`DELETE FROM deploy_freezes WHERE tenant=$1 AND project_id=$2`, t.Slug, projectID)
		if err != nil {
			return err
		}
		borradas = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return false, fmtErr("descongelar despliegues", err)
	}
	return borradas > 0, nil
}
