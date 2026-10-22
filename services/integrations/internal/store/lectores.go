// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// Quién puede consultar cada conexión (NEG-06, mig 0013). Los administradores no pasan
// por aquí —su rol ya les deja—: esto responde por los demás.

// Lectores devuelve quién creó la conexión y su lista. ErrNotFound si no es de la
// organización activa.
func (s *Store) Lectores(ctx context.Context, id string) (string, []domain.Lector, error) {
	var (
		creadaPor string
		lista     = []domain.Lector{}
	)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT created_by FROM app_instances WHERE org_id = current_setting('app.tenant', true) AND id = $1`,
			id).Scan(&creadaPor); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT kind, principal, name FROM instance_readers
			  WHERE org_id = current_setting('app.tenant', true) AND instance_id = $1
			  ORDER BY kind, lower(name), principal`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var l domain.Lector
			if err := rows.Scan(&l.Kind, &l.ID, &l.Name); err != nil {
				return err
			}
			lista = append(lista, l)
		}
		return rows.Err()
	})
	if isNoRows(err) {
		return "", nil, domain.ErrNotFound
	}
	if err != nil {
		return "", nil, fmt.Errorf("store: leer la lista de la conexión: %w", err)
	}
	return creadaPor, lista, nil
}

// PonerLectores sustituye la lista entera de una conexión, en una transacción: quien la
// lea a la vez ve la de antes o la de después, nunca media. ErrNotFound si la conexión
// no es de la organización activa.
func (s *Store) PonerLectores(ctx context.Context, id string, lista []domain.Lector) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var existe bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM app_instances WHERE org_id = current_setting('app.tenant', true) AND id = $1)`,
			id).Scan(&existe); err != nil {
			return err
		}
		if !existe {
			return domain.ErrNotFound
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM instance_readers WHERE org_id = current_setting('app.tenant', true) AND instance_id = $1`, id); err != nil {
			return err
		}
		for _, l := range lista {
			if _, err := tx.Exec(ctx,
				`INSERT INTO instance_readers (instance_id, org_id, kind, principal, name) VALUES ($1, $2, $3, $4, $5)`,
				id, t.Slug, l.Kind, l.ID, l.Name); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("store: guardar la lista de la conexión: %w", err)
	}
	return nil
}

// Consultables devuelve las conexiones de la organización activa que puede consultar
// alguien que NO la administra: las que creó y las que le nombran en su lista, a él o a
// uno de sus grupos. Sin `sub` no hay ninguna.
func (s *Store) Consultables(ctx context.Context, sub string, grupos []string) (map[string]bool, error) {
	out := map[string]bool{}
	if sub == "" {
		return out, nil
	}
	if grupos == nil {
		grupos = []string{}
	}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT i.id::text FROM app_instances i
			 WHERE i.org_id = current_setting('app.tenant', true)
			   AND (i.created_by = $1 OR EXISTS (
			        SELECT 1 FROM instance_readers r
			         WHERE r.org_id = current_setting('app.tenant', true) AND r.instance_id = i.id
			           AND ((r.kind = 'user' AND r.principal = $1)
			             OR (r.kind = 'group' AND r.principal = ANY($2)))))`,
			sub, grupos)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out[id] = true
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: conexiones consultables: %w", err)
	}
	return out, nil
}
