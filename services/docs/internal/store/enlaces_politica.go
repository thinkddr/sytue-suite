// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// cerrojoPoliticaEnlaces + organización: lo toma compartido quien crea un enlace y
// exclusivo quien cambia el techo, para que el ajuste de los enlaces vivos no se cruce
// con una creación a medias.
const cerrojoPoliticaEnlaces = "doc_org_politica_enlaces:"

// PoliticaEnlaces devuelve el techo de caducidad de la organización del contexto; sin
// fila, el techo por defecto.
func (s *Store) PoliticaEnlaces(ctx context.Context) (domain.PoliticaEnlaces, error) {
	p := domain.PoliticaEnlaces{MaxDias: domain.TechoEnlacesPorDefecto, PorDefecto: true}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT max_dias FROM doc_org_politica_enlaces`).Scan(&p.MaxDias)
		if isNoRows(err) {
			return nil
		}
		if err == nil {
			p.PorDefecto = false
		}
		return err
	})
	if err != nil {
		return domain.PoliticaEnlaces{}, fmt.Errorf("store: política de enlaces: %w", err)
	}
	return p, nil
}

// GuardarPoliticaEnlaces fija el techo de la organización y, EN LA MISMA transacción, lo
// aplica a los enlaces vivos que ya no lo cumplen: pasan a caducar en su techo (contado
// desde que se crearon) y nunca antes del plazo de gracia; no se revocan ni se borran, y
// quedan marcados para que la interfaz lo avise. Con 0 (sin techo) no se toca ninguno:
// quitar el techo no alarga lo que ya tenía fecha. Devuelve cuántos enlaces ajustó.
func (s *Store) GuardarPoliticaEnlaces(ctx context.Context, maxDias int, sub string) (int64, error) {
	if maxDias < 0 || maxDias > domain.TechoEnlacesMaximo {
		return 0, domain.ErrTechoInvalido
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return 0, err
	}
	var ajustados int64
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, cerrojoPoliticaEnlaces+t.Slug); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO doc_org_politica_enlaces (tenant, max_dias, updated_by)
			VALUES ($1, $2, $3)
			ON CONFLICT (tenant) DO UPDATE
			SET max_dias = EXCLUDED.max_dias, updated_by = EXCLUDED.updated_by, updated_at = now()`,
			t.Slug, maxDias, sub); err != nil {
			return err
		}
		if maxDias == 0 {
			return nil
		}
		tag, err := tx.Exec(ctx, `
			WITH limite AS (
			    SELECT id, GREATEST(created_at + make_interval(days => $1),
			                        now() + make_interval(days => LEAST($1, $2))) AS hasta
			    FROM doc_share_links
			    WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())
			)
			UPDATE doc_share_links l
			SET expires_at = limite.hasta, ajustado_por_politica_at = now()
			FROM limite
			WHERE l.id = limite.id AND (l.expires_at IS NULL OR l.expires_at > limite.hasta)`,
			maxDias, domain.GraciaEnlacesDias)
		ajustados = tag.RowsAffected()
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("store: guardar política de enlaces: %w", err)
	}
	return ajustados, nil
}
