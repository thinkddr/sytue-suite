// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// ListarVistas devuelve las vistas guardadas de una tabla (lector).
func (s *Store) ListarVistas(ctx context.Context, a domain.Actor, tabla string) ([]domain.Vista, error) {
	out := []domain.Vista{}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		if _, err := autorizarTabla(ctx, tx, a, tabla, domain.AccLeer); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text, table_id::text, name, kind, config, created_by FROM base_views WHERE tenant = $1 AND table_id = $2 ORDER BY created_at, id LIMIT $3`,
			a.Tenant, tabla, domain.MaxVistasPorTabla)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.Vista
			var cfg []byte
			if err := rows.Scan(&v.ID, &v.TableID, &v.Name, &v.Kind, &cfg, &v.CreatedBy); err != nil {
				return err
			}
			_ = json.Unmarshal(cfg, &v.Config)
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

// CrearVista guarda una vista (editor). La configuración se valida contra los
// campos de la tabla: no se guarda un filtro que el servidor no sabría ejecutar.
func (s *Store) CrearVista(ctx context.Context, a domain.Actor, tabla, nombre string, kind domain.TipoVista, cfg domain.ConfigVista) (domain.Vista, error) {
	nombre, err := domain.LimpiarNombre(nombre)
	if err != nil {
		return domain.Vista{}, err
	}
	v := domain.Vista{TableID: tabla, Name: nombre, Kind: kind, CreatedBy: a.Sub}
	err = s.en(ctx, a, func(tx pgx.Tx) error {
		base, err := autorizarTabla(ctx, tx, a, tabla, domain.AccGestionVistas)
		if err != nil {
			return err
		}
		campos, err := camposDe(ctx, tx, a, tabla)
		if err != nil {
			return err
		}
		if v.Config, err = domain.ValidarConfig(kind, cfg, campos); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_views WHERE tenant = $1 AND table_id = $2`, a.Tenant, tabla).Scan(&n); err != nil {
			return err
		}
		if n >= domain.MaxVistasPorTabla {
			return domain.ErrLimite
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO base_views (tenant, base_id, table_id, name, kind, config, created_by)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7) RETURNING id::text`,
			a.Tenant, base, tabla, nombre, kind, jsonDe(v.Config), a.Sub).Scan(&v.ID); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, nil, "view.create", nil, jsonDe(map[string]any{"name": nombre, "kind": kind}))
	})
	return v, err
}

// BorrarVista borra una vista (editor).
func (s *Store) BorrarVista(ctx context.Context, a domain.Actor, vista string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		var tabla, base string
		if err := errNoFilas(tx.QueryRow(ctx, `SELECT table_id::text, base_id::text FROM base_views WHERE id = $1 AND tenant = $2`, vista, a.Tenant).Scan(&tabla, &base)); err != nil {
			return err
		}
		if err := autorizar(ctx, tx, a, base, tabla, domain.AccGestionVistas); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM base_views WHERE id = $1 AND tenant = $2`, vista, a.Tenant); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, nil, "view.delete", nil, nil)
	})
}
