// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// selectOwnersForUpdate reclama las filas owner del tenant. FOR UPDATE es lo que
// hace atómica la comprobación: dos transacciones que degraden a dos owners
// DISTINTOS no se bloquean entre sí por sus propias filas (tocan filas distintas),
// así que sin este lock ambas leen «hay 2 owners», ambas pasan y la organización
// se queda con CERO —un estado del que solo se sale por la base de datos—. Con él,
// la segunda espera a la primera y, bajo READ COMMITTED, PostgreSQL re-evalúa el
// predicado tras conceder el lock: la fila que la otra acaba de degradar ya no
// cumple role='owner' y desaparece del recuento, así que la segunda se rechaza.
//
// ORDER BY sub fija el orden de adquisición: dos transacciones que bloqueen el
// mismo conjunto lo hacen en la misma secuencia y no se abrazan en un deadlock.
const selectOwnersForUpdate = `SELECT sub FROM memberships
	WHERE tenant=$1 AND role=$2 ORDER BY sub FOR UPDATE`

// guardLastOwner rechaza, DENTRO de la transacción que va a mutar, el cambio que
// dejaría al tenant sin ningún owner: degradar (newRole distinto de owner) o
// eliminar (newRole vacío) al único que queda. Vive en el store —y no en la capa
// de servicio— precisamente porque la comprobación solo vale si comparte
// transacción y lock con la escritura; separadas son un TOCTOU.
//
// Mantener el rol owner nunca reduce su número: se acepta sin consultar ni
// bloquear. Si no hay ninguna fila owner tampoco rechaza: el owner inicial existe
// antes que las membresías y lo cubre el fallback al claim del JWT
// (Service.EffectiveRole).
func guardLastOwner(ctx context.Context, tx pgx.Tx, tenant, sub string, newRole domain.Role) error {
	if newRole == domain.RoleOwner {
		return nil
	}
	rows, err := tx.Query(ctx, selectOwnersForUpdate, tenant, string(domain.RoleOwner))
	if err != nil {
		return fmtErr("bloquear owners", err)
	}
	defer rows.Close()
	owners, subIsOwner := 0, false
	for rows.Next() {
		var owner string
		if err := rows.Scan(&owner); err != nil {
			return fmtErr("leer owner", err)
		}
		owners++
		if owner == sub {
			subIsOwner = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmtErr("bloquear owners", err)
	}
	if subIsOwner && owners == 1 {
		return fmt.Errorf("%w: no puedes dejar la organización sin ningún owner", domain.ErrInvalidState)
	}
	return nil
}

// ListMemberships lista los colaboradores del tenant activo.
func (s *Store) ListMemberships(ctx context.Context) ([]domain.Membership, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Membership
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT tenant, sub, role, created_at FROM memberships WHERE tenant=$1 ORDER BY created_at`, t.Slug)
		if err != nil {
			return fmtErr("listar miembros", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m domain.Membership
			if err := rows.Scan(&m.Tenant, &m.Sub, &m.Role, &m.CreatedAt); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// UpsertMembership crea o actualiza el rol de un colaborador (idempotente).
func (s *Store) UpsertMembership(ctx context.Context, sub string, role domain.Role) (domain.Membership, error) {
	t := tenancy.MustFrom(ctx)
	var m domain.Membership
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := guardLastOwner(ctx, tx, t.Slug, sub, role); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO memberships (tenant, sub, role)
			VALUES ($1, $2, $3)
			ON CONFLICT (tenant, sub) DO UPDATE SET role = EXCLUDED.role
			RETURNING tenant, sub, role, created_at`,
			t.Slug, sub, string(role))
		e := row.Scan(&m.Tenant, &m.Sub, &m.Role, &m.CreatedAt)
		if checkViolation(e) {
			return domain.ErrInvalidInput
		}
		return e
	})
	return m, err
}

// DeleteMembership elimina un colaborador del tenant. ErrNotFound si no existe.
func (s *Store) DeleteMembership(ctx context.Context, sub string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := guardLastOwner(ctx, tx, t.Slug, sub, ""); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM memberships WHERE tenant=$1 AND sub=$2`, t.Slug, sub)
		if err != nil {
			return fmtErr("eliminar miembro", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		// Quitar a un colaborador ARRASTRA los tokens de API que emitió, en la MISMA
		// transacción: `expires_at` es opcional (ResolveAPIToken acepta NULL), así que un
		// `scl_…` con scope deploy emitido por el saliente seguía sirviendo PARA SIEMPRE
		// aunque su membresía ya no existiera. Mismo criterio que la baja de organización
		// en Sytue ID (commit 4eaeaf6): la revocación cascadea con el cambio de estado.
		if _, err := tx.Exec(ctx,
			`DELETE FROM api_tokens WHERE tenant=$1 AND created_by=$2`, t.Slug, sub); err != nil {
			return fmtErr("revocar tokens del miembro", err)
		}
		return nil
	})
}

// GetMembershipRole devuelve el rol de un sub en el tenant activo. ErrNotFound si
// el usuario no tiene membership explícito (el llamante hace fallback al claim).
func (s *Store) GetMembershipRole(ctx context.Context, sub string) (domain.Role, error) {
	t := tenancy.MustFrom(ctx)
	var role domain.Role
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT role FROM memberships WHERE tenant=$1 AND sub=$2`, t.Slug, sub).Scan(&role)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return role, err
}

// CountMemberships cuenta los miembros de la organización activa. Es la fuente de
// los PUESTOS DE SUITE facturables: se derivan de quién está en la organización,
// no de un número que alguien teclee y se quede obsoleto.
func (s *Store) CountMemberships(ctx context.Context) (int, error) {
	t := tenancy.MustFrom(ctx)
	var n int
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM memberships WHERE tenant=$1`, t.Slug).Scan(&n); err != nil {
			return fmtErr("contar miembros", err)
		}
		return nil
	})
	return n, err
}
