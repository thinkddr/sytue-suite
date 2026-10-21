// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

const domainCols = `id, tenant, project_id, hostname, status, verify_token, is_primary, created_at, verified_at`

func scanDomain(row pgx.Row, t string) (domain.Domain, error) {
	var d domain.Domain
	if err := row.Scan(&d.ID, &d.Tenant, &d.ProjectID, &d.Hostname, &d.Status,
		&d.VerifyToken, &d.IsPrimary, &d.CreatedAt, &d.VerifiedAt); err != nil {
		return domain.Domain{}, err
	}
	d.Tenant = t
	return d, nil
}

// AddDomain asocia un dominio personalizado a un proyecto (estado pending).
func (s *Store) AddDomain(ctx context.Context, projectID uuid.UUID, hostname, verifyToken string) (domain.Domain, error) {
	t := tenancy.MustFrom(ctx)
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	var d domain.Domain
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx,
			`SELECT true FROM projects WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, projectID).Scan(&ok); err != nil {
			return normalizeNoRows(err, domain.ErrNotFound)
		}
		// Verificado por otro proyecto (de cualquier organización): ocupado de
		// verdad. Una reclamación PENDIENTE ajena no lo ocupa (CLOUD-10).
		var verificado bool
		if err := enMantenimiento(ctx, tx, func() error {
			return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM domains
				WHERE lower(hostname)=$1 AND status IN ('verified','active'))`, hostname).Scan(&verificado)
		}); err != nil {
			return fmtErr("comprobar dominio", err)
		}
		if verificado {
			return domain.ErrDomainTaken
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO domains (tenant, project_id, hostname, verify_token)
			VALUES ($1, $2, $3, $4)
			RETURNING `+domainCols, t.Slug, projectID, hostname, verifyToken)
		var e error
		d, e = scanDomain(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrDomainTaken
		}
		return e
	})
	return d, err
}

// ListDomains lista los dominios de un proyecto.
func (s *Store) ListDomains(ctx context.Context, projectID uuid.UUID) ([]domain.Domain, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Domain
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+domainCols+` FROM domains WHERE tenant=$1 AND project_id=$2 ORDER BY created_at`,
			t.Slug, projectID)
		if err != nil {
			return fmtErr("listar dominios", err)
		}
		defer rows.Close()
		for rows.Next() {
			d, err := scanDomain(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}

// SetDomainStatus actualiza el estado de un dominio (verified/active/failed).
// verified marca verified_at. Cross-tenant para el controller.
func (s *Store) SetDomainStatus(ctx context.Context, tenant string, id uuid.UUID, status domain.DomainStatus) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		verified := status == domain.DomainVerified || status == domain.DomainActive
		tag, err := tx.Exec(ctx, `
			UPDATE domains SET status=$3,
				verified_at = CASE WHEN $4 THEN COALESCE(verified_at, now()) ELSE verified_at END
			WHERE tenant=$1 AND id=$2`, tenant, id, string(status), verified)
		if uniqueViolation(err) {
			// Otro proyecto lo verificó antes: esta reclamación pierde (CLOUD-10).
			return domain.ErrDomainTaken
		}
		if err != nil {
			return fmtErr("actualizar estado dominio", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// GetDomain devuelve un dominio por ID.
func (s *Store) GetDomain(ctx context.Context, id uuid.UUID) (domain.Domain, error) {
	t := tenancy.MustFrom(ctx)
	var d domain.Domain
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT `+domainCols+` FROM domains WHERE tenant=$1 AND id=$2`, t.Slug, id)
		var e error
		d, e = scanDomain(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Domain{}, domain.ErrNotFound
	}
	return d, err
}

// SetDomainPrimary marca un dominio como principal del proyecto (único).
func (s *Store) SetDomainPrimary(ctx context.Context, id uuid.UUID) (domain.Domain, error) {
	t := tenancy.MustFrom(ctx)
	var d domain.Domain
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var projectID uuid.UUID
		if err := tx.QueryRow(ctx,
			`SELECT project_id FROM domains WHERE tenant=$1 AND id=$2`, t.Slug, id).Scan(&projectID); err != nil {
			return normalizeNoRows(err, domain.ErrNotFound)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE domains SET is_primary=false WHERE tenant=$1 AND project_id=$2`, t.Slug, projectID); err != nil {
			return fmtErr("limpiar primary", err)
		}
		row := tx.QueryRow(ctx, `
			UPDATE domains SET is_primary=true WHERE tenant=$1 AND id=$2
			RETURNING `+domainCols, t.Slug, id)
		var e error
		d, e = scanDomain(row, t.Slug)
		return e
	})
	return d, err
}

// RemoveDomain elimina un dominio y devuelve sus datos (para eventos/K8s).
func (s *Store) RemoveDomain(ctx context.Context, id uuid.UUID) (domain.Domain, error) {
	t := tenancy.MustFrom(ctx)
	var d domain.Domain
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			DELETE FROM domains WHERE tenant=$1 AND id=$2
			RETURNING `+domainCols, t.Slug, id)
		var e error
		d, e = scanDomain(row, t.Slug)
		if errors.Is(e, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return e
	})
	return d, err
}

// ListPendingDomains devuelve los dominios pendientes de verificar/emitir TLS
// (cross-tenant) para el reconciliador del controller.
func (s *Store) ListPendingDomains(ctx context.Context, limit int) ([]domain.Domain, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []domain.Domain
	// Cross-tenant: InMaintenanceTx habilita la política de mantenimiento de
	// domains (0003); con FORCE RLS sería deny-by-default (0 filas).
	//
	// Dos cosas para que nadie atasque la cola de los demás (CLOUD-06): un
	// pendiente de más de siete días pasa a 'failed' (quien de verdad controla el
	// DNS lo vuelve a añadir), y se reparte por «el que lleva más sin mirarse»,
	// anotando checked_at en la misma sentencia.
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE domains SET status='failed'
			WHERE status='pending' AND created_at < now() - interval '7 days'`); err != nil {
			return fmtErr("caducar dominios pendientes", err)
		}
		rows, err := tx.Query(ctx,
			`UPDATE domains SET checked_at = now() WHERE id IN (
				SELECT id FROM domains WHERE status IN ('pending','verified')
				ORDER BY checked_at NULLS FIRST, created_at LIMIT $1 FOR UPDATE SKIP LOCKED)
			 RETURNING `+domainCols, limit)
		if err != nil {
			return fmtErr("listar dominios pendientes", err)
		}
		defer rows.Close()
		for rows.Next() {
			var d domain.Domain
			if err := rows.Scan(&d.ID, &d.Tenant, &d.ProjectID, &d.Hostname, &d.Status,
				&d.VerifyToken, &d.IsPrimary, &d.CreatedAt, &d.VerifiedAt); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return out, err
}
