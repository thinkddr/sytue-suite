// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// MaxVentanaAuditoria es el tope de operaciones que resume una página.
const MaxVentanaAuditoria = 5000

// AuditoriaOps lista, de la más reciente a la más antigua, quién cambió qué y cuándo
// en un documento: grupos de operaciones por (autor, tipo, clave, minuto). Solo lee el
// tipo y el NOMBRE de la clave de cada operación: el valor no sale de la base de datos.
//
// La página resume las `ventana` operaciones más recientes (con seq < antesDe; 0 =
// desde el final). Paginar por operaciones y no por grupos es lo que impide que dos
// grupos que se entrelazan en el tiempo se pierdan en el corte: el único efecto es que
// un grupo cortado por el borde de la ventana sale en dos entradas contiguas.
func (s *Store) AuditoriaOps(ctx context.Context, docID string, antesDe int64, ventana int) (domain.PaginaAuditoria, error) {
	ventana = min(max(ventana, 1), MaxVentanaAuditoria)
	pag := domain.PaginaAuditoria{Entradas: []domain.EntradaAuditoria{}}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		// `rutas` es DÓNDE tocó un parche (hoja/celda, diapositiva/caja): los tramos de la
		// ruta de cada cambio, sin valores. El LEFT JOIN LATERAL multiplica filas por ruta,
		// de ahí count(DISTINCT seq).
		const q = `
			WITH v AS (
				SELECT seq, author_sub, op->>'kind' AS kind, left(coalesce(op->>'key', ''), 64) AS clave, created_at,
				       CASE WHEN jsonb_typeof(op->'cambios') = 'array' THEN op->'cambios' ELSE '[]'::jsonb END AS cambios
				FROM doc_operations
				WHERE document_id = $1 AND ($2::bigint = 0 OR seq < $2)
				ORDER BY seq DESC
				LIMIT $3
			)
			SELECT min(v.seq), max(v.seq), v.author_sub, v.kind, v.clave, count(DISTINCT v.seq), max(v.created_at),
			       (SELECT count(*) FROM v), (SELECT min(seq) FROM v),
			       coalesce((array_agg(DISTINCT r.ruta) FILTER (WHERE r.ruta IS NOT NULL))[1:5], '{}')
			FROM v
			LEFT JOIN LATERAL (
				SELECT left((SELECT string_agg(CASE jsonb_typeof(t) WHEN 'string' THEN t #>> '{}' ELSE t->>'id' END, '/')
				             FROM jsonb_array_elements(c->'path') t), 96) AS ruta
				FROM jsonb_array_elements(v.cambios) c
			) r ON true
			GROUP BY v.author_sub, v.kind, v.clave, date_trunc('minute', v.created_at)
			ORDER BY max(v.seq) DESC`
		rows, err := tx.Query(ctx, q, docID, antesDe, ventana)
		if err != nil {
			return err
		}
		defer rows.Close()
		var vistas int
		var minSeq int64
		for rows.Next() {
			var e domain.EntradaAuditoria
			if err := rows.Scan(&e.SeqDesde, &e.SeqHasta, &e.AuthorSub, &e.Kind, &e.Key, &e.Cuantas, &e.At, &vistas, &minSeq, &e.Rutas); err != nil {
				return err
			}
			pag.Entradas = append(pag.Entradas, e)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if vistas == ventana { // ventana llena: puede haber operaciones más antiguas
			pag.SiguienteAntesDe = minSeq
		}
		return nil
	})
	if err != nil {
		return domain.PaginaAuditoria{}, fmt.Errorf("store: auditoría de operaciones: %w", err)
	}
	return pag, nil
}
